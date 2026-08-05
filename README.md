# D-AI

统一 AI 服务（单后端 + 单前端 + 单二进制部署）。

合并原 UniHub 的 urm-service（身份/计费/认证）+ ai-service（AI 网关）为单一服务，消除跨服务 HTTP 计费调用。三端前端合并为单一 portal，按 `userType` 动态菜单 + 动态主题。

## 项目状态

> 开发中：当前版本尚不能作为完整产品直接运行或部署。

本仓库于 2026-08-05 从 UniHub 的未跟踪 `v3/` 工作目录中独立出来，UniHub 基线提交为 `cc5bc36bc93a77c0eb7aa8bd4f654e6ad5d08ad4`。当前已知状态：

- Go 后端可以编译；全量测试仍有两项旧 schema 路径测试待迁移。
- 本地启动配置和数据库初始化流程尚未形成可复现的一键启动环境。
- 统一 Portal 仍有旧三端 API、类型和组件导入未完成迁移，生产构建暂不通过。
- Makefile、前端 embed 和独立发布流程仍需打通。

## 目录结构

```
v3/
├── go.mod                      # module xiaodou/dai
├── package.json                # bun workspace
├── Makefile
├── cmd/
│   └── server/
│       └── main.go             # 唯一二进制入口
├── internal/
│   ├── config/                 # 统一配置
│   ├── auth/                   # JWT / 黑名单 / 会话
│   ├── billing/                # 积分账本 / 租约 / 冻结确认
│   ├── user/                   # 用户管理
│   ├── tenant/                 # 租户管理
│   ├── invite/                 # 邀请码
│   ├── payment/                # 微信支付
│   ├── announcement/           # 公告
│   ├── clientsecret/           # 敏感配置加密
│   ├── system/                 # 统计分析
│   ├── legal/                  # 法律文件
│   ├── scheduler/              # 定时任务
│   ├── ai/                     # AI 网关核心
│   └── transport/              # 统一 HTTP 层
├── migrations/                 # 合并的 DB migration（全 public）
├── libs/                       # 后端公共库
├── apps/
│   └── portal/                 # 单一前端应用
└── packages/                   # 前端公共包
    ├── ui/                     #   @dai/ui 设计系统
    ├── app-core/               #   @dai/app-core
    ├── api-client/             #   @dai/api-client
    ├── auth/                   #   @dai/auth
    └── billing/                #   @dai/billing
```

## 本地开发

```bash
# 后端
go mod download
make dev

# 前端
bun install
make dev-frontend
```

## 构建

```bash
# 单二进制（含前端 embed）
make build

# 仅后端
make build-server
```

## 架构原则

- **单进程单二进制**：URM + AI 合并，计费进程内调用
- **单数据库**：全 public schema，单事务覆盖计费全链路
- **HTTP 层**：薄 chi + Huma（code-first），成功 = 强类型 2xx body，错误 = RFC 7807 `application/problem+json`
- **前端 embed**：`go:embed` 静态文件，真正单文件部署
