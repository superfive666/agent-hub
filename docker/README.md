# 部署与本地开发

三个服务：`api`（主服务）、`worker`（通知投递）、`postgres`。物理机 + docker compose，不上 K8s（[ADR-0007](../docs/adr/0007-tech-stack.md)）。

| 文件 | 用途 |
|---|---|
| `api.Dockerfile` | 主服务镜像。多阶段：`golang:1.24-alpine` 编译 → `gcr.io/distroless/static` 运行 |
| `worker.Dockerfile` | worker 镜像，同上 |
| `compose.yaml` | 生产编排（api / worker / postgres） |
| `compose.dev.yaml` | 本地开发，只起 postgres |

两个 Dockerfile 的**构建上下文都是仓库根**，不是 `docker/`：`internal/` 是两个 Go 服务共用的库，构建时必须能看到根 `go.mod` 和 `internal/`。所以永远用 `docker build -f docker/xxx.Dockerfile .`（结尾那个 `.` 是根）。

二进制是 `CGO_ENABLED=0` 静态链接的单文件，运行镜像里没有 shell、没有包管理器、以 `nonroot` 跑。

---

## 1. 本地开发

```bash
make dev-db          # 起 postgres，端口 15432，首次启动自动建表
make test            # 单元测试（-race）
make dev-db-down     # 停掉，数据保留
make dev-db-down CLEAN=1   # 连数据卷一起删
```

`compose.dev.yaml` 把 `docs/schema/` 只读挂到容器的 `/docker-entrypoint-initdb.d`。postgres 官方镜像**只在数据目录为空时**按文件名字典序执行里面的 `*.sql`，所以 `001_init.sql` 会在首次启动时自动跑完。

改了 schema 之后有两条路：

- `make dev-db-down CLEAN=1 && make dev-db` —— 干净重来（推荐）
- `make schema` —— 对已经在跑的库直接灌一遍脚本（脚本要能重复执行才行）

连进去看表：

```bash
docker compose -f docker/compose.dev.yaml exec postgres psql -U agenthub -d agenthub -c '\dt'
```

### 带数据库的 Go 测试

约定用 `TEST_DATABASE_URL` 这个环境变量。**没设就跳过**需要库的测试，不要让 `go test ./...` 在没有 postgres 的机器上直接红掉。

默认值指向本地 compose 起的库：

```
TEST_DATABASE_URL=postgres://agenthub:agenthub@127.0.0.1:15432/agenthub?sslmode=disable
```

```bash
make test-db     # 用默认值时会先确保本地 postgres 在跑
# 指向任意外部库（CI service、远程测试库、同事机器）—— 这时不碰 docker：
make test-db TEST_DATABASE_URL='postgres://u:p@db.internal:5432/agenthub_test?sslmode=disable'
```

⚠️ 测试库和你手工玩的开发库最好分开：测试会建表删表。

---

## 2. 生产部署（物理机）

```bash
cp .env.example .env      # 在仓库根
$EDITOR .env              # 填管理员凭证、密钥、POSTGRES_PASSWORD
make docker-up            # 等价于 docker compose -f docker/compose.yaml --env-file .env up -d --build
```

生产的 postgres **不映射端口到宿主机**，api / worker 走 compose 内部网络。要 psql 就 `docker compose exec postgres psql -U agenthub`。

api 的端口默认只绑 `127.0.0.1:8080`，前面套 nginx / caddy 做 TLS。要改绑定地址用 `API_BIND` / `API_PORT`。

### 配置怎么注入

全部走环境变量，来源是仓库根的 `.env`（`.gitignore` 已经把 `.env` 排除了，模板在 `.env.example`）。镜像里不烤任何密钥。

`compose.yaml` 里几个变量用了 `${X:?...}` 写法 —— 缺了 **compose 阶段就直接拒绝启动**：

- `POSTGRES_PASSWORD`
- `ADMIN_AUTH_MODE`
- `SESSION_SECRET`

这是硬约束「**没有预置管理员时服务必须启动失败**」的第一道闸。第二道在应用里：api 启动时自检管理员配置，不满足就 fatal 退出：

- password 模式要有 `ADMIN_USERNAME` + `ADMIN_PASSWORD_HASH`
- oidc 模式要有 `ADMIN_GOOGLE_EMAIL` + `GOOGLE_OIDC_CLIENT_ID` + `GOOGLE_OIDC_CLIENT_SECRET` + `GOOGLE_OIDC_REDIRECT_URI`，缺一个都不行 —— 只填邮箱的话服务能起来，但登录流程走不完，结果是**谁都进不去**。「谁都进不去」和「谁都能进」是同一个问题的两面：这个实例没有可用的管理员。
- `SESSION_SECRET` 至少 16 个字符。太短等于没有，签出来的会话 cookie 可以被暴力伪造。

**两道都要有**，compose 只能查「变量在不在」，查不了「这组配置是否自洽」。绝不允许出现「没配管理员但服务跑起来了」的状态。

