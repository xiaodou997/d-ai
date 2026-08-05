# D-AI Repository Workflow

- Work directly on `main` by default.
- Do not create or switch to a feature, fix, or task branch unless the user explicitly requests branch-based work.

# Project Overview

D-AI 是一个统一的 AI 服务平台，合并了原 UniHub 的 urm-service + ai-service 为单一后端，三端前端合并为单一 portal。

## Architecture

- **单后端**：Go + chi + Huma（code-first OpenAPI），单二进制，单 PostgreSQL 库（全 public schema）
- **单前端**：Vue 3 + Pinia + Tailwind + Vite，按 `userType` 动态菜单 + 动态主题
- **单部署**：前端 build 后 embed 进 Go 二进制，单文件部署
- **计费**：进程内函数调用（原跨服务 HTTP 调用已消除）

## Go module

- module: `xiaodou/dai`
- 所有代码在此 module 下，import 路径前缀 `xiaodou/dai/`

# Frontend Design System (DsUI)

三端（管理端 admin / 租户端 tenant / 用户端 customer）共享一套设计系统，颜色只由 token 决定。

- **Tokens**：`apps/portal/src/shared/ui/styles/base.css`。中性色阶、语义色、圆角、阴影、字体全部在此；每端只通过 `.ds-theme-{admin|tenant|customer}` 覆盖 accent 系（管理端紫 `#7c3aed` / 租户端蓝 `#2563EB` / 用户端赤陶 `#B0603C`）。新代码颜色只允许 `var(--ds-*)`，禁止硬编码 hex/rgba。
- **组件**（`apps/portal/src/shared/ui`）：DsTable / DsPagination / DsFilterBar / DsTag / DsMetricCard / DsTabs / DsEmpty / DsSkeleton / DsModal / DsDrawer / DsConfirmDialog。
- **页面骨架**：一律用 `PortalPagePanel`（`apps/portal/src/platform/page`）。
- **壳层**：`DsAppShell` / `DsTopbar` / `DsSidebar`（`apps/portal/src/shared/ui/layout`）。
- **动态主题**：登录后根据 JWT `user_type`（1=超管 / 2=平台管理员 / 3=租户 / 4=终端用户）切换 `.ds-theme-{admin|tenant|customer}`。

## User Types

| userType | 角色 | 主题 | 菜单可见性 |
|---|---|---|---|
| 1 | 超级管理员 | admin | 全部 |
| 2 | 平台管理员 | admin | 全部（除超管专属） |
| 3 | 租户 | tenant | 租户自助 + AI 运营 |
| 4 | 终端用户 | customer | 用户中心 + AI 工作台 |
