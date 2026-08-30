package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/superfive666/agent-hub/internal/domain"
	"github.com/superfive666/agent-hub/internal/store"
	"github.com/superfive666/agent-hub/internal/testdb"
)

// 这些用例必须跑在真 PostgreSQL 上。SKIP LOCKED、advisory lock、事务隔离的行为
// mock 不出来，而它们正是 outbox 方案的地基 —— 用 mock 测等于什么都没测。
//
//	make dev-db && make test-db
func newStore(t *testing.T) *store.Store { return testdb.New(t) }

func mkAgent(t *testing.T, s *store.Store, name string) domain.AgentID {
	t.Helper()
	id, err := s.CreateAgent(context.Background(), name, "测试用 agent", "superfive")
	if err != nil {
		t.Fatalf("创建 agent %s: %v", name, err)
	}
	return id
}

func countRows(t *testing.T, s *store.Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.DB().QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("计数失败 (%s): %v", query, err)
	}
	return n
}

// 需求：发帖与 outbox 必须同事务 —— 帖子发成功了，通知就一定会到。
func TestCreateTodoWritesPostAndOutboxTogether(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover := mkAgent(t, s, "rover")

	res, err := s.CreateTodo(ctx, store.CreateTodoParams{
		New:       domain.NewTodo{Title: "改退避", Body: "改成指数退避", PrimaryAgentID: rover},
		CreatedBy: "admin",
	})
	if err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}
	if res.ThreadID == "" || res.PostID == "" || res.StartedAt.IsZero() {
		t.Fatalf("返回值不完整: %+v", res)
	}
	if got := countRows(t, s, `SELECT count(*) FROM post WHERE thread_id=$1`, res.ThreadID); got != 1 {
		t.Errorf("post 行数 = %d, want 1", got)
	}
	if got := countRows(t, s, `SELECT count(*) FROM outbox_event WHERE status='pending'`); got != 1 {
		t.Errorf("待扇出 outbox 行数 = %d, want 1", got)
	}
}

// 需求：主 agent 必选。不给就拒绝，而且什么都不能写进去。
func TestCreateTodoRejectsMissingPrimaryAgent(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	_, err := s.CreateTodo(ctx, store.CreateTodoParams{
		New: domain.NewTodo{Title: "改退避", Body: "正文"},
	})
	if err == nil {
		t.Fatal("没有主 agent 时应当拒绝创建")
	}
	if got := countRows(t, s, `SELECT count(*) FROM thread`); got != 0 {
		t.Errorf("被拒绝后仍写入了 %d 条 thread", got)
	}
}

// 需求：主 agent 拿到 todo.assigned 进队列；被 @ 的只拿到 mentioned；作者不收自己的。
func TestFanoutRoutesByRole(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover, nova := mkAgent(t, s, "rover"), mkAgent(t, s, "nova")

	if _, err := s.CreateTodo(ctx, store.CreateTodoParams{
		New: domain.NewTodo{
			Title: "改退避", Body: "@nova 看下", PrimaryAgentID: rover,
			Mentions: []domain.AgentID{nova},
		},
		CreatedBy: "admin",
	}); err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}
	if _, err := s.ProcessOutboxBatch(ctx, 10, nil); err != nil {
		t.Fatalf("ProcessOutboxBatch: %v", err)
	}

	for _, tc := range []struct {
		agent domain.AgentID
		name  string
		want  domain.EventKind
	}{
		{rover, "主 agent", domain.EventTodoAssigned},
		{nova, "被 @ 的", domain.EventTodoMentioned},
	} {
		evs, err := s.ReadInbox(ctx, tc.agent, 0, 50)
		if err != nil {
			t.Fatalf("ReadInbox(%s): %v", tc.name, err)
		}
		if len(evs) != 1 {
			t.Fatalf("%s 收到 %d 条事件, want 1（%v）", tc.name, len(evs), evs)
		}
		if evs[0].Kind != tc.want {
			t.Errorf("%s 收到 %s, want %s", tc.name, evs[0].Kind, tc.want)
		}
		if evs[0].Priority != tc.want.Priority() {
			t.Errorf("%s 的 priority = %d, want %d", tc.name, evs[0].Priority, tc.want.Priority())
		}
	}
}

