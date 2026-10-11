# D-AI Portal

Portal 是仓库唯一的前端项目。原 `packages/*` 的 UI、shell、鉴权、billing、API facade 和业务工作区均已合并到这里，不再通过 workspace package 暴露边界。

## 目录边界

- `src/api`：统一 request adapter、平台/AI facade、领域类型和 `generated/dai.ts`。
- `src/platform`：环境、鉴权、路由、shell、公共业务工作区和跨页面基础能力。
- `src/shared/ui`：DsUI token、基础组件和布局组件。
- `src/features`：按领域组织的可复用工作区。
- `src/views`：按 `userType` 组织的路由页面。
- `scripts`：统一 OpenAPI 契约的 TypeScript 类型生成与 freshness 检查。

## 首屏加载约定

Element Plus 模板组件和 `v-loading` 等指令由 Vite 的 `ElementPlusResolver` 按需导入，
样式随组件所在的 chunk 加载；不要在入口全量安装 Element Plus、注册整个图标库或导入
`element-plus/dist/index.css`。图标使用具名导入，中文配置由根组件的 `ElConfigProvider` 提供。
脚本中直接调用的 Message / MessageBox 样式由 `src/main.ts` 引入；手动导入的其他组件也需要
显式引入对应 `element-plus/es/components/<name>/style/css`。`DsTrendChart` 通过异步组件加载，
避免公共 UI 导出把 ECharts 带入应用壳。

两种生产构建均生成 Vite manifest，并自动执行 `scripts/validate-bundle-size.mjs`：
按入口、登录页、应用壳分别累计去重后的静态 JS/CSS 依赖及 gzip 大小，超出预算则构建失败。
应用壳预算不包含具体业务页面；图表等按需加载资源仍计入实际使用它们的页面。

## 常用命令

```bash
bun run dev:frontend
bun run typecheck
bun run test
bun run validate:frontend-quality
bun run validate:portal-styles
bun run validate:portal-architecture
bun run build:frontend
bun run build:portal-static
bun run build:portal-static
make openapi
bun run generate:api
bun run ensure:api
bun run test:e2e:install
bun run test:e2e
```

Portal 在 `/login` 直接提供统一的用户名密码表单，不要求用户选择管理端、租户端或用户端，也不接受单独的前端授权地址配置。后端根据账号凭证解析 `userType`，登录后的菜单、路由和主题再按身份与权限生效。

`bun run build:frontend` 生成后端 embed 所需的 `cmd/server/frontend_dist`；发布时使用
`bun run build:portal-static` 生成可由 CDN/反向代理托管的 `release/portal`，并写入
`release/portal/SHA256SUMS`。静态制品可运行 `make portal-smoke` 验收，embed 二进制可运行
`make portal-smoke-embed` 验收。

浏览器验收由 `e2e/portal.spec.ts` 驱动，默认使用本地 Vite dev server 和确定性 API fixture，覆盖四种
`userType` 登录/菜单、邀请码注册、跨端工作区、账务页面、token refresh、跨标签登出、权限变更以及
桌面/移动截图、Axe WCAG 无障碍、ARIA/键盘和控制台错误检查；mock provider 还会断言用户创建、API Key、流式对话、生图任务、充值、退款冲正、订阅购买和余额变化。运行真实后端时设置 `DAI_E2E_MOCK=0`
与 `DAI_E2E_BASE_URL`，并可通过 `DAI_E2E_*_USERNAME` / `DAI_E2E_*_PASSWORD` 覆盖开发账号；
CI 或新环境先运行 `bun run test:e2e:install` 安装 Chromium。

依赖安装和审计使用仓库根目录 `.npmrc` 指定的官方 npm registry；前端质量门禁会阻止新增未审查的显式 `any`、硬编码视觉值和 Portal 架构越界。

OpenAPI 唯一来源是根目录的 `contracts/openapi.yaml`，由 `cmd/openapi` 从当前 Go 后端的 Huma route registration 导出。运行时 facade 不直接暴露生成器细节，页面只从 `@/api` 和领域 facade 引用。

`bun run build:frontend` 生成后端 embed 所需的 `cmd/server/frontend_dist`；发布时使用
`bun run build:portal-static` 生成可由 CDN/反向代理托管的 `release/portal`，并写入
`release/portal/SHA256SUMS`。静态制品可运行 `make portal-smoke` 验收，embed 二进制可运行
`make portal-smoke-embed` 验收。
