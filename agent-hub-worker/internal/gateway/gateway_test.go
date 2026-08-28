package gateway_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/superfive666/agent-hub/agent-hub-worker/internal/gateway"
	"github.com/superfive666/agent-hub/internal/store"
)

// 需求：通知只是信号。一个连上了但不响应的端点，不能把其他 agent 的通知拖住。
func TestWebhookSlowEndpointDoesNotBlockOthers(t *testing.T) {
	t.Parallel()

	var fastHits int32
	block := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block // 连上了，但永不响应
	}))
	defer slow.Close()
	defer close(block)

	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fastHits, 1)
	}))
	defer fast.Close()

	gw := &gateway.Webhook{
		Timeout: 5 * time.Second,
		Lookup: func(_ context.Context, agent string) (string, error) {
			if agent == "slow" {
				return slow.URL, nil
			}
			return fast.URL, nil
		},
	}

	start := time.Now()
	gw.Notify(context.Background(), []store.Notification{
		{AgentID: "slow", Seq: 1},
		{AgentID: "fast", Seq: 1},
	})
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Notify 被慢端点阻塞了 %v —— 投递必须是非阻塞的", elapsed)
	}

	deadline := time.Now().Add(3 * time.Second)
	for atomic.LoadInt32(&fastHits) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&fastHits) != 1 {
		t.Errorf("快端点应当照常收到通知，实得 %d 次", fastHits)
	}
}

// 需求：每个 agent 一个有界待发槽，满了直接丢。丢信号是安全的 ——
// agent 下次拉 inbox 会自然补齐，为了不丢一条而阻塞所有人才是真的亏。
func TestWebhookDropsWhenSlotFull(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer srv.Close()
	defer close(block)

	gw := &gateway.Webhook{
		Slots:   1,
		Timeout: 5 * time.Second,
		Lookup:  func(context.Context, string) (string, error) { return srv.URL, nil },
	}

	for i := 0; i < 6; i++ {
		gw.Notify(context.Background(), []store.Notification{{AgentID: "stuck", Seq: int64(i)}})
	}
	// 第一条占住唯一的槽，其余五条应当被丢弃而不是排队等待。
	deadline := time.Now().Add(2 * time.Second)
	for gw.Dropped() < 5 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := gw.Dropped(); got != 5 {
		t.Errorf("丢弃计数 = %d, want 5", got)
	}
}

// 没配 webhook 的 agent 直接跳过，不该报错也不该发请求。
func TestWebhookSkipsAgentsWithoutEndpoint(t *testing.T) {
	t.Parallel()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	gw := &gateway.Webhook{
		Lookup: func(_ context.Context, agent string) (string, error) {
			if agent == "has-webhook" {
				return srv.URL, nil
			}
			return "", nil
		},
	}
	gw.Notify(context.Background(), []store.Notification{
		{AgentID: "no-webhook", Seq: 1},
		{AgentID: "has-webhook", Seq: 1},
	})
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&hits) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("只有配了 webhook 的 agent 该收到请求，实得 %d 次", hits)
	}
}

// 投递失败不重试：agent 下次拉 inbox 时会自然补齐。
func TestWebhookDoesNotRetryOnFailure(t *testing.T) {
	t.Parallel()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	gw := &gateway.Webhook{Lookup: func(context.Context, string) (string, error) { return srv.URL, nil }}
	gw.Notify(context.Background(), []store.Notification{{AgentID: "a", Seq: 1}})

	time.Sleep(400 * time.Millisecond)
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("失败后请求次数 = %d, want 1（不该重试）", got)
	}
}

type recorder struct {
	mu sync.Mutex
	ns []store.Notification
}

func (r *recorder) Notify(_ context.Context, ns []store.Notification) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ns = append(r.ns, ns...)
}

func TestMultiFansOutToAll(t *testing.T) {
	t.Parallel()
	a, b := &recorder{}, &recorder{}
	gateway.Multi{a, b}.Notify(context.Background(), []store.Notification{{AgentID: "x", Seq: 7}})
	for name, r := range map[string]*recorder{"第一个出口": a, "第二个出口": b} {
		if len(r.ns) != 1 || r.ns[0].Seq != 7 {
			t.Errorf("%s 收到 %+v, want 一条 seq=7", name, r.ns)
		}
	}
}
