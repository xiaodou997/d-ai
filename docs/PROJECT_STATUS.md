# D-AI 项目状态与实施清单

更新日期：2026-08-30

## 当前结论

D-AI 已统一为“单后端 + 单 Portal”工程结构。P0 安全与生产正确性、P1 领域/数据库治理、P2 运行角色与 Portal 架构，以及 P3 可观测性、质量门禁、供应链和发布代码侧均已完成并通过自动化回归。真实支付 Provider、域名和证书属于上线后的环境验收，不作为本轮代码阻塞。

## 已完成

- 身份、权限、计费和 AI 能力运行在同一个 Go 进程、PostgreSQL 数据库和二进制入口中。
- 数据库使用 `internal/db/init.sql` schema v25 完整基线；应用只校验 `dai_schema_metadata.version`，不执行生产迁移。
- `internal/db/changes/` 保存 v1→v25 的 forward-only 人工升级链，`cmd/checkschema` 校验连续版本、来源 guard、事务和基线版本，并在 CI 校验生成清单。
- 平台管理端及 AI 管理端的状态变更、密钥/支付/Provider/路由/风控配置写操作统一要求 10 分钟内近期密码或 MFA 认证；只读查询保持可用。
- `gateway` 角色只注册 runtime/file/console 路由，Portal 仅由 `all` 和 `control-api` 提供；账户统计和租户收入统计统一读取 schema v25 只读投影视图。
- `make dev` 负责本地配置、PostgreSQL、Redis、空库初始化和后端启动，不升级已有数据库。
- Portal 已从多端/多包结构合并为 `apps/portal` 一个项目；仓库不再有 `packages/*` workspace 包。
- API facade、领域类型、请求适配器、鉴权、shell、DsUI 和 billing 能力均在 `apps/portal/src` 内通过清晰目录边界组织。
- 旧服务注册、服务准入、服务令牌、跨服务 HTTP API 与 SSO 会话流程均已删除。
- `cmd/openapi` 从统一 Go route registration 导出 `contracts/openapi.yaml`。
- `apps/portal/scripts/generate.mjs` 只消费统一契约，并生成 `apps/portal/src/api/generated/dai.ts`；`ensure:api` 在契约或生成物缺失/过期时失败。
- 登录上下文已统一为单一 `portal`；后端根据账号凭证跨管理员、租户用户和终端用户表解析 `userType`，Portal 再按身份和权限生成菜单、路由与主题。
- 浏览器 Mock 验收覆盖四种 `userType`、桌面/移动视口、Axe 无障碍和充值/退款冲正/订阅/AI 工作流；Portal 当前 137 个测试文件、400 个测试通过。
- HTTP 请求已接入 Prometheus 指标和 chi 路由模板标签；请求日志提取 W3C Trace Context 并记录 `trace_id`/`span_id`，数据库池、Outbox、支付扫单、异步任务和审计 inbox 均有可观测信号。
- 生产镜像默认使用非 root、只读根文件系统、临时目录和最小 Linux capability；release 可生成 SPDX SBOM、provenance 和 SHA256 清单。

## 目录约定

| 边界 | 位置 | 责任 |
| --- | --- | --- |
| 后端 HTTP | `internal/transport`, `internal/ai/transport` | chi + Huma code-first 路由和 DTO |
| 后端契约 | `cmd/openapi`, `contracts/openapi.yaml` | 统一 OpenAPI 导出与快照 |
| Portal API | `apps/portal/src/api` | request adapter、业务 facade、领域类型、生成类型 |
| Portal 平台层 | `apps/portal/src/platform` | 环境、鉴权、路由、shell 和公共工作区 |
| Portal 设计系统 | `apps/portal/src/shared/ui` | token、DsUI 组件和布局 |
| Portal 业务层 | `apps/portal/src/features`, `apps/portal/src/views` | 领域工作区和 userType 页面 |
| 数据库 | `internal/db/init.sql`, `internal/db/changes`, `cmd/checkschema` | schema v25 新库基线、v1→v25 forward-only 人工变更和结构门禁 |

## 本轮重构收口状态

### P3-01：统一可观测性

- [x] HTTP Prometheus middleware、路由模板标签、入站 Trace Context、trace ID 日志和 parent-based 可配置采样。
- [x] 请求、上游、计费、Outbox、异步任务、审计和数据库池指标及告警 runbook。

### P3-02：静态分析和格式门禁

- [x] 固定兼容当前 Go 工具链的 golangci-lint 版本和配置。
- [x] Portal 前端质量契约（显式 `any` 基线、DsUI 颜色、Portal 架构/依赖边界）。
- [x] 格式化、lint、vet、typecheck 和测试均纳入 CI。

### P3-03：供应链和发布验证

- [x] 固定 Go、Bun、Node、golangci-lint 和 govulncheck 版本。
- [x] Go/前端依赖审计、SPDX SBOM、provenance、制品 checksum、非 root 只读容器契约。
- [x] 独立 Portal、embed 二进制和发布元数据 smoke；迁移链、备份和回滚规则沿用现有发布手册。

### P3-04：文档同步

- [x] 本状态、P3 执行记录、可观测性和 release 说明已同步；真实支付环境验收仍按上线窗口执行。

## 上线后剩余任务

- [ ] 在目标环境执行真实 `/health`、`/ready`、Portal、API、流式响应和支付 Provider 验收；这项依赖正式域名、证书、回调和生产凭证，按上线窗口执行，不属于本轮代码开发。

## 验证结果

| 命令/检查 | 结果 |
| --- | --- |
| `bun run typecheck` | 通过 |
| `bun run build:frontend` | 通过，产物写入 `cmd/server/frontend_dist` |
| `bun run test` | 137 个测试文件、400 个测试通过 |
| `make openapi` / `go run ./cmd/openapi` | 通过，生成统一 `contracts/openapi.yaml` |
| `bun run generate:api` | 通过，生成 `src/api/generated/dai.ts` |
| `go run ./cmd/checkschema` | 通过，schema v1→v25、24 个 forward migration 连续且与基线一致 |
| `bun run validate:frontend-quality` | 通过；显式 `any` 仅允许在已审查基线内 |
| `golangci-lint` v2.12.2 + `staticcheck ./...` | 通过；未使用目录级或全局 ignore |
| `govulncheck ./...`（Go 1.26.6） | 通过；0 个受影响漏洞 |
| `make build-linux-amd64` + release metadata | 通过；生成 SBOM、provenance 和 SHA256 清单 |
| `make dev`、后端 health/management-ready/info | 本地可验证；目标环境真实验收在上线后执行 |
| `go test ./...` | 通过 |

## 后续顺序

1. 合并并运行 P3 CI 门禁，修复目标环境中出现的工具或依赖兼容问题。
2. 上线后执行真实 `/health`、`/ready`、Portal、API、流式响应和支付 Provider 验收。
