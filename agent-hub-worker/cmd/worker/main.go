// Command worker 是 agent-hub 的通知投递进程：消费 outbox、扇出 inbox、通知 agent。
//
// 部署时只跑一个实例（docker/compose.yaml 里写死 replicas: 1）。
// 它挂掉是完全静默的失败，所以 outbox_lag 告警不可关闭。
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/superfive666/agent-hub/agent-hub-worker/internal/gateway"
	"github.com/superfive666/agent-hub/agent-hub-worker/internal/worker"
	"github.com/superfive666/agent-hub/internal/store"
)

func main() {
	if err := run(); err != nil {
		if errors.Is(err, worker.ErrNotLeader) {
			slog.Info("已有 worker 在运行，本实例退出")
			return // 正常退出：这不是错误，是单实例约束在起作用
		}
		slog.Error("worker 退出", "err", err)
		os.Exit(1)
	}
}

func run() error {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("必须配置 DATABASE_URL")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer st.Close()

	gw := gateway.Multi{
		&gateway.PgNotify{DB: st.DB(), Log: log},
		&gateway.Webhook{Lookup: webhookLookup(st.DB()), Log: log},
	}

	cfg := worker.Config{
		BatchSize:    envInt("WORKER_BATCH_SIZE", 100),
		IdleInterval: envDuration("WORKER_IDLE_INTERVAL", 500*time.Millisecond),
		LagWarnAfter: envDuration("OUTBOX_LAG_WARN_AFTER", 30*time.Second),
	}

	go serveMetrics(ctx, st, log)

	return worker.New(st, gw, cfg, log).Run(ctx)
}

// webhookLookup 从 Agent Card 的扩展字段里取 webhook 地址。
// 没配的返回空串，Webhook 出口会跳过它。
func webhookLookup(db *sql.DB) gateway.EndpointLookup {
	return func(ctx context.Context, agent string) (string, error) {
		var url sql.NullString
		err := db.QueryRowContext(ctx, `
			SELECT document #>> '{capabilities,extensions,0,webhookUrl}'
			FROM agent_card WHERE agent_id = $1
			ORDER BY version DESC LIMIT 1`, agent).Scan(&url)
		if err == sql.ErrNoRows {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		return url.String, nil
	}
}

// serveMetrics 暴露健康检查与那条不可关闭的 lag 指标。
func serveMetrics(ctx context.Context, st *store.Store, log *slog.Logger) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := st.DB().PingContext(r.Context()); err != nil {
			http.Error(w, "db unreachable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		lag, err := st.OutboxLagSeconds(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "# HELP agenthub_outbox_lag_seconds 最老一条待扇出事件已等待的秒数\n")
		fmt.Fprintf(w, "# TYPE agenthub_outbox_lag_seconds gauge\n")
		fmt.Fprintf(w, "agenthub_outbox_lag_seconds %f\n", lag)
	})

	srv := &http.Server{
		Addr:              ":" + envStr("WORKER_METRICS_PORT", "9090"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.WithoutCancel(ctx)) }()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("指标端口退出", "err", err)
	}
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
