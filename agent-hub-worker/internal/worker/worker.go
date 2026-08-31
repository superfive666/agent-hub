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
	// RenotifyEvery 多久扫一遍「欠着事件又失联」的 agent 并重发信号。默认 60s。
	// 设成负数关掉（测试里用）。
	RenotifyEvery time.Duration
	// PurgeEvery 多久清一次已 ack 且过期的 inbox 事件。默认 1 小时。
	PurgeEvery time.Duration
	// InboxRetention 保留期的兜底值。**只影响已经被 ack 的事件** ——
	// 没 ack 的一条都不删，那是断线 agent 全部的救命数据。
	//
	// 真正生效的是平台设置里的 `inboxRetentionDays`（管理员在设置页能改）；
	// 这里只是读不到设置时的兜底。0 表示不清理。
	InboxRetention time.Duration
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
	if c.RenotifyEvery == 0 {
		c.RenotifyEvery = time.Minute
	}
	if c.PurgeEvery == 0 {
		c.PurgeEvery = time.Hour
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
	lock, err := w.store.TryWorkerLock(ctx)
	if err != nil {
		return err
	}
	if lock == nil {
		return ErrNotLeader
	}
	defer lock.Release()

	w.log.Info("worker 启动",
		"batch", w.cfg.BatchSize, "idle", w.cfg.IdleInterval, "lagWarnAfter", w.cfg.LagWarnAfter)

	lagTicker := time.NewTicker(10 * time.Second)
	defer lagTicker.Stop()

	// 补投与清理各自一个慢 ticker。跟着 outbox 主循环走会让它们的节奏
	// 随负载漂：忙的时候几乎不跑，闲的时候一秒跑好几遍。
	renotify := newTicker(w.cfg.RenotifyEvery)
	defer renotify.Stop()
	purge := newTicker(w.cfg.PurgeEvery)
	defer purge.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("worker 收到停止信号，退出")
			return nil
		case <-lagTicker.C:
			w.checkLag(ctx)
			if err := w.checkLock(ctx, lock); err != nil {
				return err
			}
		case <-renotify.C:
			w.renotifyStalled(ctx)
		case <-purge.C:
			w.purgeInbox(ctx)
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

// newTicker 包一层，好让「关掉」也有个统一写法。
// 负数或 0 得到一个永远不响的 ticker —— 比在调用点到处判空干净。
func newTicker(d time.Duration) *time.Ticker {
	if d <= 0 {
		t := time.NewTicker(time.Hour)
		t.Stop() // 停掉的 ticker 的 channel 永远不会有值
		return t
	}
	return time.NewTicker(d)
}

// renotifyStalled 给「欠着事件又失联」的 agent 重发一次信号。
//
// **这是「断线重连之后自动补上」的 hub 那一半。** connector 那一半是
// cursor 落盘、重连后续拉；但那只在「有人在拉」时成立。走 webhook 契约的端点
// 是被动的：信号在它下线那一刻发出、丢掉，之后没有第二次，
// 事件就一直躺在 inbox 里，而它以为自己没事可做 —— 没有任何一端会报错。
//
// 重发是幂等的：信号里只有 {agentId, seq}，收到几次都只导致「去拉一次」。
// 拉取本身按 cursor 做增量，所以重发绝不会让 agent 重复处理同一条事件。
func (w *Worker) renotifyStalled(ctx context.Context) {
	owed, err := w.store.AgentsOwingEvents(ctx, 200)
	if err != nil {
		w.log.Error("查欠账 agent 失败", "err", err)
		return
	}
	if len(owed) == 0 || w.gw == nil {
		return
	}
	ns := make([]store.Notification, 0, len(owed))
	for _, o := range owed {
		ns = append(ns, store.Notification{AgentID: o.AgentID, Seq: o.LastSeq})
	}
	w.gw.Notify(ctx, ns)
	// Info 而不是 Debug：这条日志是「有 agent 掉线了」的第一手线索。
	w.log.Info("给失联的 agent 重发信号", "agents", len(ns),
		"worst", owed[0].LastSeq-owed[0].Cursor, "silentFor", owed[0].SilentFor.Round(time.Second))
}

// purgeInbox 清掉已 ack 且过期的事件。
//
// `InboxRetention` 这个设置项以前是个空壳：契约里有、设置页里有，
// 但全仓一行清理代码都没有，表一直在长。
func (w *Worker) purgeInbox(ctx context.Context) {
	// 每轮都读一次设置，而不是启动时读一次：管理员在设置页把保留期改了，
	// 不该还要重启 worker 才生效。一小时一次查询，可以忽略。
	retain := w.cfg.InboxRetention
	if st, err := w.store.GetSettings(ctx, "UTC"); err == nil && st.InboxRetentionDays > 0 {
		retain = time.Duration(st.InboxRetentionDays) * 24 * time.Hour
	}
	if retain <= 0 {
		return
	}
	n, err := w.store.PurgeAckedInboxEvents(ctx, retain)
	if err != nil {
		w.log.Error("清理 inbox 失败", "err", err)
		return
	}
	if n > 0 {
		w.log.Info("清理已确认的 inbox 事件", "deleted", n, "retention", retain)
	}
}

// checkLock 定期确认单实例锁还在自己手上。
//
// **不做这件事的后果不是「少一条日志」**：持锁的那条连接从启动起就再没被用过，
// 是整个系统里最容易被悄悄掐断的东西（PG 重启、idle_session_timeout、连接池、
// 防火墙 NAT 老化）。断了之后 PostgreSQL 立刻放锁，而这个进程照常干活 ——
// 控制台从此永远显示「worker 无心跳」（一条永不消失的假告警），
// 同时锁真的空了出来，第二个 worker 能直接拿到，两个一起扇出。见 store.WorkerLock。
func (w *Worker) checkLock(ctx context.Context, lock *store.WorkerLock) error {
	err := lock.Verify(ctx)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrLockTaken):
		// 别人接管了。**停下来是对的**：两个 worker 同时扇出会打乱 per-agent 的
		// 因果顺序（ADR-0004），那是数据正确性问题，而且没有任何告警会报；
		// 而「零个 worker」有 outbox_lag 那条不可关闭的告警兜着。
		w.log.Warn("单实例锁已被其它实例接管，本实例退出")
		return ErrNotLeader
	default:
		// 没查清楚（比如库正在重启）。不退出：下一轮再问一次。
		// 这里退出会把一次短暂的抖动变成一次真正的停服。
		w.log.Error("确认单实例锁失败，下一轮重试", "err", err)
		return nil
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
