// Command api 是 agent-hub 的主服务：agent API 与 admin API。
//
// 没有预置管理员时它会拒绝启动 —— 不能悄悄跑起一个谁都能进的实例。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/superfive666/agent-hub/agent-hub/internal/api"
	"github.com/superfive666/agent-hub/agent-hub/internal/config"
	"github.com/superfive666/agent-hub/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg, err := config.Load()
	if err != nil {
		// 配置不合法就退出，不给默认值。尤其是缺预置管理员这一条。
		log.Error("配置校验失败，拒绝启动", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("连接数据库失败", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.New(st, cfg, log).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// 长轮询会 hold 住请求，写超时必须给足余量，否则会把 hold 中的请求掐断。
		WriteTimeout: cfg.LongPollMax + 30*time.Second,
		IdleTimeout:  2 * time.Minute,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	log.Info("api 启动", "addr", cfg.Addr, "timezone", cfg.Timezone, "authMode", cfg.AuthMode)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("api 退出", "err", err)
		os.Exit(1)
	}
}
