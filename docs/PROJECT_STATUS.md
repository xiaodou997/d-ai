# D-AI 项目状态与实施清单

更新日期：2026-08-26

## 当前结论

D-AI 已统一为“单后端 + 单 Portal”工程结构。数据库、Go 后端、OpenAPI 导出、Portal 类型检查、生产构建和前端测试均已打通；剩余工作主要是运行时业务验收、契约细节校准和发布自动化。

## 已完成

- 身份、权限、计费和 AI 能力运行在同一个 Go 进程、PostgreSQL 数据库和二进制入口中。
- 数据库使用 `internal/db/init.sql` schema v21 完整基线；应用只校验 `dai_schema_metadata.version`，不执行生产迁移。
- `internal/db/changes/` 保存 v1→v21 的 forward-only 人工升级链，`cmd/checkschema` 校验连续版本、来源 guard、事务和基线版本，并在 CI 校验生成清单。
- `make dev` 负责本地配置、PostgreSQL、Redis、空库初始化和后端启动，不升级已有数据库。
- Portal 已从多端/多包结构合并为 `apps/portal` 一个项目；仓库不再有 `packages/*` workspace 包。
- API facade、领域类型、请求适配器、鉴权、shell、DsUI 和 billing 能力均在 `apps/portal/src` 内通过清晰目录边界组织。
- 旧服务注册、服务准入、服务令牌、跨服务 HTTP API 与 SSO 会话流程均已删除。
- `cmd/openapi` 从统一 Go route registration 导出 `contracts/openapi.yaml`。
- `apps/portal/scripts/generate.mjs` 只消费统一契约，并生成 `apps/portal/src/api/generated/dai.ts`；`ensure:api` 在契约或生成物缺失/过期时失败。
- 登录上下文已统一为单一 `portal`；后端根据账号凭证跨管理员、租户用户和终端用户表解析 `userType`，Portal 再按身份和权限生成菜单、路由与主题。

## 目录约定

| 边界 | 位置 | 责任 |
| --- | --- | --- |
| 后端 HTTP | `internal/transport`, `internal/ai/transport` | chi + Huma code-first 路由和 DTO |
| 后端契约 | `cmd/openapi`, `contracts/openapi.yaml` | 统一 OpenAPI 导出与快照 |
| Portal API | `apps/portal/src/api` | request adapter、业务 facade、领域类型、生成类型 |
| Portal 平台层 | `apps/portal/src/platform` | 环境、鉴权、路由、shell 和公共工作区 |
| Portal 设计系统 | `apps/portal/src/shared/ui` | token、DsUI 组件和布局 |
| Portal 业务层 | `apps/portal/src/features`, `apps/portal/src/views` | 领域工作区和 userType 页面 |
| 数据库 | `internal/db/init.sql`, `internal/db/changes`, `cmd/checkschema` | schema v21 新库基线、v1→v21 forward-only 人工变更和结构门禁 |

## 当前剩余任务

### P0：运行时产品验收

- 用真实本地管理员完成 admin、tenant、customer 三种 `userType` 的登录、动态菜单、动态主题和关键页面冒烟。
- 逐个校验 facade 的路径、请求参数、错误处理和分页字段与 `contracts/openapi.yaml` 及后端实际响应一致。
- 对 Portal 页面做一次浏览器级验收，重点覆盖登录、租户邀请/用户管理、AI 工作台、用量、计费和异步任务。

### P1：发布门禁

- 将 `make openapi`、`bun run ensure:api`、`bun run typecheck`、`bun run test`、`bun run build:frontend` 纳入 CI。
- 增加 `make build` 后的单二进制业务 `/health`、管理 `/ready`/`/metrics`、首页和 `/api/v1/info` smoke。
- 补充应用 Dockerfile、release 构建说明和跨平台构建矩阵。

### P1：测试环境统一

- 统一并文档化 Go 集成测试使用的 PostgreSQL/Redis 环境变量。
- 保持默认单元测试无需外部服务；真实数据库测试明确标注 skip 条件和 schema 初始化方式。

## 验证结果

| 命令/检查 | 结果 |
| --- | --- |
| `bun run typecheck` | 通过 |
| `bun run build:frontend` | 通过，产物写入 `cmd/server/frontend_dist` |
| `bun run test` | 55 个测试文件、191 个测试通过 |
| `make openapi` / `go run ./cmd/openapi` | 通过，生成统一 `contracts/openapi.yaml` |
| `bun run generate:api` | 通过，生成 `src/api/generated/dai.ts` |
| `make dev`、后端 health/management-ready/info | 旧本地数据卷需先 `make db-recreate` 后验证 |
| `go test ./...` | 通过 |

## 后续顺序

1. 完成真实浏览器流程和三类用户的运行时验收。
2. 校准 API facade 与统一 OpenAPI 契约的字段/错误/分页细节。
3. 将前后端检查接入 CI，并补单二进制 smoke。
4. 再做 Docker/release 和部署文档。
