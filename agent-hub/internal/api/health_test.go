package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/superfive666/agent-hub/internal/domain"
	"github.com/superfive666/agent-hub/internal/store"
)

// healthOf 拉一次 /api/admin/health，同时把原始 JSON 交回来 ——
// 有些用例要断言的是「这个 key 到底在不在」，反序列化之后就分不清
// 「返回了 false」和「压根没返回」了，而这两者正是本文件要区分的东西。
func healthOf(t *testing.T, c *http.Client, base string) (map[string]any, []byte) {
	t.Helper()
	resp, body := doJSON(t, c, http.MethodGet, base+"/api/admin/health", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("查运行状态失败: %d %s", resp.StatusCode, body)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("运行状态不是合法 JSON: %v (%s)", err, body)
	}
	return raw, body
}

// 需求（openapi.yaml /api/admin/health）：五个字段全是必填。
//
// 这条用例防的是一个真实发生过的故障：接口只实现了 outboxLagSeconds 和 outboxDead，
// 另外三个字段从来没返回过。控制台对缺失字段的兜底是「读不到就当坏了」，
// 于是横幅永远显示「worker 无心跳」——worker 其实活得好好的。
// 少一个字段不会退化成「显示未知」，会退化成一条永远挂着的假告警。
func TestHealthReturnsEveryFieldInTheContract(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)

	raw, body := healthOf(t, c, srv.URL)
	for _, k := range []string{
		"outboxLagSeconds", "outboxPending", "outboxDead", "workerAlive", "pendingLongPolls",
	} {
		if _, ok := raw[k]; !ok {
			t.Errorf("运行状态缺字段 %q —— 契约里它是必填的。实得 %s", k, body)
		}
	}
}

// 需求：没有 worker 在跑时，workerAlive 必须是 false —— 这条告警要真的能报出来。
func TestHealthReportsWorkerDeadWhenNoWorkerHoldsTheLock(t *testing.T) {
	srv, _ := newServer(t)
	c := adminClient(t, srv.URL)

	raw, _ := healthOf(t, c, srv.URL)
	if raw["workerAlive"] != false {
		t.Errorf("没有 worker 在跑时 workerAlive = %v, want false", raw["workerAlive"])
	}
}

// 需求：有 worker 在跑时，workerAlive 必须是 true —— 否则就是假告警。
//
// 「在跑」的判据取的是 worker 的单实例 advisory lock 还被持有着，
// 所以这里就用 worker 启动时走的那条路径（TryWorkerLock）来造现场，
// 不另外造一个只有测试才有的开关。
func TestHealthReportsWorkerAliveWhileItHoldsTheLock(t *testing.T) {
	srv, st := newServer(t)
	c := adminClient(t, srv.URL)
	ctx := context.Background()

	lock, err := st.TryWorkerLock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if lock == nil {
		t.Fatal("没有别的 worker 在跑，应当拿得到单实例锁")
	}

	raw, _ := healthOf(t, c, srv.URL)
	if raw["workerAlive"] != true {
		t.Errorf("worker 持锁期间 workerAlive = %v, want true", raw["workerAlive"])
	}

	// 放锁之后要能立刻翻回 false —— 存活判定不能有粘滞。
	lock.Release()
	raw, _ = healthOf(t, c, srv.URL)
	if raw["workerAlive"] != false {
		t.Errorf("worker 放锁之后 workerAlive = %v, want false", raw["workerAlive"])
	}
}

// 需求：outboxPending 反映真实的待扇出条数，扇出完要归零。
func TestHealthCountsPendingOutboxEvents(t *testing.T) {
	srv, st := newServer(t)
	c := adminClient(t, srv.URL)
	ctx := context.Background()
	id, _ := register(t, srv, st, "rover")

	if raw, _ := healthOf(t, c, srv.URL); numOf(t, raw, "outboxPending") != 0 {
		t.Fatalf("还没发生任何事，outboxPending 应当是 0")
	}

	if _, err := st.CreateTodo(ctx, store.CreateTodoParams{
		New: domain.NewTodo{Title: "t", Body: "b", PrimaryAgentID: id}, CreatedBy: "admin",
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := healthOf(t, c, srv.URL)
	if n := numOf(t, raw, "outboxPending"); n < 1 {
		t.Errorf("刚建完 todo，outboxPending = %v, want ≥1", n)
	}

	if _, err := st.ProcessOutboxBatch(ctx, 10, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ = healthOf(t, c, srv.URL)
	if n := numOf(t, raw, "outboxPending"); n != 0 {
		t.Errorf("扇出完之后 outboxPending = %v, want 0", n)
	}
}

// 需求：pendingLongPolls 是「此刻挂着几个长轮询」，请求结束要减回来。
func TestHealthCountsInFlightLongPolls(t *testing.T) {
	srv, st := newServer(t)
	c := adminClient(t, srv.URL)
	_, cred := register(t, srv, st, "rover")

	if raw, _ := healthOf(t, c, srv.URL); numOf(t, raw, "pendingLongPolls") != 0 {
		t.Fatalf("没有请求挂着时 pendingLongPolls 应当是 0")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		getWith(t, srv.URL+"/api/agent/me/inbox?after=0&wait=3s", cred)
	}()

	// 等长轮询真的挂上去。轮询而不是 sleep 一个固定值：
	// 固定值短了必然假失败，长了白等。
	var got float64
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		raw, _ := healthOf(t, c, srv.URL)
		if got = numOf(t, raw, "pendingLongPolls"); got >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got < 1 {
		t.Errorf("有一个长轮询挂着时 pendingLongPolls = %v, want ≥1", got)
	}

	<-done
	if raw, _ := healthOf(t, c, srv.URL); numOf(t, raw, "pendingLongPolls") != 0 {
		t.Errorf("长轮询结束之后 pendingLongPolls 应当减回 0")
	}
}

func numOf(t *testing.T, raw map[string]any, key string) float64 {
	t.Helper()
	v, ok := raw[key].(float64)
	if !ok {
		t.Fatalf("%s = %#v，不是数字", key, raw[key])
	}
	return v
}
