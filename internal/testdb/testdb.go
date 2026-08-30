// Package testdb 给需要真 PostgreSQL 的测试提供一个干净的库。
//
// 为什么要有它：SKIP LOCKED、advisory lock、事务隔离的行为 mock 不出来，
// 而它们正是 outbox 方案的地基 —— 这些必须在真库上测。
// 但 go test 会并行跑不同的包，几个包同时 TRUNCATE 同一个库会互相撞成死锁，
// 所以这里用一把会话级 advisory lock 把它们串起来。
package testdb

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/superfive666/agent-hub/internal/store"
)

// serialLockID 是「谁在用测试库」的锁。和 worker 的单实例锁是两把，别复用。
const serialLockID int64 = 0x7E57D8

// New 返回一个已经清空的测试库。没配 TEST_DATABASE_URL 就跳过整个用例。
//
// **加了新表就要加进下面的 TRUNCATE 名单。** CASCADE 确实会连带清掉引用它的表，
// 但那是「碰巧被清到」而不是「说好要清」—— 名单是这份文件里唯一能读出
// 「用例之间到底重置了什么」的地方，漏写一张表的代价是用例之间互相污染，
// 而这种失败通常表现为「单独跑就过、一起跑就挂」。
//
//	make dev-db && make test-db
func New(t *testing.T) *store.Store {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("未设置 TEST_DATABASE_URL，跳过需要真库的用例")
	}
	ctx := context.Background()

	s, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("连接测试库: %v", err)
	}
	assertDisposable(t, s)

	// 独占整个测试库，直到本用例结束。
	conn, err := s.DB().Conn(ctx)
	if err != nil {
		s.Close()
		t.Fatalf("取独占连接: %v", err)
	}
	// 给取锁加超时，把「无限挂住」换成「一条能看懂的失败」。
	// 最常见的踩法是**同一个用例里调了两次 New** —— 第一把锁要到用例结束才放，
	// 第二次就永远等下去，go test 只会在十分钟后超时，什么线索都不给。
	if _, err := conn.ExecContext(ctx, `SET lock_timeout = '60s'`); err != nil {
		conn.Close()
		s.Close()
		t.Fatalf("设置锁超时: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, serialLockID); err != nil {
		conn.Close()
		s.Close()
		t.Fatalf("获取测试库独占锁失败（一个用例只能调一次 testdb.New；"+
			"要两个 server 就共用同一个 *store.Store）: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, serialLockID)
		conn.Close()
		s.Close()
	})

	if _, err := s.DB().ExecContext(ctx, `
		TRUNCATE inbox_event, outbox_event, agent_inbox_state, mention, thread_watcher,
		         todo_step, post, todo, tweet, thread, agent_card, agent_credential,
		         agent_dead_letter, registration_token, agent, audit_log,
		         subscription RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("清空测试库: %v", err)
	}
	return s
}

// once 保证防呆只在**第一次** New 时判一次。
// 之后每个用例都会 TRUNCATE，库里当然是空的 —— 每次都判等于只判了个寂寞。
var once sync.Once

// assertDisposable 拦住「把测试跑到生产库上」这一种事故。
//
// New 会 TRUNCATE 掉 agent、audit_log、post、thread…… 几乎所有表。
// 而这个仓库会被 clone 到跑着 hub 的那台机器上（agent 就住在那儿），
// 那里 `DATABASE_URL` 指的就是生产库 —— 只要有人顺手
// `TEST_DATABASE_URL=$DATABASE_URL go test ./...`，整个平台当场清空，
// 而且**没有任何一步会问你确不确定**。
//
// 判据不是「库叫什么名字」（生产和开发很可能都叫 agenthub），而是
// **这个库里有没有别人的数据**：
//
//   - platform_config 里打过测试标记 → 是我们自己的测试库，放行（标记不在 TRUNCATE 名单里，跨用例还在）
//   - 没标记，但 agent 和 audit_log 都是空的 → 干净的新库，打上标记后放行
//   - 没标记，却有 agent 或审计记录 → **这是别人在用的库，停下**
//
// 真要清一个有数据的库，`AGENT_HUB_TEST_DB_FORCE=1` 明说一次。
func assertDisposable(t *testing.T, s *store.Store) {
	t.Helper()
	var fatal string
	once.Do(func() {
		ctx := context.Background()
		db := s.DB()

		var marked bool
		// platform_config 是单行表，可能一行都还没有 —— 那也算「没标记」。
		if err := db.QueryRowContext(ctx,
			`SELECT coalesce((SELECT config->>'testDatabase' = 'true' FROM platform_config LIMIT 1), false)`,
		).Scan(&marked); err != nil {
			fatal = "读测试库标记失败: " + err.Error()
			return
		}
		if marked {
			return
		}

		var agents, audits int
		if err := db.QueryRowContext(ctx,
			`SELECT (SELECT count(*) FROM agent), (SELECT count(*) FROM audit_log)`,
		).Scan(&agents, &audits); err != nil {
			fatal = "检查测试库是否干净失败: " + err.Error()
			return
		}

		forced := os.Getenv("AGENT_HUB_TEST_DB_FORCE") == "1"
		if (agents > 0 || audits > 0) && !forced {
			fatal = fmt.Sprintf(
				"拒绝在这个库上跑测试：里面已经有 %d 个 agent、%d 条审计记录。\n"+
					"测试会 TRUNCATE 掉几乎所有表 —— 如果 TEST_DATABASE_URL 指的是生产库，"+
					"整个平台会当场清空。\n"+
					"先确认 TEST_DATABASE_URL 指对了地方（`make dev-db` 起的那个是安全的）。\n"+
					"确实想清掉这个库：AGENT_HUB_TEST_DB_FORCE=1 再跑一次。",
				agents, audits)
			return
		}

		// 干净的库、或者已经明说要清的库，打上标记。
		// **强清过一次之后也要标记**：它从此就是个测试库了，
		// 再要求每次都带 FORCE 只会训练人养成无脑加 FORCE 的习惯 —— 那这道防呆就白做了。
		if _, err := db.ExecContext(ctx, `
			INSERT INTO platform_config (id, config) VALUES (true, '{"testDatabase": true}'::jsonb)
			ON CONFLICT (id) DO UPDATE
			SET config = platform_config.config || '{"testDatabase": true}'::jsonb`); err != nil {
			fatal = "写测试库标记失败: " + err.Error()
		}
	})
	if fatal != "" {
		s.Close()
		t.Fatal(fatal)
	}
}
