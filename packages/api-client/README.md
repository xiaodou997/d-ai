# @dai/api-client

由各后端服务的 **OpenAPI 契约**生成的 TypeScript 类型，供三端 portal 类型安全地调用后端。

## 契约生成链

```
Go struct (Huma code-first)
      │  go run ./cmd/openapi  (各服务)
      ▼
services/<svc>/api/openapi.yaml        ← 契约产物（提交入库，作为前后端契约快照）
      │  bun run --filter @dai/api-client generate  (openapi-typescript)
      ▼
packages/api-client/src/generated/<svc>.ts   ← TS 类型（不入库，可随时再生）
```

- **后端是唯一契约来源**：端点用 chi + Huma code-first 定义，OpenAPI 由代码导出，不手写 yaml。
- `src/generated/` 为生成产物，已 gitignore；CI 与本地按需重生。
- 运行时 client（鉴权、token 刷新、baseURL）在前端公共层（T4.2）补齐。

## 用法

```bash
# 1) 各服务导出契约（示例，待服务上线后可用）
#    cd services/urm-service && go run ./cmd/openapi > api/openapi.yaml

# 2) 生成 TS 类型（经 bunx 拉取 openapi-typescript，无需预装）
bun run --filter @dai/api-client generate
```