// 需求：作者不收自己的通知。
func TestAuthorDoesNotNotifySelf(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover, nova := mkAgent(t, s, "rover"), mkAgent(t, s, "nova")

	res, err := s.CreateTodo(ctx, store.CreateTodoParams{
		New:       domain.NewTodo{Title: "t", Body: "b", PrimaryAgentID: rover, Mentions: []domain.AgentID{nova}},
		CreatedBy: "admin",
	})
	if err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}
	if _, err := s.ProcessOutboxBatch(ctx, 10, nil); err != nil {
		t.Fatal(err)
	}
	before, _ := s.ReadInbox(ctx, rover, 0, 50)

	if _, err := s.AppendPost(ctx, store.AppendPostParams{
		ThreadID: res.ThreadID, AuthorKind: "agent", AuthorID: rover, Body: "我来看看",
	}); err != nil {
		t.Fatalf("AppendPost: %v", err)
	}
	if _, err := s.ProcessOutboxBatch(ctx, 10, nil); err != nil {
		t.Fatal(err)
	}

	after, _ := s.ReadInbox(ctx, rover, 0, 50)
	if len(after) != len(before) {
		t.Errorf("作者收到了自己发言的通知：之前 %d 条，之后 %d 条", len(before), len(after))
	}
	novaEvents, _ := s.ReadInbox(ctx, nova, 0, 50)
	if len(novaEvents) != 2 {
		t.Errorf("关注者应当收到这条回复，共 2 条事件，实得 %d", len(novaEvents))
	}
}

// 需求：一条 post 里 @ 同一个 agent 两次，只算一次。
func TestDuplicateMentionCountsOnce(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover, nova := mkAgent(t, s, "rover"), mkAgent(t, s, "nova")

	res, err := s.CreateTodo(ctx, store.CreateTodoParams{
		New: domain.NewTodo{Title: "t", Body: "b", PrimaryAgentID: rover,
			Mentions: []domain.AgentID{nova, nova, nova}},
		CreatedBy: "admin",
	})
	if err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}
	if got := countRows(t, s, `SELECT count(*) FROM mention WHERE post_id=$1`, res.PostID); got != 1 {
		t.Errorf("mention 行数 = %d, want 1", got)
	}
	if _, err := s.ProcessOutboxBatch(ctx, 10, nil); err != nil {
		t.Fatal(err)
	}
	evs, _ := s.ReadInbox(ctx, nova, 0, 50)
	if len(evs) != 1 {
		t.Errorf("被 @ 三次应当只收到 1 条通知，实得 %d", len(evs))
	}
}

// 需求：主 agent 同时被 @ 时只产生一条事件，取优先级最高的那个。
func TestPrimaryAgentMentionedStillGetsOneEvent(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover := mkAgent(t, s, "rover")

	if _, err := s.CreateTodo(ctx, store.CreateTodoParams{
		New: domain.NewTodo{Title: "t", Body: "@rover", PrimaryAgentID: rover,
			Mentions: []domain.AgentID{rover}},
		CreatedBy: "admin",
	}); err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}
	if _, err := s.ProcessOutboxBatch(ctx, 10, nil); err != nil {
		t.Fatal(err)
	}
	evs, _ := s.ReadInbox(ctx, rover, 0, 50)
	if len(evs) != 1 {
		t.Fatalf("应当只有 1 条事件，实得 %d（%v）", len(evs), evs)
	}
	if evs[0].Kind != domain.EventTodoAssigned {
		t.Errorf("应当取优先级最高的 todo.assigned，实得 %s", evs[0].Kind)
	}
}

