# D-AI Makefile

BINARY   := dai
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_DIR := release
FRONTEND_DIST := cmd/server/frontend_dist
DB_RELEASE_DIR := $(BUILD_DIR)/sql
LEGAL_RELEASE_FILES := LICENSE NOTICE THIRD-PARTY-LICENSES.md TRADEMARKS.md COMMERCIAL_LICENSE.md

.PHONY: dev dev-setup dev-frontend deps-up deps-down deps-logs db-version dev-seed db-recreate build build-server build-linux-amd64 database-artifacts legal-artifacts release-metadata release-smoke frontend embed clean test test-unit test-db-up test-billing-invariants test-frontend check-transport-coverage typecheck openapi generate-api ensure-api validate-frontend-quality check-module-deps check-authz check-schema check-schema-release replay-schema-chain check-db-role-provision check-db-ownership-cutover check-db-ownership help

# ---- Portal delivery ----

.PHONY: frontend-static portal-smoke portal-smoke-embed

.PHONY: frontend-static portal-smoke portal-smoke-embed

# ---- 本地开发 ----

dev: dev-setup ## 准备依赖并启动 all 角色后端
	go run ./cmd/server all

dev-role: dev-setup ## 按 ROLE 启动后端角色（ROLE=all|control-api|gateway|worker）
	go run ./cmd/server $(or $(ROLE),all)

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

build: frontend frontend-static embed database-artifacts legal-artifacts ## 构建静态 Portal、embed 二进制和发布附件
	@echo "Building $(BINARY) v$(VERSION)..."
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY) ./cmd/server
	@echo "Done: $(BUILD_DIR)/$(BINARY)"

build-server: database-artifacts legal-artifacts ## 只构建后端和发布附件（不含前端）
	@echo "Building server-only..."
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY) ./cmd/server
	@echo "Done: $(BUILD_DIR)/$(BINARY)"

build-linux-amd64: frontend frontend-static embed database-artifacts legal-artifacts ## 构建生产 Docker 使用的 Linux amd64 二进制与静态 Portal
	@echo "Building $(BINARY) linux/amd64 v$(VERSION)..."
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./cmd/server
	@echo "Done: $(BUILD_DIR)/$(BINARY)-linux-amd64"

database-artifacts: ## 将初始化、人工升级和回滚 SQL 复制到发布目录
	rm -rf $(DB_RELEASE_DIR)
	mkdir -p $(DB_RELEASE_DIR)
	cp internal/db/init.sql $(DB_RELEASE_DIR)/init.sql
	cp -R internal/db/changes $(DB_RELEASE_DIR)/
	cp -R internal/db/rollback $(DB_RELEASE_DIR)/
	cp deploy/production/schema_release.sh $(DB_RELEASE_DIR)/schema_release.sh
	cp internal/db/ownership.sql $(DB_RELEASE_DIR)/ownership.sql
	cp deploy/production/apply_db_ownership.sh $(DB_RELEASE_DIR)/apply_db_ownership.sh
	cp deploy/production/provision_db_roles.sh $(DB_RELEASE_DIR)/provision_db_roles.sh
	cp deploy/production/cutover_db_ownership.sh $(DB_RELEASE_DIR)/cutover_db_ownership.sh

legal-artifacts: ## 将开源许可、第三方通知和商标政策复制到发布目录
	mkdir -p $(BUILD_DIR)
	cp $(LEGAL_RELEASE_FILES) $(BUILD_DIR)/

frontend: ## 构建前端
	bun install --frozen-lockfile
	bun run build:frontend

frontend-static: ## 构建可由 CDN/反向代理托管的独立 Portal 制品
	bun install --frozen-lockfile
	bun run build:portal-static

portal-smoke: ## 校验独立 Portal 静态制品及 checksum
	bash scripts/smoke_portal.sh release/portal

portal-smoke-embed: ## 校验 embed Portal 二进制包含前端制品
	bash scripts/smoke_embed_portal.sh release/$(BINARY)

release-metadata: ## 生成 release SBOM、provenance 和 SHA256 清单（先完成构建）
	bash scripts/generate_release_metadata.sh release "$(VERSION)"

release-smoke: ## 验证已部署构建的业务、管理和可选流式端点
	bash scripts/smoke_release.sh "$(DAI_RELEASE_SMOKE_PUBLIC_URL)" "$(DAI_RELEASE_SMOKE_MANAGEMENT_URL)"

