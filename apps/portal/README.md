# D-AI Portal

Portal 是仓库唯一的前端项目。原 `packages/*` 的 UI、shell、鉴权、billing、API facade 和业务工作区均已合并到这里，不再通过 workspace package 暴露边界。

## 目录边界

- `src/api`：统一 request adapter、平台/AI facade、领域类型和 `generated/dai.ts`。
- `src/platform`：环境、鉴权、路由、shell、公共业务工作区和跨页面基础能力。
- `src/shared/ui`：DsUI token、基础组件和布局组件。
- `src/features`：按领域组织的可复用工作区。
- `src/views`：按 `userType` 组织的路由页面。
- `scripts`：统一 OpenAPI 契约的 TypeScript 类型生成与 freshness 检查。

## 常用命令

```bash
bun run dev:frontend
bun run typecheck
bun run test
bun run build:frontend
make openapi
bun run generate:api
bun run ensure:api
```

Portal 在 `/login` 直接提供统一的用户名密码表单，不要求用户选择管理端、租户端或用户端，也不接受单独的前端授权地址配置。后端根据账号凭证解析 `userType`，登录后的菜单、路由和主题再按身份与权限生效。

OpenAPI 唯一来源是根目录的 `contracts/openapi.yaml`，由 `cmd/openapi` 从当前 Go 后端的 Huma route registration 导出。运行时 facade 不直接暴露生成器细节，页面只从 `@/api` 和领域 facade 引用。
