# D-AI Makefile

BINARY   := dai
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_DIR := release
FRONTEND_DIST := cmd/server/frontend_dist
DB_RELEASE_DIR := $(BUILD_DIR)/sql

.PHONY: dev dev-setup dev-frontend deps-up deps-down deps-logs db-version dev-seed db-recreate build build-server build-linux-amd64 database-artifacts frontend embed clean test test-frontend typecheck openapi generate-api ensure-api help

# ---- 本地开发 ----

dev: dev-setup ## 准备依赖并启动后端
	go run ./cmd/server

dev-setup: dev-seed ## 初始化本地配置、依赖、数据库和开发账号
	@test -f config.yaml || cp config.example.yaml config.yaml

dev-frontend: ## 启动前端 dev server
	bun run dev:frontend

deps-up: ## 启动 PostgreSQL 和 Redis
	docker compose up -d --wait postgres redis

deps-down: ## 停止本地依赖
	docker compose down

deps-logs: ## 查看本地依赖日志
	docker compose logs -f postgres redis

db-version: ## 查看当前数据库 schema 版本
	docker compose exec -T postgres psql -U postgres -d dai -c "SELECT version, initialized_at FROM dai_schema_metadata WHERE singleton = TRUE"

dev-seed: deps-up ## 幂等初始化本地 userType 1/2/3/4 测试账号
	docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U postgres -d dai < internal/db/dev_seed.sql

db-recreate: ## 删除本地数据卷并用 init.sql 重建（会清空本地数据）
	docker compose down -v
	docker compose up -d --wait postgres redis

# ---- 构建 ----

build: frontend embed database-artifacts ## 构建单二进制和数据库 SQL 发布附件
	@echo "Building $(BINARY) v$(VERSION)..."
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY) ./cmd/server
	@echo "Done: $(BUILD_DIR)/$(BINARY)"

build-server: database-artifacts ## 只构建后端和数据库 SQL 发布附件（不含前端）
	@echo "Building server-only..."
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY) ./cmd/server
	@echo "Done: $(BUILD_DIR)/$(BINARY)"

build-linux-amd64: frontend embed database-artifacts ## 构建生产 Docker 使用的 Linux amd64 二进制
	@echo "Building $(BINARY) linux/amd64 v$(VERSION)..."
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./cmd/server
	@echo "Done: $(BUILD_DIR)/$(BINARY)-linux-amd64"

database-artifacts: ## 将初始化和人工升级 SQL 复制到发布目录
	rm -rf $(DB_RELEASE_DIR)
	mkdir -p $(DB_RELEASE_DIR)
	cp internal/db/init.sql $(DB_RELEASE_DIR)/init.sql
	cp -R internal/db/changes $(DB_RELEASE_DIR)/

frontend: ## 构建前端
	bun install
	bun run build:frontend

embed: ## 前端 dist 必须存在
	@if [ ! -d $(FRONTEND_DIST) ]; then \
		echo "ERROR: $(FRONTEND_DIST) not found. Run 'make frontend' first."; \
		exit 1; \
	fi

# ---- 测试 ----

test: ## 运行 Go 测试
	go test ./...

test-frontend: ## 运行前端测试
	bun run test

typecheck: ## 前端类型检查
	bun run typecheck

# ---- 清理 ----

clean: ## 清理构建产物
	rm -rf $(BUILD_DIR)
	rm -rf $(FRONTEND_DIST)
	go clean -testcache

# ---- 辅助 ----

openapi: ## 导出 OpenAPI 文档
	go run ./cmd/openapi

generate-api: ## 根据统一 OpenAPI 契约生成 Portal 类型
	bun run generate:api

ensure-api: ## 校验 Portal 生成类型是否与统一契约一致
	bun run ensure:api

help: ## 显示帮助
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
