# agent-hub-worker：通知投递 worker（消费 outbox → 扇出 inbox → 经 gateway 通知 agent）
#
# 构建上下文是**仓库根目录**：
#   docker build -f docker/worker.Dockerfile -t agent-hub-worker:dev .
#
# 注意：这个镜像**只允许跑一个实例**（ADR-0004）。约束在 compose 的 deploy.replicas: 1
# 表达，进程自身还会取 pg_advisory_lock 兜底，防止部署时新旧实例重叠那几秒。

# ---------- 构建阶段 ----------
FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS=-trimpath \
    go build -tags netgo -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/worker ./agent-hub-worker/cmd/worker

# ---------- 运行阶段 ----------
FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=build /out/worker /worker

USER nonroot:nonroot
# worker 不服务业务流量，只暴露指标 / 健康端口
EXPOSE 9090

# 健康检查思路（比 api 更重要，因为 worker 挂掉是**完全静默**的失败）：
#   进程活着 != worker 在干活。真正的存活信号是 outbox_lag：
#       outbox_lag = now() - min(occurred_at) WHERE status = 'pending'
#   建议 worker 在 :9090 上暴露 /healthz（进程 + 数据库连接 + 是否持有 advisory lock）
#   和 /metrics（含 outbox_lag），由外部 Prometheus 抓取并对 lag 告警。
#   容器级 healthcheck 同样受 distroless 无 shell 限制，推荐做法是给二进制加 `-health`
#   子命令：HEALTHCHECK CMD ["/worker", "-health"]。现在还没实现，故留空。
#   ⚠️ 只靠容器 healthcheck 是不够的：lag 告警不可关闭（ADR-0004）。
ENTRYPOINT ["/worker"]
