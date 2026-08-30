# agent-hub 根 Makefile。`make help` 列出所有目标。
SHELL := /bin/bash

DEV_COMPOSE  := docker/compose.dev.yaml
PROD_COMPOSE := docker/compose.yaml
COMPOSE      := docker compose

DEV_PG_PORT ?= 15432
DEV_PG_USER ?= agenthub
DEV_PG_PASS ?= agenthub
DEV_PG_DB   ?= agenthub

# 带数据库的测试连哪个库。默认指向 make dev-db 起的本地库，
# 但可以覆盖成任意外部 PostgreSQL（CI 的 service、同事的机器、远程测试库都行）：
#   make test-db TEST_DATABASE_URL=postgres://u:p@somehost:5432/db?sslmode=disable
TEST_DATABASE_URL ?= postgres://$(DEV_PG_USER):$(DEV_PG_PASS)@127.0.0.1:$(DEV_PG_PORT)/$(DEV_PG_DB)?sslmode=disable

SCHEMA_FILES := $(sort $(wildcard docs/schema/*.sql))
VERSION ?= dev

.DEFAULT_GOAL := help

.PHONY: help dev-db dev-db-down schema test test-db lint build docker-build docker-up docker-down \
        backend web web-test connector-test api-docs android-core-test android-test android-apk verify

help: ## 列出所有目标
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

dev-db: ## 起本地开发用 postgres（首次启动自动执行 docs/schema/*.sql 建表）
	$(COMPOSE) -f $(DEV_COMPOSE) up -d --wait

dev-db-down: ## 停掉本地 postgres（加 CLEAN=1 连数据卷一起删，下次重新建表）
	$(COMPOSE) -f $(DEV_COMPOSE) down $(if $(CLEAN),-v,)

schema: ## 对本地库执行建表脚本（库已存在、initdb 不会再跑时用这个）
	@test -n "$(SCHEMA_FILES)" || { echo "docs/schema/ 下没有 .sql"; exit 1; }
	@for f in $(SCHEMA_FILES); do \
		echo ">> $$f"; \
		$(COMPOSE) -f $(DEV_COMPOSE) exec -T postgres \
			psql -v ON_ERROR_STOP=1 -U $(DEV_PG_USER) -d $(DEV_PG_DB) < $$f || exit 1; \
	done

test: ## 跑全部单元测试（-race，不需要数据库）
	go test ./... -race

test-db: ## 跑带数据库的测试；TEST_DATABASE_URL 未指向外部库时先起本地 postgres
	@if [ "$(TEST_DATABASE_URL)" = "postgres://$(DEV_PG_USER):$(DEV_PG_PASS)@127.0.0.1:$(DEV_PG_PORT)/$(DEV_PG_DB)?sslmode=disable" ]; then \
		echo ">> 用本地 compose 起的库，先确保它在跑"; $(MAKE) dev-db; \
	else \
		echo ">> 用外部库：$(TEST_DATABASE_URL)"; \
	fi
	TEST_DATABASE_URL='$(TEST_DATABASE_URL)' go test ./... -race

lint: ## gofmt 格式检查 + go vet
	@bad=$$(gofmt -l . | grep -v '^vendor/' || true); \
	if [ -n "$$bad" ]; then echo "gofmt 未通过："; echo "$$bad"; exit 1; fi
	go vet ./...

build: ## 在本机编译两个二进制到 bin/
	CGO_ENABLED=0 go build -trimpath -o bin/api    ./agent-hub/cmd/api
	CGO_ENABLED=0 go build -trimpath -o bin/worker ./agent-hub-worker/cmd/worker

docker-build: ## 构建 api / worker 镜像（构建上下文是仓库根）
	docker build -f docker/api.Dockerfile    --build-arg VERSION=$(VERSION) -t agent-hub-api:$(VERSION)    .
	docker build -f docker/worker.Dockerfile --build-arg VERSION=$(VERSION) -t agent-hub-worker:$(VERSION) .

docker-up: ## 生产编排起全套（api / worker / postgres），需要根目录有 .env
	$(COMPOSE) -f $(PROD_COMPOSE) --env-file .env up -d --build

# `--no-deps` 是这条命令的重点，不是可选项。
# compose.yaml 里 api / worker 都 `depends_on: postgres (service_healthy)`，
# 所以不带它的话，即使你只写了 `up api worker`，compose 也会把编排里那个
# postgres 一并拉起来 —— 用外部库的部署会因此多出一个空库跟真库并存，
# 而且 api/worker 要先等这个空库健康检查通过才肯启动。
# 用编排自带 postgres 的部署请用 `make docker-up`，那条会连库一起管。
backend: ## 只重建 api / worker 两个容器（用外部 postgres 的部署走这个，不要用 docker-up）
	$(COMPOSE) -f $(PROD_COMPOSE) --env-file .env up -d --build --no-deps api worker

docker-down: ## 停掉生产编排（数据卷保留；加 CLEAN=1 才删数据）
	$(COMPOSE) -f $(PROD_COMPOSE) --env-file .env down $(if $(CLEAN),-v,)

web: ## 构建管理控制台（会先按 openapi.yaml 重新生成类型）
	cd web && npm ci && npm run gen:api && npm run build

# ⚠️ 必须是 `tsc -b`，**不能写 `tsc --noEmit`**。
# web/tsconfig.json 是 solution 风格的（"files": []，只有 references），
# 对它跑 --noEmit 是个**空操作**：一个文件都不检查，永远绿。
# 真正带 noUnusedLocals / strict 的是 tsconfig.app.json，只有 -b 会走到它。
# 这个洞让一次「未使用的 import」一路过了自查、合了 PR，最后炸在生产机的
# `make web` 上（那里跑的是 npm run build，也就是 tsc -b）。
web-test: ## 控制台的类型检查与单元测试（含契约类型是否已重新生成）
	cd web && npm ci && npm run gen:api
	@# schema.d.ts 是从 openapi.yaml 生成的，但它进了仓库 —— 改了契约不重新生成，
	@# 下一个人 `make web` 时就会凭空多出一份 diff，每次都得手动带上。
	@# 这里生成完直接比 git 状态：脏了就说清楚该跑哪条命令。
	@if ! git diff --quiet -- web/src/api/schema.d.ts; then \
		echo ""; \
		echo "!! web/src/api/schema.d.ts 和 docs/api/openapi.yaml 对不上了。"; \
		echo "   改了契约就要一起提交生成结果：cd web && npm run gen:api"; \
		echo ""; \
		git --no-pager diff --stat -- web/src/api/schema.d.ts; \
		exit 1; \
	fi
	cd web && npx tsc -b && npx vitest run

connector-test: ## connector 的单元测试
	cd connector && npm ci && npm test

api-docs: ## 构建 API 文档站到 api-docs/dist
	cd api-docs && npm ci && npm run build

# ⚠️ 这一条**不需要 Android SDK**，这正是把 core 拆成独立构建的理由
# （见 android/README.md）。领域规则的测试都在这儿，几秒钟跑完。
android-core-test: ## Android 客户端的纯逻辑测试（不需要 Android SDK）
	cd android && ./gradlew -p core test

android-test: ## Android app 层的单元测试（需要 Android SDK）
	cd android && ./gradlew :app:testDebugUnitTest

android-apk: ## 构建 release APK（需要 Android SDK 与签名密钥环境变量）
	cd android && ./gradlew :app:assembleRelease
	@echo ">> 产物：android/app/build/outputs/apk/release/"
	@echo ">> 放到 ANDROID_APK_PATH 指向的路径，见 docs/08-deployment.md §5.5"

verify: ## v1 发布前的全量自检：Go + connector + web + Android 逻辑层 + 文档站
	$(MAKE) lint
	$(MAKE) test-db
	$(MAKE) connector-test
	$(MAKE) web-test
	$(MAKE) android-core-test
	$(MAKE) api-docs
	@echo ">> 全部通过"

# android-test / android-apk 不在 verify 里：它们要 Android SDK，
# 而 verify 是「任何一台开发机上都该能跑完」的那一档。SDK 相关的由 CI 保证。
