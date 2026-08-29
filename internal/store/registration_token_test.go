package store_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/superfive666/agent-hub/internal/store"
)

// 需求：注册 token 有两道保险 —— **24 小时自动过期**，以及**用掉即刻作废**。
//
// 两道都落在同一条 SQL 上（见 ExchangeRegistrationToken）：
//
//	UPDATE registration_token SET used_at = now()
//	WHERE token_hash = $1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at > now()
//
// 条件更新即认领 —— 「查一下还能不能用」和「把它标成用过」是同一个原子动作。
// 下面几条用例分别盯住这条语句的每一个 WHERE 条件；少任何一条，
// 这张一次性 token 就不再是一次性的。

// 签发时不给 TTL（或给 0）要落到 24 小时这个默认值上。
// 写死在这里是故意的：这个数字是安全属性的一部分，改它应该让用例红。
func TestRegistrationTokenDefaultsTo24Hours(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	agent := mkAgent(t, s, "rover")

	for _, ttl := range []time.Duration{0, -time.Hour} {
		_, expiresAt, err := s.IssueRegistrationToken(ctx, agent, ttl)
		if err != nil {
			t.Fatalf("签发失败: %v", err)
		}
		got := time.Until(expiresAt)
		// 留一分钟余量给这几行代码本身的耗时，不要卡到秒
		if got < 23*time.Hour+59*time.Minute || got > 24*time.Hour+time.Minute {
			t.Errorf("ttl=%v 时有效期 = %v, want ≈24h —— 默认值是安全属性的一部分", ttl, got)
		}
	}

	// 显式给的 TTL 要被尊重，不能被默认值盖掉
	_, expiresAt, err := s.IssueRegistrationToken(ctx, agent, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got := time.Until(expiresAt); got > 31*time.Minute {
		t.Errorf("显式 TTL 被忽略了：有效期 = %v, want ≈30m", got)
	}
}

// 过期的 token 换不出凭证 —— `expires_at > now()` 那个条件。
//
// 用一个已经过去的 TTL 直接造出过期状态，比 sleep 真等一段时间可靠得多，
// 也不会让这条用例变成跑得最慢的那个。
func TestExpiredRegistrationTokenIsRejected(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	agent := mkAgent(t, s, "rover")

	// 负的 TTL 会被当成「没给」而落到 24h 默认值，所以这里不能走 IssueRegistrationToken
	// 的默认分支 —— 先正常签一张，再把它的 expires_at 拨到过去。
	tok, _, err := s.IssueRegistrationToken(ctx, agent, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().ExecContext(ctx,
		`UPDATE registration_token SET expires_at = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}

	if _, _, err := s.ExchangeRegistrationToken(ctx, tok); !errors.Is(err, store.ErrTokenInvalid) {
		t.Errorf("过期的 token 应当被拒（ErrTokenInvalid），实得 %v", err)
	}
	if n := countRows(t, s, `SELECT count(*) FROM agent_credential`); n != 0 {
		t.Errorf("过期 token 竟然换出了 %d 份凭证", n)
	}
}

// 被吊销的 token 换不出凭证 —— `revoked_at IS NULL` 那个条件。
func TestRevokedRegistrationTokenIsRejected(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	agent := mkAgent(t, s, "rover")

	tok, _, err := s.IssueRegistrationToken(ctx, agent, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().ExecContext(ctx,
		`UPDATE registration_token SET revoked_at = now()`); err != nil {
		t.Fatal(err)
	}

	if _, _, err := s.ExchangeRegistrationToken(ctx, tok); !errors.Is(err, store.ErrTokenInvalid) {
		t.Errorf("已作废的 token 应当被拒，实得 %v", err)
	}
}

// **「用掉即刻作废」只有在兑换是原子的时候才成立。**
//
// 这条用例是那句话的地基：如果实现写成「先 SELECT 看看能不能用，再 UPDATE 标记」，
// 两个同时到达的请求会双双通过检查，于是**同一张一次性 token 换出两份长期凭证** ——
// 而且两边都拿到 200，没有任何地方报错，事后只能从 agent_credential 多出来的那一行
// 看出不对劲。
//
// **为什么要在用例内部重复很多轮**：单轮打八个并发抓不住这个 bug —— 实测把实现
// 改成非原子的「先查再改」，单轮照样全绿，要跑到 -count=15 才会红。数据库那几个
// 往返本身就把请求拉开了，撞不上那个窗口。所以这里自己重复 rounds 轮，
// 让一次普通的 `go test` 就是可靠的，而不是把可靠性寄托在「多跑几次」上。
func TestConcurrentExchangeYieldsExactlyOneCredential(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	const (
		rounds = 25
		racers = 8
	)
	for round := 0; round < rounds; round++ {
		agent := mkAgent(t, s, fmt.Sprintf("rover-%d", round))
		tok, _, err := s.IssueRegistrationToken(ctx, agent, time.Hour)
		if err != nil {
			t.Fatal(err)
		}

		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			won     int
			invalid int
			other   []error
			start   = make(chan struct{})
		)
		for i := 0; i < racers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start // 一起放出去，才是真并发
				_, _, err := s.ExchangeRegistrationToken(ctx, tok)
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					won++
				case errors.Is(err, store.ErrTokenInvalid):
					invalid++
				default:
					other = append(other, err)
				}
			}()
		}
		close(start)
		wg.Wait()

		if len(other) > 0 {
			t.Fatalf("第 %d 轮出现了预期之外的错误: %v", round, other)
		}
		if won != 1 || invalid != racers-1 {
			t.Fatalf("第 %d 轮：换成 %d 次、被拒 %d 次，want 1 / %d —— 一次性 token 被并发换了多次",
				round, won, invalid, racers-1)
		}
		// 落库的凭证数才是最终判据：即使上面某一路把错误吞了，这里也会露出来
		if n := countRows(t, s,
			`SELECT count(*) FROM agent_credential WHERE agent_id = $1`, string(agent)); n != 1 {
			t.Fatalf("第 %d 轮：agent_credential 有 %d 行, want 1", round, n)
		}
	}

	// 每轮恰好烧掉一张
	if n := countRows(t, s,
		`SELECT count(*) FROM registration_token WHERE used_at IS NOT NULL`); n != rounds {
		t.Errorf("被标记用过的 token 有 %d 张, want %d", n, rounds)
	}
}
