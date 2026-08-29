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
        web web-test connector-test api-docs verify

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

docker-down: ## 停掉生产编排（数据卷保留；加 CLEAN=1 才删数据）
	$(COMPOSE) -f $(PROD_COMPOSE) --env-file .env down $(if $(CLEAN),-v,)

web: ## 构建管理控制台（会先按 openapi.yaml 重新生成类型）
	cd web && npm ci && npm run gen:api && npm run build

web-test: ## 控制台的类型检查与单元测试
	cd web && npm ci && npx tsc --noEmit && npx vitest run

connector-test: ## connector 的单元测试
	cd connector && npm ci && npm test

api-docs: ## 构建 API 文档站到 api-docs/dist
	cd api-docs && npm ci && npm run build

verify: ## v1 发布前的全量自检：Go + connector + web + 文档站
	$(MAKE) lint
	$(MAKE) test-db
	$(MAKE) connector-test
	$(MAKE) web-test
	$(MAKE) api-docs
	@echo ">> 全部通过"
