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
	"errors"
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

// WorkerLockID 是 outbox worker 的 advisory lock 编号。
// 单实例约束靠它保证：取不到就退出，不是等待。见 ADR-0004。
//
// 导出是有意的：排查「控制台说 worker 无心跳」时，第一件事就是去库里看这把锁还在不在，
// 而 pg_locks 里一个库可能同时有好几把 advisory 锁（测试夹具也用一把来串行化），
// 不带编号过滤很容易看错行。手查用：
//
//	SELECT pid, granted FROM pg_locks
//	WHERE locktype = 'advisory' AND objid::bigint = 174527489 AND objsubid = 1;
const WorkerLockID int64 = 0x0A6E7401

// WorkerAlive 判断当前是否有 worker 实例活着。
//
// 判据就是 TryWorkerLock 那把 advisory lock 还被人持有着 —— 这比心跳表可靠：
// 锁挂在 worker 自己的那条连接上，进程被 kill、机器掉电、网络断开，
// 连接一断 PostgreSQL 就自动放锁，不需要 worker 配合写「我要死了」。
// 心跳表则要么依赖 worker 主动续期（假死时续期线程还活着，业务线程已经不跑了），
// 要么要定超时阈值（多久算死？）—— 这里两个问题都不存在。
//
// 只看**本库**里的锁：advisory lock 是按 database 隔离的，
// pg_locks 却是整个实例的视图，不过滤的话同实例另一个库里的 worker 会被算成自己的。
func (s *Store) WorkerAlive(ctx context.Context) (bool, error) {
	// classid/objid 是 oid（无符号 32 位），转成 bigint 再比，省得驱动去猜类型。
	var alive bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_locks
			WHERE locktype = 'advisory'
			  AND granted
			  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
			  AND classid::bigint = $1
			  AND objid::bigint   = $2
			  AND objsubid = 1
		)`, WorkerLockID>>32, WorkerLockID&0xFFFFFFFF).Scan(&alive)
	if err != nil {
		return false, fmt.Errorf("查 worker 存活: %w", err)
	}
	return alive, nil
}

// ErrLockTaken 表示锁在中途丢了，而且已经被另一个实例拿走 —— 本实例必须退出。
var ErrLockTaken = errors.New("单实例锁已被其它实例接管")

// WorkerLock 是 outbox worker 的单实例锁，挂在一条**专属连接**上。
//
// **为什么不能拿完就不管了**：这条连接从拿到锁那一刻起就再也不被使用，
// 于是它是整个系统里最容易被悄悄掐断的东西 —— PG 重启、`idle_session_timeout`、
// 连接池、防火墙 NAT 老化，任何一个都会把它掐掉。连接一断 PostgreSQL 立刻放锁，
// **而 worker 进程毫不知情**：业务查询走的是另一个连接池，会自动重连、继续正常干活。
//
// 后果是双向的，两边都很坏：
//   - 控制台的 `workerAlive` 从此永远是 false，横幅永远挂着一条假告警。
//     假告警和漏告警一样坏 —— 第三次「又是误报」之后，就没人再看这条横幅了。
//   - 更糟的是**锁真的空出来了**：这时起第二个 worker 会成功拿到锁，
//     两个 worker 同时扇出，per-agent 的因果顺序被打乱（ADR-0004）。
//     这是数据正确性问题，而且没有任何告警会报。
//
// 所以持锁方必须定期 Verify。
type WorkerLock struct {
	store *Store
	conn  *sql.Conn
}

// TryWorkerLock 尝试拿下 outbox worker 的单实例锁。
//
// 拿不到说明已经有一个 worker 在跑 —— 直接退出，不要等待（返回 nil, nil）。
// 主要防的是部署时新旧实例重叠的那几秒。
//
// 拿到之后必须定期 Verify，并在退出前 Release。
func (s *Store) TryWorkerLock(ctx context.Context) (*WorkerLock, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("取连接: %w", err)
	}
	ok, err := tryLockOn(ctx, conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("获取 advisory lock: %w", err)
	}
	if !ok {
		conn.Close()
		return nil, nil
	}
	return &WorkerLock{store: s, conn: conn}, nil
}

func tryLockOn(ctx context.Context, conn *sql.Conn) (bool, error) {
	var ok bool
	err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, WorkerLockID).Scan(&ok)
	return ok, err
}

// heldByThisSession 问的是「**这条连接**现在还持着锁吗」，不是「有没有人持着锁」。
// 后者用 WorkerAlive，那是给控制台看的；这里要区分「锁还在我手上」和
// 「锁在别人手上」，两者对 worker 的含义完全相反。
func heldByThisSession(ctx context.Context, conn *sql.Conn) (bool, error) {
	var held bool
	err := conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_locks
			WHERE locktype = 'advisory' AND granted AND pid = pg_backend_pid()
			  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
			  AND classid::bigint = $1 AND objid::bigint = $2 AND objsubid = 1
		)`, WorkerLockID>>32, WorkerLockID&0xFFFFFFFF).Scan(&held)
	return held, err
}

// Verify 确认锁还在自己手上，丢了就试着抢回来。
//
//   - nil        —— 还持着（或刚刚重新抢到），继续干活
//   - ErrLockTaken —— 锁已经被别人拿走，**本实例必须停下**
//   - 其它 error —— 这一轮没查清楚，调用方应当当成「暂时不确定」重试，而不是退出
//
// 只在确认自己**没有**持锁时才重新抢：advisory lock 在同一个 session 里是可重入的，
// 盲目再抢一次会让计数加一，之后一次 unlock 放不干净。
func (l *WorkerLock) Verify(ctx context.Context) error {
	held, err := heldByThisSession(ctx, l.conn)
	if err == nil && held {
		return nil
	}
	if err == nil {
		// 连接还活着，但锁没了（有人 pg_advisory_unlock_all 之类）。同一条连接上抢回来。
		ok, aerr := tryLockOn(ctx, l.conn)
		if aerr != nil {
			return fmt.Errorf("重新获取 advisory lock: %w", aerr)
		}
		if !ok {
			return ErrLockTaken
		}
		return nil
	}
	// 查不动了 —— 这条连接多半已经断了。换一条新的重抢。
	// 换连接而不是在老连接上重试，是因为老 session 一旦真的没了，
	// PostgreSQL 已经把锁放掉，这时新 session 抢得到才说明「确实没有别人」。
	l.conn.Close()
	conn, cerr := l.store.db.Conn(ctx)
	if cerr != nil {
		return fmt.Errorf("锁连接已断，重连失败: %w", cerr)
	}
	ok, aerr := tryLockOn(ctx, conn)
	if aerr != nil {
		conn.Close()
		return fmt.Errorf("锁连接已断，重抢失败: %w", aerr)
	}
	l.conn = conn
	if !ok {
		return ErrLockTaken
	}
	return nil
}

// Release 把锁还回去。退出前必须调用。
func (l *WorkerLock) Release() {
	ctx := context.WithoutCancel(context.Background())
	_, _ = l.conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, WorkerLockID)
	l.conn.Close()
}