embed: ## 前端 dist 必须存在
	@if [ ! -d $(FRONTEND_DIST) ]; then \
		echo "ERROR: $(FRONTEND_DIST) not found. Run 'make frontend' first."; \
		exit 1; \
	fi

# ---- 测试 ----

# 计费路径的测试全部需要真实 PostgreSQL。没有数据库时它们会静默 skip，
# 所以 test 默认就把库拉起来——「动钱的代码没被验证过」不该是默认状态。
TEST_DATABASE_URL ?= postgres://postgres:postgres@127.0.0.1:15432/dai_test?sslmode=disable
TEST_REDIS_ADDR   ?= 127.0.0.1:16379

test: test-db-up ## 运行 Go 测试（含数据库集成测试）
	DAI_TEST_DATABASE_URL="$(TEST_DATABASE_URL)" \
	DAI_TEST_REDIS_ADDR="$(TEST_REDIS_ADDR)" \
	DAI_TEST_DATABASE_STRICT=1 \
	go test ./...

test-unit: ## 只跑不需要外部依赖的测试（数据库测试会 skip）
	go test ./...

test-billing-invariants: test-db-up ## 运行统一资金不变量生命周期测试
	DAI_TEST_DATABASE_URL="$(TEST_DATABASE_URL)" \
	DAI_TEST_DATABASE_STRICT=1 \
	GOCACHE="$(CURDIR)/.cache/go-build" \
	go test ./internal/billing/invariants -count=1 -v

test-db-up: ## 准备测试用 PostgreSQL/Redis 和 dai_test 库
	docker compose up -d --wait postgres redis
	@docker compose exec -T postgres psql -U postgres -tAc \
		"SELECT 1 FROM pg_database WHERE datname='dai_test'" | grep -q 1 || \
		docker compose exec -T postgres psql -U postgres -c "CREATE DATABASE dai_test"

test-frontend: ## 运行前端测试
	bun run test

check-transport-coverage: ## 校验 Transport 关键路径覆盖率不得回退
	bash scripts/check_transport_coverage.sh

typecheck: ## 前端类型检查
	bun run typecheck

validate-frontend-quality: ## 前端 any/颜色/架构契约
	bun run validate:frontend-quality
	bun run validate:portal-styles
	bun run validate:portal-architecture

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

check-module-deps: ## 校验 Go 模块依赖方向和已登记历史例外
	GOCACHE="$(CURDIR)/.cache/go-build" go run ./cmd/checkdeps

check-authz: ## 校验 OpenAPI operation 的 capability 授权矩阵覆盖率
	GOCACHE="$(CURDIR)/.cache/go-build" go run ./cmd/checkauthz

check-schema: ## 校验数据库完整基线与 forward-only 迁移链
	GOCACHE="$(CURDIR)/.cache/go-build" go run ./cmd/checkschema

check-schema-release: check-schema ## 校验发布期 schema 脚本语法和帮助入口
	bash -n deploy/production/schema_release.sh
	deploy/production/schema_release.sh --help >/dev/null

check-db-role-provision: ## 校验生产数据库角色 provisioning 脚本入口和确认门禁
	bash -n deploy/production/provision_db_roles.sh
	deploy/production/provision_db_roles.sh --help >/dev/null
	@if DB_ROLE_PROVISION_DATABASE_URL=postgres://invalid.example/dai deploy/production/provision_db_roles.sh apply >/dev/null 2>&1; then \
		echo "ERROR: role provisioning ran without explicit confirmation"; \
		exit 1; \
	fi

check-db-ownership-cutover: ## 校验生产数据库 ownership 切换脚本入口和确认门禁
	bash -n deploy/production/cutover_db_ownership.sh
	deploy/production/cutover_db_ownership.sh --help >/dev/null
	@if DB_OWNERSHIP_CUTOVER_ADMIN_DATABASE_URL=postgres://invalid.example/dai \
		DB_OWNERSHIP_CUTOVER_RUNTIME_DATABASE_URL=postgres://invalid.example/dai \
		DB_OWNERSHIP_CUTOVER_BILLING_DATABASE_URL=postgres://invalid.example/dai \
		deploy/production/cutover_db_ownership.sh apply >/dev/null 2>&1; then \
		echo "ERROR: ownership cutover ran without explicit confirmation"; \
		exit 1; \
	fi

replay-schema-chain: check-schema ## 在临时 PostgreSQL schema 中重放 v1 到当前基线
	bash scripts/replay_schema_chain.sh

check-db-ownership: ## 验证 runtime/billing 数据库角色与账务表越权失败
	bash scripts/check_db_ownership.sh

help: ## 显示帮助
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