// **纪律 ③ 的专用用例**：通知必须在事务提交之后发。
//
// 如果谁把 notify 挪进事务里，这个用例会挂 —— 回调里用另一条连接查 inbox，
// 未提交的数据是看不见的。这个坑在低负载下几乎不出现，只能靠用例守着。
func TestNotifyFiresAfterCommit(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover := mkAgent(t, s, "rover")

	if _, err := s.CreateTodo(ctx, store.CreateTodoParams{
		New: domain.NewTodo{Title: "t", Body: "b", PrimaryAgentID: rover}, CreatedBy: "admin",
	}); err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}

	var seenInNotify int
	var notified []store.Notification
	_, err := s.ProcessOutboxBatch(ctx, 10, func(ctx context.Context, ns []store.Notification) {
		notified = ns
		// 回调此刻走的是连接池里的另一条连接：拉得到，说明事务已经提交了。
		evs, err := s.ReadInbox(ctx, rover, 0, 50)
		if err != nil {
			t.Errorf("回调里读 inbox: %v", err)
		}
		seenInNotify = len(evs)
	})
	if err != nil {
		t.Fatalf("ProcessOutboxBatch: %v", err)
	}
	if seenInNotify != 1 {
		t.Errorf("通知回调里应当已经能拉到 1 条事件，实得 %d —— 说明通知发早了，事务还没提交", seenInNotify)
	}
	if len(notified) != 1 || notified[0].AgentID != rover || notified[0].Seq != 1 {
		t.Errorf("通知内容不对: %+v", notified)
	}
}

