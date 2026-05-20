# Uni AI API — 统一 AI 网关代理服务

> Uni AI API 是一个 OpenAI 兼容的 AI 网关代理，提供多供应商路由、API Key 管理、用量计费、URM 结算等核心能力。

## 项目结构

```
uni-ai-api/
├── ai-service/      # 后端网关服务（Go + Gin + PostgreSQL）
├── ai-admin/        # 管理员前端（Vue 3 + Element Plus）
├── ai-tenant/       # 租户前端（Vue 3 + Element Plus）
├── ai-customer/     # 用户前端（Vue 3 + Element Plus）
├── docs/            # 项目文档
└── deployments/     # 部署配置
```

## 快速开始

### 环境要求

| 依赖 | 版本 |
|------|------|
| Go | 1.25+ |
| Node.js / Bun | Bun 1.0+ |
| PostgreSQL | 15+ |
| Redis | 7+（可选） |
| URM | 需提前部署 |

### 1. 启动后端

```bash
cd ai-service

# 复制配置模板并修改
cp config.example.yaml config.yaml
# 编辑 config.yaml，填入数据库、URM 地址等

# 启动
go run ./cmd/server
# 或
make dev
```

后端默认监听 `:13010`。

### 2. 启动前端

```bash
# 管理员后台
cd ai-admin && bun install && bun run dev     # http://localhost:13011

# 租户门户
cd ai-tenant && bun install && bun run dev    # http://localhost:13012

# 用户端
cd ai-customer && bun install && bun run dev  # http://localhost:13013
```

## 技术架构

### 后端（ai-service）

- **语言**：Go 1.25，module 名 `xiaodou/uni-ai-api`
- **框架**：Gin
- **数据库**：PostgreSQL（pgx/v5，连接池 pgxpool）
- **代码生成**：SQLC（`sqlc.yaml`）
- **缓存**：Redis（路由缓存、速率限制、配额预留）
- **认证**：依赖 URM 的 JWT（RS256）+ JWKS 公钥自动轮换
- **计费**：URM HMAC Freeze→Confirm→Cancel 结算集成
- **日志**：Uber Zap

### 前端（ai-admin / ai-tenant / ai-customer）

- Vue 3 + Composition API + `<script setup>`
- Pinia（状态管理）
- Vue Router
- Element Plus（UI）
- Tailwind CSS
- Bun（包管理和构建）

## 认证模型

Uni AI API 不自建用户体系，完全依赖 URM 认证：

- **SSO 登录**：前端通过 URM 的 OAuth2 授权码流程登录
- **JWKS 验签**：后端从 URM 拉取公钥，自动轮换
- **角色区分**：Admin / Tenant / Customer，通过 JWT 中的 `userType` 区分
- **API Key 认证**：运行时 API 使用 `sk-ai-` 前缀的 Bearer Token

## 运行时 API

| 端点 | 协议 | 说明 |
|------|------|------|
| `POST /v1/chat/completions` | openai_chat | OpenAI Chat Completions |
| `POST /v1/responses` | openai_responses | OpenAI Responses API |
| `POST /v1/messages` | anthropic_messages | Anthropic Messages |
| `POST /v1/embeddings` | openai_embeddings | OpenAI Embeddings |
| `POST /v1/images/generations` | openai_images | OpenAI Image Generation |
| `POST /v1beta/models/{model}:{action}` | gemini | Gemini Generate/Embeddings |
| `GET /v1/models` | - | 模型列表 |
| `GET /health` | - | 健康检查 |

## 产品策略

- 公共模型码为规范标识，供应商模型名通过 Deployment 映射
- 严格 1:1 协议透传：客户端协议必须与 Deployment 的 `upstream_protocol` 完全一致
- 供应商成本价用于审计和毛利报告
- 运行时计费使用平台和租户模型价格，以整数积分计算
- Token 接口按 Token 单位计费；图像接口按生成图片数量计费
- API Key 配额为本地管理；URM 为账户余额和积分结算的唯一来源
- 租户持有的 API Key 通过 URM 向租户收费；用户持有的 API Key 同时向租户和用户收费

## 文档

- [Admin API](./docs/ADMIN_API.md)
- [本地冒烟测试](./docs/LOCAL_SMOKE.md)
- [业务边界](./docs/BUSINESS_BOUNDARY.md)
- [后端用量架构](./docs/backend-usage-architecture.md)
- [前端认证](./docs/frontend-auth.md)

## 生产构建

```bash
cd ai-service
make build
# 输出到 release/ 目录
```

## License

Private — All rights reserved.
