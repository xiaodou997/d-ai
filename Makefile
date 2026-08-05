# D-AI Makefile

BINARY   := dai
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_DIR := release
FRONTEND_DIST := cmd/server/frontend_dist

.PHONY: dev dev-setup dev-frontend deps-up deps-down deps-logs db-version db-recreate build build-server frontend embed clean test test-frontend typecheck openapi generate-api ensure-api help

# ---- 本地开发 ----

dev: dev-setup ## 准备依赖并启动后端
	go run ./cmd/server

dev-setup: deps-up ## 初始化本地配置和依赖
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

db-recreate: ## 删除本地数据卷并用 init.sql 重建（会清空本地数据）
	docker compose down -v
	docker compose up -d --wait postgres redis

# ---- 构建 ----

build: frontend embed ## 构建单二进制（前端 embed）
	@echo "Building $(BINARY) v$(VERSION)..."
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY) ./cmd/server
	@echo "Done: $(BUILD_DIR)/$(BINARY)"

build-server: ## 只构建后端（不含前端）
	@echo "Building server-only..."
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY) ./cmd/server
	@echo "Done: $(BUILD_DIR)/$(BINARY)"

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
