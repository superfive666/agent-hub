// Package gateway 把「你有新事件了」这个信号送给 agent。
//
// 它只送信号，不送内容 —— 正确性完全由 inbox + cursor 保证，
// 信号丢了 agent 下次拉取自然补齐。所以这里的每一处失败都可以直接放弃，
// 不需要重试到死。见 ADR-0006。
package gateway

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/superfive666/agent-hub/internal/store"
)

// Gateway 是通知投递的出口。
type Gateway interface {
	Notify(ctx context.Context, ns []store.Notification)
}

// Multi 把通知同时发给多个出口。
type Multi []Gateway

func (m Multi) Notify(ctx context.Context, ns []store.Notification) {
	for _, g := range m {
		g.Notify(ctx, ns)
	}
}

// Channel 是 PostgreSQL LISTEN/NOTIFY 用的频道名。
// API 进程 LISTEN 它，收到后去完成挂起的长轮询请求。
const Channel = "agent_inbox"

// PgNotify 通过 pg_notify 把信号广播给正在 LISTEN 的 API 进程。
//
// 用它而不是让 worker 直连 API：worker 和 API 是两个进程，
// 而库本来就是它们唯一的共享点，不必再引一条进程间通道。
type PgNotify struct {
	DB  *sql.DB
	Log *slog.Logger
}

func (p *PgNotify) Notify(ctx context.Context, ns []store.Notification) {
	for _, n := range ns {
		payload, err := json.Marshal(map[string]any{"agentId": n.AgentID, "seq": n.Seq})
		if err != nil {
			continue
		}
		// 失败就算了：信号可以丢。
		if _, err := p.DB.ExecContext(ctx, `SELECT pg_notify($1,$2)`, Channel, string(payload)); err != nil {
			p.log().Debug("pg_notify 失败，忽略", "agent", n.AgentID, "err", err)
		}
	}
}

func (p *PgNotify) log() *slog.Logger {
	if p.Log != nil {
		return p.Log
	}
	return slog.Default()
}

// EndpointLookup 查某个 agent 的 webhook 地址。没配 webhook 的返回空串。
type EndpointLookup func(ctx context.Context, agent string) (string, error)

// Webhook 把信号 POST 给 connector 的本地端点。
//
// **每个 agent 一个有界待发槽，满了直接丢。** 一个连上了但不响应的端点
// 不能把其他 agent 的通知拖住 —— 丢信号是安全的，为了不丢一条而阻塞所有人才是真的亏。
type Webhook struct {
	Lookup  EndpointLookup
	Client  *http.Client
	Slots   int // 每个 agent 的待发槽，默认 1
	Timeout time.Duration
	Log     *slog.Logger

	mu       sync.Mutex
	inFlight map[string]int
	dropped  int
}

func (w *Webhook) Notify(ctx context.Context, ns []store.Notification) {
	for _, n := range ns {
		agent := string(n.AgentID)
		if !w.reserve(agent) {
			w.countDrop()
			w.log().Debug("待发槽已满，丢弃通知", "agent", agent, "seq", n.Seq)
			continue
		}
		go func(n store.Notification) {
			defer w.release(agent)
			w.deliver(context.WithoutCancel(ctx), n)
		}(n)
	}
}

// Dropped 返回累计丢弃的通知数。它应当被暴露成指标：
// 持续增长说明某个 agent 的端点有问题，而不是平台坏了。
func (w *Webhook) Dropped() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.dropped
}

func (w *Webhook) reserve(agent string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.inFlight == nil {
		w.inFlight = map[string]int{}
	}
	slots := w.Slots
	if slots <= 0 {
		slots = 1
	}
	if w.inFlight[agent] >= slots {
		return false
	}
	w.inFlight[agent]++
	return true
}

func (w *Webhook) release(agent string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.inFlight[agent] > 0 {
		w.inFlight[agent]--
	}
}

func (w *Webhook) countDrop() {
	w.mu.Lock()
	w.dropped++
	w.mu.Unlock()
}

func (w *Webhook) deliver(ctx context.Context, n store.Notification) {
	if w.Lookup == nil {
		return
	}
	url, err := w.Lookup(ctx, string(n.AgentID))
	if err != nil || url == "" {
		return
	}
	timeout := w.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, _ := json.Marshal(map[string]any{"agentId": n.AgentID, "seq": n.Seq})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		// 不重试：agent 下次拉 inbox 时会自然补齐。
		w.log().Debug("webhook 投递失败，不重试", "agent", n.AgentID, "err", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		w.log().Debug("webhook 返回错误码，不重试",
			"agent", n.AgentID, "status", resp.StatusCode)
	}
}

func (w *Webhook) log() *slog.Logger {
	if w.Log != nil {
		return w.Log
	}
	return slog.Default()
}

var _ Gateway = (*PgNotify)(nil)
var _ Gateway = (*Webhook)(nil)
var _ Gateway = Multi(nil)
