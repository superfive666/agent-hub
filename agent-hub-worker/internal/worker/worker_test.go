package worker_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/superfive666/agent-hub/agent-hub-worker/internal/worker"
	"github.com/superfive666/agent-hub/internal/domain"
	"github.com/superfive666/agent-hub/internal/store"
	"github.com/superfive666/agent-hub/internal/testdb"
)

func newStore(t *testing.T) *store.Store { return testdb.New(t) }

type spy struct {
	mu sync.Mutex
	ns []store.Notification
}

func (s *spy) Notify(_ context.Context, ns []store.Notification) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ns = append(s.ns, ns...)
}

func (s *spy) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.ns)
}

func TestWorkerProcessesAndNotifies(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rover, err := s.CreateAgent(ctx, "rover", "测试", "superfive")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTodo(ctx, store.CreateTodoParams{
		New: domain.NewTodo{Title: "t", Body: "b", PrimaryAgentID: rover}, CreatedBy: "admin",
	}); err != nil {
		t.Fatal(err)
	}

	sp := &spy{}
	n, err := worker.New(s, sp, worker.Config{}, nil).RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 1 {
		t.Errorf("处理条数 = %d, want 1", n)
	}
	if sp.count() != 1 {
		t.Errorf("通知条数 = %d, want 1", sp.count())
	}
	evs, _ := s.ReadInbox(ctx, rover, 0, 10)
	if len(evs) != 1 || evs[0].Kind != domain.EventTodoAssigned {
		t.Errorf("主 agent 的 inbox = %+v, want 一条 todo.assigned", evs)
	}
}

func TestWorkerRunOnceReturnsZeroWhenIdle(t *testing.T) {
	s := newStore(t)
	n, err := worker.New(s, &spy{}, worker.Config{}, nil).RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("没活干时应当返回 0，实得 %d", n)
	}
}

// 需求：单实例。第二个 worker 拿不到锁就退出，不是等待。
func TestSecondWorkerExitsInsteadOfWaiting(t *testing.T) {
	s := newStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	first := worker.New(s, &spy{}, worker.Config{IdleInterval: 20 * time.Millisecond}, nil)
	done := make(chan error, 1)
	go func() { done <- first.Run(ctx) }()

	// 等第一个把锁拿稳。
	time.Sleep(300 * time.Millisecond)

	second := worker.New(s, &spy{}, worker.Config{}, nil)
	start := time.Now()
	err := second.Run(context.Background())
	if err != worker.ErrNotLeader {
		t.Fatalf("第二个 worker 的返回 = %v, want ErrNotLeader", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("第二个 worker 等了 %v —— 应当立刻退出而不是排队等锁", elapsed)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("第一个 worker 退出时报错: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("第一个 worker 收到取消后没有及时退出")
	}
}

// worker 跑起来之后，新发的帖子应当被自动扇出，不需要外部推一把。
func TestWorkerLoopPicksUpNewEvents(t *testing.T) {
	s := newStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rover, err := s.CreateAgent(ctx, "rover", "测试", "superfive")
	if err != nil {
		t.Fatal(err)
	}

	sp := &spy{}
	w := worker.New(s, sp, worker.Config{IdleInterval: 20 * time.Millisecond}, nil)
	go func() { _ = w.Run(ctx) }()
	time.Sleep(100 * time.Millisecond)

	if _, err := s.CreateTodo(ctx, store.CreateTodoParams{
		New: domain.NewTodo{Title: "t", Body: "b", PrimaryAgentID: rover}, CreatedBy: "admin",
	}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for sp.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if sp.count() != 1 {
		t.Errorf("worker 循环应当自动捡起新事件，通知数 = %d", sp.count())
	}
	if lag, _ := s.OutboxLagSeconds(ctx); lag != 0 {
		t.Errorf("扇出完之后 lag 应当回到 0，实得 %v", lag)
	}
}

// 需求：**给「欠着事件又失联」的 agent 重发信号。**
//
// 这是「断线重连之后自动补上」的 hub 那一半。connector 那一半是 cursor 落盘、
// 重连后续拉；但那只在「有人在拉」时成立。走 webhook 契约的端点是被动的：
// 信号在它下线那一刻发出、丢掉，之后没有第二次 —— 事件就一直躺在 inbox 里，
// 而它以为自己没事可做，**两端都不报错**。
func TestWorkerRenotifiesStalledAgents(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	id, err := st.CreateAgent(ctx, "renotify-agent", "测试", "superfive")
	if err != nil {
		t.Fatalf("建 agent: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx,
		`UPDATE agent SET status = 'active' WHERE id = $1`, string(id)); err != nil {
		t.Fatal(err)
	}
	// 欠 3 条，两小时没来拉过
	if _, err := st.DB().ExecContext(ctx, `
		INSERT INTO agent_inbox_state (agent_id, last_seq, cursor, last_pull_at)
		VALUES ($1, 5, 2, now() - interval '2 hours')`, string(id)); err != nil {
		t.Fatal(err)
	}

	g := &spy{}
	// RenotifyEvery 给得极短，好让这一轮立刻发生；purge 关掉，本用例不测它。
	w := worker.New(st, g, worker.Config{
		IdleInterval:  10 * time.Millisecond,
		RenotifyEvery: 20 * time.Millisecond,
		PurgeEvery:    -1,
	}, nil)

	runCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(runCtx) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && g.count() == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done

	if g.count() == 0 {
		t.Fatal("失联又欠着事件的 agent 必须被重发信号 —— " +
			"没有它，webhook 契约的端点下线一次就再也收不到任何信号")
	}
	// 信号里报的是 last_seq：agent 收到后按自己的 cursor 拉，增量补齐
	g.mu.Lock()
	first := g.ns[0]
	g.mu.Unlock()
	if first.AgentID != id {
		t.Errorf("重发的对象不对：%s", first.AgentID)
	}
	if first.Seq != 5 {
		t.Errorf("信号该报 last_seq=5，实际 %d", first.Seq)
	}
}

// 需求：正在正常拉取的 agent 不该被反复戳 —— 它本来就在拉，戳它只是噪音。
func TestWorkerDoesNotRenotifyBusyAgents(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	id, err := st.CreateAgent(ctx, "renotify-busy", "测试", "superfive")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx,
		`UPDATE agent SET status = 'active' WHERE id = $1`, string(id)); err != nil {
		t.Fatal(err)
	}
	// 也欠着，但刚刚才拉过
	if _, err := st.DB().ExecContext(ctx, `
		INSERT INTO agent_inbox_state (agent_id, last_seq, cursor, last_pull_at)
		VALUES ($1, 9, 1, now())`, string(id)); err != nil {
		t.Fatal(err)
	}

	g := &spy{}
	w := worker.New(st, g, worker.Config{
		IdleInterval:  10 * time.Millisecond,
		RenotifyEvery: 20 * time.Millisecond,
		PurgeEvery:    -1,
	}, nil)

	runCtx, cancel := context.WithTimeout(ctx, 600*time.Millisecond)
	done := make(chan error, 1)
	go func() { done <- w.Run(runCtx) }()
	time.Sleep(400 * time.Millisecond)
	cancel()
	<-done

	if g.count() != 0 {
		t.Errorf("窗口内刚拉过的 agent 不该被戳，实际发了 %d 条", g.count())
	}
}
