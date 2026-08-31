# agent-hub 主服务（admin API / agent API / thread / todo / tweet / inbox / 名录）
#
# 构建上下文是**仓库根目录**，不是 docker/：
#   docker build -f docker/api.Dockerfile -t agent-hub-api:dev .
# 因为 internal/ 是两个 Go 服务共用的库（ADR-0007），构建时必须能看到根 go.mod 和 internal/。

# ---------- 构建阶段 ----------
FROM golang:1.26-alpine AS build

WORKDIR /src

# 先只拷依赖描述，命中 layer 缓存；改业务代码时不用重新下模块
COPY go.mod go.sum* ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 + netgo：静态链接单二进制，distroless/static 里没有 libc，也没有 /etc/nsswitch.conf
# -trimpath 去掉构建机路径，-s -w 去符号表，镜像更小
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS=-trimpath \
    go build -tags netgo -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/api ./agent-hub/cmd/api

# ---------- 运行阶段 ----------
FROM gcr.io/distroless/static:nonroot

# 镜像标签。**不只是元数据**：`make backend` 每次 --build 都会把上一版镜像变成悬空镜像
# （tag 挪走了、层还占着磁盘），Makefile 的 docker-prune 就是按下面这个 source 标签
# 把「自己的」悬空镜像挑出来清掉的 —— 物理机上跑的不止 agent-hub，不能用全局 prune。
# ⚠️ 这个值和 Makefile 里的 IMAGE_LABEL_VALUE 必须一致，改一处要改两处。
# 对不上的后果是**清理静默变成空操作**：命令照样返回 0，磁盘继续涨。
# docker-prune 因此会先 grep 这两个文件核对，对不上直接报错，不让它默默失效。
LABEL org.opencontainers.image.source="https://github.com/superfive666/agent-hub"
LABEL org.opencontainers.image.title="agent-hub-api"

# CA 证书（distroless/static 自带 /etc/ssl/certs），时区数据不带 —— 平台时区靠
# PLATFORM_TIMEZONE 环境变量在应用层处理，不依赖系统 tzdata。
# 若将来需要 time.LoadLocation，改用 distroless/base 或在 build 阶段 COPY zoneinfo。

WORKDIR /
COPY --from=build /out/api /api

USER nonroot:nonroot
EXPOSE 8080

# 健康检查思路：
#   distroless 里没有 shell、没有 curl、没有 wget，写不了 HEALTHCHECK CMD。
#   所以健康检查放在 compose 里（见 compose.yaml），或者由 api 自己暴露 /healthz，
#   外部 LB / 监控去探。两条路二选一：
#     a) 应用内置一个 `-health` 子命令，用同一个二进制做探针：
#        HEALTHCHECK CMD ["/api", "-health"]      ← 推荐，不引入额外依赖
#     b) 用 compose 的 healthcheck + 外部工具探 http://api:8080/healthz
#   现在 binary 还没实现 -health，所以这里先不写 HEALTHCHECK，compose 里也留了注释。
ENTRYPOINT ["/api"]