// 需求：断线十分钟后重连，期间的事件按 cursor 一条不少地补齐。
func TestCursorIncrementalCatchUp(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover, nova := mkAgent(t, s, "rover"), mkAgent(t, s, "nova")

	res, err := s.CreateTodo(ctx, store.CreateTodoParams{
		New: domain.NewTodo{Title: "t", Body: "b", PrimaryAgentID: rover}, CreatedBy: "admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	// rover 「离线」期间，nova 连发 5 条。
	for i := 0; i < 5; i++ {
		if _, err := s.AppendPost(ctx, store.AppendPostParams{
			ThreadID: res.ThreadID, AuthorKind: "agent", AuthorID: nova, Body: "补充",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.ProcessOutboxBatch(ctx, 100, nil); err != nil {
		t.Fatal(err)
	}

	all, err := s.ReadInbox(ctx, rover, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 6 {
		t.Fatalf("离线期间的事件应当全部补齐（1 条指派 + 5 条回复），实得 %d", len(all))
	}
	for i, e := range all {
		if e.Seq != int64(i+1) {
			t.Errorf("seq 不连续：第 %d 条的 seq = %d", i, e.Seq)
		}
	}
	// 从中间接着拉，不重不漏。
	rest, err := s.ReadInbox(ctx, rover, all[2].Seq, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest) != 3 {
		t.Errorf("从 seq=%d 之后应当还剩 3 条，实得 %d", all[2].Seq, len(rest))
	}
}

// 需求：cursor 只许前进，迟到的 ack 不能把它拽回去重放已处理的事件。
func TestAckCursorOnlyMovesForward(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover := mkAgent(t, s, "rover")

	for _, c := range []int64{5, 9, 3} {
		if err := s.AckCursor(ctx, rover, c); err != nil {
			t.Fatalf("AckCursor(%d): %v", c, err)
		}
	}
	got, err := s.Cursor(ctx, rover)
	if err != nil {
		t.Fatal(err)
	}
	if got != 9 {
		t.Errorf("cursor = %d, want 9（迟到的 ack=3 不应把它拽回去）", got)
	}
}

// SKIP LOCKED 的行为 mock 不出来：两个 worker 并发认领，同一条事件不能被处理两次。
func TestConcurrentWorkersDoNotDoubleProcess(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover := mkAgent(t, s, "rover")

	const n = 12
	for i := 0; i < n; i++ {
		if _, err := s.CreateTodo(ctx, store.CreateTodoParams{
			New: domain.NewTodo{Title: "t", Body: "b", PrimaryAgentID: rover}, CreatedBy: "admin",
		}); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	total := 0
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				got, err := s.ProcessOutboxBatch(ctx, 3, nil)
				if err != nil {
					t.Errorf("worker 出错: %v", err)
					return
				}
				if got == 0 {
					return
				}
				mu.Lock()
				total += got
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if total != n {
		t.Errorf("处理总数 = %d, want %d（有事件被处理了多次或漏了）", total, n)
	}
	if got := countRows(t, s, `SELECT count(*) FROM inbox_event WHERE agent_id=$1`, string(rover)); got != n {
		t.Errorf("inbox 事件数 = %d, want %d", got, n)
	}
	if got := countRows(t, s, `SELECT count(*) FROM outbox_event WHERE status='pending'`); got != 0 {
		t.Errorf("仍有 %d 条待扇出", got)
	}
}

// 需求：worker 单实例。第二个拿不到锁就该退出，不是等待。
func TestWorkerLockIsSingleInstance(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	first, err := s.TryWorkerLock(ctx)
	if err != nil {
		t.Fatalf("TryWorkerLock: %v", err)
	}
	if first == nil {
		t.Fatal("第一个 worker 应当拿到锁")
	}

	second, err := s.TryWorkerLock(ctx)
	if err != nil {
		t.Fatalf("第二次 TryWorkerLock: %v", err)
	}
	if second != nil {
		second.Release()
		first.Release()
		t.Fatal("第二个 worker 不该拿到锁 —— 单实例约束失效了")
	}

	first.Release()
	third, err := s.TryWorkerLock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if third == nil {
		t.Error("释放之后应当能重新拿到锁")
	} else {
		third.Release()
	}
}

// killLockBackend 掐掉正持着单实例锁的那条后端连接。
// 用它造「连接被中间设备悄悄掐断」的现场 —— 线上就是这么坏的，
// 不是有人调了 unlock，而是连接没了、PG 顺手放锁、进程毫不知情。
func killLockBackend(t *testing.T, s *store.Store) {
	t.Helper()
	var pid int
	// **必须按编号过滤。** 同一个库里不止一把 advisory 锁 —— testdb 夹具自己就用一把
	// 来串行化用例。不带编号 LIMIT 1 会掐到夹具那条连接上，这条用例就在测别的东西了。
	err := s.DB().QueryRow(`
		SELECT pid FROM pg_locks
		WHERE locktype = 'advisory' AND granted
		  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
		  AND objid::bigint = $1 AND objsubid = 1`,
		store.WorkerLockID&0xFFFFFFFF).Scan(&pid)
	if err != nil {
		t.Fatalf("找不到持锁的连接: %v", err)
	}
	if _, err := s.DB().Exec(`SELECT pg_terminate_backend($1)`, pid); err != nil {
		t.Fatalf("掐连接失败: %v", err)
	}
	// pg_terminate_backend 是异步的，等它真的退出，否则锁可能还挂着
	for i := 0; i < 100; i++ {
		var still bool
		if err := s.DB().QueryRow(
			`SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE pid = $1)`, pid).Scan(&still); err != nil {
			t.Fatal(err)
		}
		if !still {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("等了 2s 持锁连接还没退出")
}

// 需求：**持锁方要能自己发现锁掉了。**
//
// 这条用例来自一次真实故障：worker 容器 Up 4 小时、日志只有一行「worker 启动」，
// 而 `pg_locks` 里一条 advisory 锁都没有。持锁的那条连接从启动起就再没被用过，
// 被中间设备悄悄掐断之后 PostgreSQL 放了锁，进程却毫不知情 ——
// 控制台从此永远显示「worker 无心跳」，同时锁真的空了出来。
func TestWorkerLockNoticesItLostTheLock(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	lock, err := s.TryWorkerLock(ctx)
	if err != nil || lock == nil {
		t.Fatalf("拿锁失败: %v", err)
	}
	defer lock.Release()

	// 正常情况下 Verify 什么都不该做
	if err := lock.Verify(ctx); err != nil {
		t.Fatalf("持锁期间 Verify 不该报错: %v", err)
	}

	// 从外面把持锁的那条连接掐掉 —— 这正是线上发生的事，
	// 不是「优雅地 unlock」而是「连接没了，PG 顺手放了锁，进程不知道」。
	killLockBackend(t, s)
	if alive, err := s.WorkerAlive(ctx); err != nil {
		t.Fatal(err)
	} else if alive {
		t.Fatal("造现场失败：锁应当已经不在了")
	}

	if err := lock.Verify(ctx); err != nil {
		t.Fatalf("锁空着的时候 Verify 应当抢回来，而不是报错: %v", err)
	}
	if alive, err := s.WorkerAlive(ctx); err != nil {
		t.Fatal(err)
	} else if !alive {
		t.Error("Verify 之后锁应当回到手上 —— 否则控制台的假告警永远不会消失")
	}
}

// 需求：锁掉了**而且被别人拿走**时，Verify 必须报 ErrLockTaken 让本实例停下。
// 两个 worker 同时扇出会打乱 per-agent 的因果顺序（ADR-0004），
// 那是数据正确性问题，而且没有任何告警会报。
func TestWorkerLockStandsDownWhenAnotherInstanceTookIt(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	lock, err := s.TryWorkerLock(ctx)
	if err != nil || lock == nil {
		t.Fatalf("拿锁失败: %v", err)
	}
	defer lock.Release()

	// 锁从我这条连接上掉了，随后被另一条连接（另一个实例）拿走
	killLockBackend(t, s)
	other, err := s.TryWorkerLock(ctx)
	if err != nil || other == nil {
		t.Fatalf("另一个实例应当能拿到空着的锁: %v", err)
	}
	defer other.Release()

	if err := lock.Verify(ctx); !errors.Is(err, store.ErrLockTaken) {
		t.Errorf("锁被别人拿走时应当报 ErrLockTaken，实际 %v", err)
	}
}

// 需求：控制台要能分辨 worker 是死是活，判据是那把单实例锁还在不在。
//
// 用锁而不是心跳表：锁挂在 worker 自己那条连接上，进程被 kill、机器掉电、
// 网络断开，连接一断 PostgreSQL 就自动放锁 —— 不需要 worker 配合上报。
func TestWorkerAliveFollowsTheSingleInstanceLock(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	alive, err := s.WorkerAlive(ctx)
	if err != nil {
		t.Fatalf("WorkerAlive: %v", err)
	}
	if alive {
		t.Fatal("没有 worker 持锁时 WorkerAlive 应当是 false")
	}

	lock, err := s.TryWorkerLock(ctx)
	if err != nil || lock == nil {
		t.Fatalf("拿单实例锁失败: %v", err)
	}
	if alive, err = s.WorkerAlive(ctx); err != nil {
		t.Fatal(err)
	} else if !alive {
		t.Error("worker 持锁期间 WorkerAlive 应当是 true —— 否则控制台会挂一条假告警")
	}

	// 放锁之后必须立刻翻回 false，不能有粘滞：worker 挂了就得马上看得出来。
	lock.Release()
	if alive, err = s.WorkerAlive(ctx); err != nil {
		t.Fatal(err)
	} else if alive {
		t.Error("worker 放锁之后 WorkerAlive 应当立刻回到 false")
	}
}

// 需求：outboxPending 是「积了多少」，和 outbox_lag 的「最老的等了多久」是一对。
// worker 刚挂的头一秒滞后还是 0，条数已经在涨了。
func TestOutboxPendingCount(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover := mkAgent(t, s, "rover")

	n, err := s.OutboxPendingCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("没有待扇出事件时应当是 0，实得 %d", n)
	}

	for range 2 {
		if _, err := s.CreateTodo(ctx, store.CreateTodoParams{
			New: domain.NewTodo{Title: "t", Body: "b", PrimaryAgentID: rover}, CreatedBy: "admin",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err = s.OutboxPendingCount(ctx); err != nil {
		t.Fatal(err)
	} else if n != 2 {
		t.Errorf("建了两条 todo，待扇出应当是 2，实得 %d", n)
	}

	if _, err := s.ProcessOutboxBatch(ctx, 10, nil); err != nil {
		t.Fatal(err)
	}
	if n, err = s.OutboxPendingCount(ctx); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Errorf("扇出完之后应当回到 0，实得 %d", n)
	}
}

// outbox_lag 是唯一能发现 worker 静默死亡的指标，它本身必须是对的。
func TestOutboxLag(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover := mkAgent(t, s, "rover")

	lag, err := s.OutboxLagSeconds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if lag != 0 {
		t.Errorf("没有待扇出事件时 lag 应当是 0，实得 %v", lag)
	}

	if _, err := s.CreateTodo(ctx, store.CreateTodoParams{
		New: domain.NewTodo{Title: "t", Body: "b", PrimaryAgentID: rover}, CreatedBy: "admin",
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	lag, err = s.OutboxLagSeconds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if lag < 1 {
		t.Errorf("有事件积压一秒后 lag 应当 >= 1，实得 %v", lag)
	}

	if _, err := s.ProcessOutboxBatch(ctx, 10, nil); err != nil {
		t.Fatal(err)
	}
	if lag, _ = s.OutboxLagSeconds(ctx); lag != 0 {
		t.Errorf("扇出完之后 lag 应当回到 0，实得 %v", lag)
	}
}

// 标签是用户输入。带引号、反斜杠、逗号的标签不能破坏 SQL，
// 也不能被悄悄截断 —— 手拼 text[] 字面量正是在这里出事。
func TestTagsSurviveHostileCharacters(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover := mkAgent(t, s, "rover")

	nasty := []string{`he said "hi"`, `back\slash`, `a,b`, `{braces}`, `双引号"里面`}
	res, err := s.CreateTodo(ctx, store.CreateTodoParams{
		New:       domain.NewTodo{Title: "t", Body: "b", PrimaryAgentID: rover},
		CreatedBy: "admin", Tags: nasty,
	})
	if err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}

	var got []string
	if err := s.DB().QueryRowContext(ctx,
		`SELECT array_to_json(tags)::text FROM todo WHERE thread_id=$1`, res.ThreadID,
	).Scan(new(string)); err != nil {
		t.Fatalf("读回标签: %v", err)
	}
	rows, err := s.DB().QueryContext(ctx,
		`SELECT unnest(tags) FROM todo WHERE thread_id=$1`, res.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		got = append(got, v)
	}
	if len(got) != len(nasty) {
		t.Fatalf("标签数量 = %d, want %d（%q）", len(got), len(nasty), got)
	}
	for i := range nasty {
		if got[i] != nasty[i] {
			t.Errorf("第 %d 个标签 = %q, want %q", i, got[i], nasty[i])
		}
	}
}

// 空标签不该写成 NULL，也不该写成 {""}。
func TestEmptyTagsBecomeEmptyArray(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover := mkAgent(t, s, "rover")

	res, err := s.CreateTodo(ctx, store.CreateTodoParams{
		New: domain.NewTodo{Title: "t", Body: "b", PrimaryAgentID: rover}, CreatedBy: "admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.DB().QueryRowContext(ctx,
		`SELECT coalesce(array_length(tags,1),0) FROM todo WHERE thread_id=$1`, res.ThreadID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("空标签长度 = %d, want 0", n)
	}
}

// 需求：**「按活动」口径下，一条 thread 这一天只出一行。**
//
// 来自实际使用中的反馈：一条广播底下你回一句、它回一句，看板上就冒出三条「广播」，
// 看的人会以为今天发了三条广播，而实际上是一条广播加两句对话。
func TestBoardActivityCollapsesOneThreadIntoOneRow(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	author := mkAgent(t, s, "board-author")
	other := mkAgent(t, s, "board-other")

	threadID, err := s.CreateTweet(ctx, store.CreateTweetParams{Author: author, Body: "一条广播"})
	if err != nil {
		t.Fatalf("发广播: %v", err)
	}
	for _, body := range []string{"第一句回复", "第二句回复"} {
		if _, err := s.AppendPost(ctx, store.AppendPostParams{
			ThreadID: threadID, AuthorKind: "agent", AuthorID: other, Body: body,
		}); err != nil {
			t.Fatalf("回帖: %v", err)
		}
	}

	items, err := s.Board(ctx, time.Now(), "activity", time.UTC)
	if err != nil {
		t.Fatalf("查看板: %v", err)
	}
	var mine []store.BoardItem
	for _, it := range items {
		if it.ThreadID == threadID {
			mine = append(mine, it)
		}
	}
	if len(mine) != 1 {
		t.Fatalf("一条广播加两句回复应当只出 1 行，实际 %d 行 —— 看的人会以为今天发了 %d 条广播",
			len(mine), len(mine))
	}
	if mine[0].ReplyCount != 3 {
		t.Errorf("这一天这条 thread 一共 3 条发言，replyCount = %d", mine[0].ReplyCount)
	}
	// **摘要是 thread 自己的主题，不是最后一条发言。**
	// 挂上「第二句回复」的话，读起来就成了这条广播本身在说「第二句回复」——
	// 看板这一行要回答的是「今天哪条事有动静」，那就得写得出是哪条事。
	if mine[0].Summary != "一条广播" {
		t.Errorf("摘要应当是广播自己的主题，实际 %q", mine[0].Summary)
	}
}

// 需求：todo 在看板上也用自己的标题，不是最后一条回复。
// 和广播那条是同一个道理，但走的是另一个字段（todo.title 而不是 tweet.body），
// 所以单独钉一条 —— 只测广播的话，todo 那半边坏了没人知道。
func TestBoardActivityUsesTodoTitleNotLastReply(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	primary := mkAgent(t, s, "board-primary")

	res, err := s.CreateTodo(ctx, store.CreateTodoParams{
		New: domain.NewTodo{
			Title: "把 outbox lag 接到告警上", Body: "正文", PrimaryAgentID: primary,
		},
		CreatedBy: "admin",
	})
	if err != nil {
		t.Fatalf("建 todo: %v", err)
	}
	if _, err := s.AppendPost(ctx, store.AppendPostParams{
		ThreadID: res.ThreadID, AuthorKind: "agent", AuthorID: primary, Body: "收到，我看一下",
	}); err != nil {
		t.Fatalf("回帖: %v", err)
	}

	items, err := s.Board(ctx, time.Now(), "activity", time.UTC)
	if err != nil {
		t.Fatalf("查看板: %v", err)
	}
	for _, it := range items {
		if it.ThreadID != res.ThreadID {
			continue
		}
		if it.Summary != "把 outbox lag 接到告警上" {
			t.Errorf("todo 应当显示自己的标题，实际 %q", it.Summary)
		}
		if it.ReplyCount != 2 {
			t.Errorf("当天两条发言，replyCount = %d", it.ReplyCount)
		}
		return
	}
	t.Fatal("看板里没有这条 todo")
}

// 需求：广播的回复只通知发起人和被 @ 的人 —— 这条要在真库上端到端成立，
// 不只是 domain.Fanout 的单元测试。老关注者留在 thread_watcher 里，但不再收到通知。
func TestTweetReplyOnlyNotifiesAuthorAndMentioned(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	author := mkAgent(t, s, "tw-author")
	chatter := mkAgent(t, s, "tw-chatter") // 早先说过话，是老关注者
	pulled := mkAgent(t, s, "tw-pulled")   // 这条回复 @ 到的人
	replier := mkAgent(t, s, "tw-replier")

	threadID, err := s.CreateTweet(ctx, store.CreateTweetParams{Author: author, Body: "广播正文"})
	if err != nil {
		t.Fatalf("发广播: %v", err)
	}
	if _, err := s.AppendPost(ctx, store.AppendPostParams{
		ThreadID: threadID, AuthorKind: "agent", AuthorID: chatter, Body: "我先说一句",
	}); err != nil {
		t.Fatalf("chatter 回帖: %v", err)
	}
	if _, err := s.AppendPost(ctx, store.AppendPostParams{
		ThreadID: threadID, AuthorKind: "agent", AuthorID: replier,
		Body: "@tw-pulled 你看下", Mentions: []domain.AgentID{pulled},
	}); err != nil {
		t.Fatalf("replier 回帖: %v", err)
	}

	// chatter 确实是关注者 —— 关注关系照常记录，只是不再产生通知
	if n := countRows(t, s,
		`SELECT count(*) FROM thread_watcher WHERE thread_id = $1 AND agent_id = $2`,
		threadID, string(chatter)); n != 1 {
		t.Fatalf("chatter 应当在关注者里（详情页要用），实际 %d 行", n)
	}

	if _, err := s.ProcessOutboxBatch(ctx, 100, nil); err != nil {
		t.Fatalf("扇出: %v", err)
	}

	got := func(id domain.AgentID) int {
		return countRows(t, s,
			`SELECT count(*) FROM inbox_event WHERE thread_id = $1 AND agent_id = $2
			   AND kind IN ('tweet.replied','tweet.mentioned')`, threadID, string(id))
	}
	if got(author) == 0 {
		t.Error("发起人应当收到回复通知 —— 这是它自己的广播")
	}
	if got(pulled) == 0 {
		t.Error("被 @ 的人应当收到通知 —— @ 是平台上唯一的连接动作")
	}
	if n := got(chatter); n != 0 {
		t.Errorf("老关注者不该再被叫醒（实际 %d 条）—— 否则一条广播底下每多一个人说话，"+
			"后面每条回复就多吵醒一个人", n)
	}
}

// 需求：**说过话的 agent 删不掉。**
//
// post.author_id 上没有外键（admin 发帖时它是 NULL，加不了 NOT NULL 的引用），
// 所以删掉 agent 之后它的 post 会变成孤儿 —— 而读 thread 的查询是
// `coalesce(a.name, 'superfive')`，那些帖子会**挂到人类头上**。
// 设计语言 §1.1 的第一条就是「人和 agent 必须一眼分得开」，
// 一条 agent 说过的话变成人说的，比留一条停用记录严重得多。
//
// 这条以前是漏的：计数查询抄了两份，一份数了 post 一份没数。
func TestAgentThatSpokeCannotBeDeleted(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	author := mkAgent(t, s, "spoke-author")
	replier := mkAgent(t, s, "spoke-replier")

	threadID, err := s.CreateTweet(ctx, store.CreateTweetParams{Author: author, Body: "广播"})
	if err != nil {
		t.Fatalf("发广播: %v", err)
	}
	// replier 只是在别人的 thread 里回了一句：没有 todo、没有 tweet、没有 step。
	if _, err := s.AppendPost(ctx, store.AppendPostParams{
		ThreadID: threadID, AuthorKind: "agent", AuthorID: replier, Body: "我说一句",
	}); err != nil {
		t.Fatalf("回帖: %v", err)
	}

	refs, err := s.CountAgentRefs(ctx, replier)
	if err != nil {
		t.Fatalf("数留痕: %v", err)
	}
	if refs.Posts != 1 {
		t.Errorf("它说过 1 句话，Posts = %d", refs.Posts)
	}

	// 关键：CountAgentRefs 说「有留痕」，DeleteAgent 就必须也拒绝。
	// 两处判据不一致的话，界面说删不了、真去删却删成功了。
	if _, err := s.DeleteAgent(ctx, replier); !errors.Is(err, store.ErrAgentInUse) {
		t.Fatalf("说过话的 agent 应当删不掉（ErrAgentInUse），实际 %v", err)
	}

	// 一句话都没说过的才删得掉
	silent := mkAgent(t, s, "spoke-silent")
	if _, err := s.DeleteAgent(ctx, silent); err != nil {
		t.Errorf("没有任何留痕的 agent 应当删得掉，实际 %v", err)
	}
}
