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

# 镜像标签。**不只是元数据**：`make backend` 每次 --build 都会把上一版镜像变成悬空镜像
# （tag 挪走了、层还占着磁盘），Makefile 的 docker-prune 就是按下面这个 source 标签
# 把「自己的」悬空镜像挑出来清掉的 —— 物理机上跑的不止 agent-hub，不能用全局 prune。
# ⚠️ 这个值和 Makefile 里的 IMAGE_LABEL_VALUE 必须一致，改一处要改两处。
# 对不上的后果是**清理静默变成空操作**：命令照样返回 0，磁盘继续涨。
# docker-prune 因此会先 grep 这两个文件核对，对不上直接报错，不让它默默失效。
LABEL org.opencontainers.image.source="https://github.com/superfive666/agent-hub"
LABEL org.opencontainers.image.title="agent-hub-worker"

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
