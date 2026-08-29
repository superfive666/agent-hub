// Package worker 消费 outbox、扇出到各 agent 的 inbox，然后经 gateway 发出通知。
//
// **这个进程挂了，整个平台会静默地停止工作**：帖子照常能发（写 outbox 成功），
// agent 照常能拉 inbox（只是拉不到新东西），没有任何报错。
// 所以这里的每一处设计取舍都要从这一点出发 —— 尤其是 outbox_lag 指标。
package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/superfive666/agent-hub/agent-hub-worker/internal/gateway"
	"github.com/superfive666/agent-hub/internal/store"
)

// ErrNotLeader 表示已经有另一个 worker 在跑。拿不到锁就退出，不是等待。
var ErrNotLeader = errors.New("已有 worker 实例在运行，本实例退出")

// Config 是 worker 的运行参数。
type Config struct {
	// BatchSize 一次认领多少条 outbox。默认 100。
	BatchSize int
	// IdleInterval 没活干时歇多久再看一眼。默认 500ms。
	// 这是通知延迟的上界之一，但不是唯一 —— webhook 是即时的。
	IdleInterval time.Duration
	// LagWarnAfter 超过这个积压时长就告警。默认 30s。
	// 这条告警不可关闭：它是唯一能发现 worker 静默死亡的地方。
	LagWarnAfter time.Duration
}

func (c Config) withDefaults() Config {
	if c.BatchSize <= 0 {
		c.BatchSize = 100
	}
	if c.IdleInterval <= 0 {
		c.IdleInterval = 500 * time.Millisecond
	}
	if c.LagWarnAfter <= 0 {
		c.LagWarnAfter = 30 * time.Second
	}
	return c
}

// Worker 是 outbox 的消费者。
type Worker struct {
	store *store.Store
	gw    gateway.Gateway
	cfg   Config
	log   *slog.Logger
}

func New(s *store.Store, gw gateway.Gateway, cfg Config, log *slog.Logger) *Worker {
	if log == nil {
		log = slog.Default()
	}
	return &Worker{store: s, gw: gw, cfg: cfg.withDefaults(), log: log}
}

// RunOnce 处理一批，返回处理条数。0 表示当前没活干。
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	return w.store.ProcessOutboxBatch(ctx, w.cfg.BatchSize, func(ctx context.Context, ns []store.Notification) {
		// 这里已经在事务提交之后 —— agent 现在来拉一定拉得到。
		if w.gw != nil {
			w.gw.Notify(ctx, ns)
		}
	})
}

// Run 一直跑到 ctx 结束。
//
// 单实例：进来先抢 advisory lock，抢不到直接返回 ErrNotLeader。
// 多 worker 会打乱 per-agent 的因果顺序（「回复」可能排在「被回复的帖子」前面），
// 代码虽然是 N-worker 安全的，部署仍然只跑一个。见 ADR-0004。
func (w *Worker) Run(ctx context.Context) error {
	ok, release, err := w.store.TryWorkerLock(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotLeader
	}
	defer release()

	w.log.Info("worker 启动",
		"batch", w.cfg.BatchSize, "idle", w.cfg.IdleInterval, "lagWarnAfter", w.cfg.LagWarnAfter)

	lagTicker := time.NewTicker(10 * time.Second)
	defer lagTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("worker 收到停止信号，退出")
			return nil
		case <-lagTicker.C:
			w.checkLag(ctx)
		default:
		}

		n, err := w.RunOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			// 单次失败不退出：outbox 行会回到 pending，下一轮重来。
			w.log.Error("处理 outbox 失败，下一轮重试", "err", err)
			if !sleep(ctx, w.cfg.IdleInterval) {
				return nil
			}
			continue
		}
		if n == 0 {
			if !sleep(ctx, w.cfg.IdleInterval) {
				return nil
			}
		}
	}
}

// checkLag 盯住那条不可关闭的告警。
func (w *Worker) checkLag(ctx context.Context) {
	lag, err := w.store.OutboxLagSeconds(ctx)
	if err != nil {
		w.log.Error("读取 outbox_lag 失败", "err", err)
		return
	}
	if lag >= w.cfg.LagWarnAfter.Seconds() {
		// 这条日志不许降级成 Debug，也不许加静默窗口。
		w.log.Error("outbox 积压超过阈值",
			"lagSeconds", lag, "thresholdSeconds", w.cfg.LagWarnAfter.Seconds())
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
