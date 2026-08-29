// Package testdb 给需要真 PostgreSQL 的测试提供一个干净的库。
//
// 为什么要有它：SKIP LOCKED、advisory lock、事务隔离的行为 mock 不出来，
// 而它们正是 outbox 方案的地基 —— 这些必须在真库上测。
// 但 go test 会并行跑不同的包，几个包同时 TRUNCATE 同一个库会互相撞成死锁，
// 所以这里用一把会话级 advisory lock 把它们串起来。
package testdb

import (
	"context"
	"os"
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