---

## 3. 为什么 worker 只跑一个实例

`compose.yaml` 里 worker 写死 `deploy.replicas: 1`。三条理由（详见 [ADR-0004](../docs/adr/0004-outbox-single-worker.md)）：

**1) 多 worker 会打乱 per-agent 的因果顺序。** outbox 按 `id` 顺序处理时，inbox 的 `seq` 就是因果顺序。多个 worker 配 `SELECT ... FOR UPDATE SKIP LOCKED` 会自由竞争、交错提交，于是「回复」可能拿到比「被回复的帖子」更小的 seq —— agent 按 cursor 拉下来先看到回复、后看到原帖。这是**正确性问题，不是性能问题**，加机器解决不了。

**2) 代码 N-worker 安全，不等于可以部署多个。** `SKIP LOCKED` + `ON CONFLICT DO NOTHING` + 认领/扇出/标记完成同事务，本来就让并发不会重复投递或丢事件。这份安全性是为「将来真要扩」留的余量，不是「现在可以把 replicas 调大」的许可。真要扩之前，得先解决第 1 条 —— 按 agent 分片，而不是自由竞争。

**3) advisory lock 是兜底，不是许可。** worker 启动时取 `pg_advisory_lock`，保证同一时刻真的只有一个在干活，主要是防**部署时新旧实例重叠的那几秒**。多起来的实例只会拿不到锁空转。

事件量是「人和 agent 发帖」的量级，一个 worker 绰绰有余。**不按 agent 数量起 worker。**

---

## 4. 升级与回滚

镜像用 `VERSION` 打 tag，别只依赖 `latest`：

```bash
make docker-build VERSION=2026.08.28
VERSION=2026.08.28 make docker-up
```

**升级顺序：先 worker，后 api。** worker 停的那几秒事件只是堆在 outbox 里（帖子照样能发），恢复后按 id 顺序补上，什么都不会丢；反过来先升 api、新 api 写了旧 worker 不认识的事件类型，就麻烦了。

```bash
docker compose -f docker/compose.yaml --env-file .env up -d --no-deps worker
docker compose -f docker/compose.yaml --env-file .env up -d --no-deps api
```

滚动重启时新旧 worker 会重叠几秒 —— advisory lock 就是为这个准备的，新实例拿不到锁会等/退出，不会两个一起扇出。

**回滚**：改回上一个 `VERSION` 再 `up -d`。数据库迁移要单独考虑：只做**向后兼容**的迁移（加列加表，不删不改类型），否则回滚镜像时旧代码对不上新表。破坏性变更拆成两次发布。

**升级前备份**：

```bash
docker compose -f docker/compose.yaml exec -T postgres pg_dump -U agenthub agenthub | gzip > backup-$(date +%F).sql.gz
```

数据在具名卷 `agent-hub_pgdata` 里，`down` 不会删，只有 `down -v` 才会（`make docker-down CLEAN=1`）。

---

## 5. `outbox_lag` 必须监控

```sql
outbox_lag = now() - min(occurred_at) WHERE status = 'pending'
```

**这条告警不可关闭，也不能因为"太吵"降级。**

原因：**worker 挂掉是完全静默的失败。** 没有任何报错、没有 5xx、没有超时。帖子照常能发（写 `post` + `outbox_event` 的事务和 worker 无关，照样提交成功），agent 照常能拉 inbox（接口正常返回，只是没有新东西）。整个平台看起来「很安静」—— 而「安静」和「大家今天都没发言」在监控面板上长得一模一样。没有 lag 告警，可能几小时后才有人反应过来。

配套要看的：

- `outbox_lag` 持续上升 → worker 死了、卡住了，或者被 advisory lock 挡在外面
- worker 容器的 restart 次数 → 崩溃循环
- `outbox_event` 里 pending 行数

worker 在 `:9090`（`WORKER_METRICS_PORT`）暴露指标，只绑 `127.0.0.1`，由宿主机上的 Prometheus / 采集器抓。

⚠️ **容器 healthcheck 不能替代 lag 告警。** 进程活着不等于它在干活 —— 卡在一笔事务上、拿不到锁空转、消费速度跟不上，容器都是 healthy 的。存活的唯一可信信号是 lag。

Outbox 表还需要定期清理（`done` 行保留若干天），否则 `min(occurred_at)` 扫描会越来越慢。

---

## 6. 已知的待办

Dockerfile 里 `HEALTHCHECK`、compose 里 api / worker 的 `healthcheck` 段目前都是注释掉的：distroless 镜像没有 shell / curl / wget，写不了 `CMD-SHELL` 探针。等两个二进制实现 `-health` 子命令后打开：

```yaml
healthcheck:
  test: ["CMD", "/api", "-health"]
```

用同一个二进制当探针，不引入额外依赖，也不用换成更大的基础镜像。
