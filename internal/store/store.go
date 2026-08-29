// Package store 是 agent-hub 的持久化层。
//
// 这一层只做一件事：把领域层算出来的结果，按正确的事务边界落进 PostgreSQL。
// 业务判断不在这里 —— 谁该收到通知是 domain.Fanout 的事。
//
// 三条事务纪律写在各方法的注释里，它们是这个项目最容易写错、错了最难查的地方：
//  1. 发帖与 outbox 必须同事务（ADR-0004）
//  2. worker 的认领、扇出、标记完成必须同事务
//  3. 通知必须在 COMMIT 之后发
package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql 驱动
)

// Store 持有连接池。用 database/sql 而不是 pgx 原生接口，
// 是因为这个项目里事务边界必须显式可见，不能被隐式行为搅乱。
type Store struct {
	db *sql.DB
}

// Open 连上库并验证可达。
func Open(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("连接数据库: %w", err)
	}
	return &Store{db: db}, nil
}

// New 用已有的 *sql.DB 构造 Store，测试用。
func New(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Close() error { return s.db.Close() }

// DB 暴露底层连接，仅供迁移与测试使用。业务代码不要用它绕过 Store 的方法。
func (s *Store) DB() *sql.DB { return s.db }

// inTx 把 fn 包在一个事务里。fn 返回错误就回滚。
//
// 注意 fn 里不要做任何对外的副作用（发通知、调 HTTP）——
// 事务可能回滚，而副作用回滚不了。通知一律放到 Commit 之后。
func (s *Store) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // 已提交的 Rollback 是 no-op

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务: %w", err)
	}
	return nil
}

// workerLockID 是 outbox worker 的 advisory lock 编号。
// 单实例约束靠它保证：取不到就退出，不是等待。见 ADR-0004。
const workerLockID int64 = 0x0A6E7401

// TryWorkerLock 尝试拿下 outbox worker 的单实例锁。
//
// 拿不到说明已经有一个 worker 在跑 —— 直接退出，不要等待。
// 主要防的是部署时新旧实例重叠的那几秒。
//
// 返回的 release 必须在退出前调用；它会把锁还回去。
func (s *Store) TryWorkerLock(ctx context.Context) (ok bool, release func(), err error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("取连接: %w", err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, workerLockID).Scan(&ok); err != nil {
		conn.Close()
		return false, nil, fmt.Errorf("获取 advisory lock: %w", err)
	}
	if !ok {
		conn.Close()
		return false, nil, nil
	}
	return true, func() {
		_, _ = conn.ExecContext(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, workerLockID)
		conn.Close()
	}, nil
}
