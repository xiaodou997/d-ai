# D-AI Makefile

BINARY   := dai
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_DIR := release
FRONTEND_DIST := apps/portal/dist

.PHONY: dev build frontend embed clean migrate test typecheck

# ---- 本地开发 ----

dev: ## 启动后端（依赖 PostgreSQL + Redis）
	go run ./cmd/server

dev-frontend: ## 启动前端 dev server
	bun run dev:frontend

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

# ---- 数据库 ----

migrate: ## 执行数据库迁移
	go run ./cmd/migrate

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

help: ## 显示帮助
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
