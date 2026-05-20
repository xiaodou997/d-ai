# ai-service

Uni AI API 的 Go 网关服务，负责多供应商路由、协议透传、计费结算、API Key 管理。

## 启动

```bash
cp config.example.yaml config.yaml
# 编辑 config.yaml

go run ./cmd/server
# 或
make dev
```

## 构建发布

```bash
make build
# 输出到 release/ 目录
```

## 运行依赖

- PostgreSQL
- Redis（可选）
- URM / SSO 服务

## 运行时 API

- `GET /health`
- `GET /v1/models`
- `POST /v1/chat/completions`（openai_chat）
- `POST /v1/responses`（openai_responses）
- `POST /v1/messages`（anthropic_messages）
- `POST /v1/embeddings`（openai_embeddings）
- `POST /v1/images/generations`（openai_images）
- `POST /v1beta/models/{model}:{action}`（gemini）
- `POST /v1/messages/count_tokens`（Anthropic token 估算）

## 核心能力

- 运行时 API Key 认证（`sk-ai-` 前缀）
- 租户/用户模型授权与 Key 允许模型检查
- 严格 1:1 协议透传路由（client_protocol = upstream_protocol）
- 非流式 + SSE 流式透传
- URM HMAC Freeze→Confirm→Cancel 结算集成
- Redis 粘性路由、速率限制、端点冷却、配额预留
- Admin API：供应商、端点、模型、部署、价格、授权、Key、限速、审计日志

## 关键配置

| 环境变量 | 说明 |
|---------|------|
| `DATABASE_URL` | PostgreSQL DSN |
| `URM_BASE_URL` | URM 服务地址 |
| `URM_CLIENT_ID` | 本服务在 URM 的 client_id |
| `PROVIDER_KEY_MASTER` | Provider API Key 加密主密钥 |
| `REDIS_ADDR` | Redis 地址（留空禁用） |
| `SERVER_ADDR` | 监听地址，默认 `:13010` |
| `LOG_LEVEL` | 日志级别：debug / info / warn / error |

## 相关文档

- [Admin API](../docs/ADMIN_API.md)
- [本地冒烟测试](../docs/LOCAL_SMOKE.md)
- [后端用量架构](../docs/backend-usage-architecture.md)
