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
