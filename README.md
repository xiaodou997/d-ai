# D-AI

统一 AI 服务（单后端 + 单前端 + 单二进制部署）。

D-AI 在一个进程中提供身份、权限、计费和 AI 能力，Portal 按 `userType` 动态展示菜单和主题。

## 项目状态

> 开发中：后端、数据库和 Portal 工程链已打通；完整产品仍需按业务流程做运行时验收。

本仓库于 2026-08-05 从 UniHub 的未跟踪 `v3/` 工作目录中独立出来，UniHub 基线提交为 `cc5bc36bc93a77c0eb7aa8bd4f654e6ad5d08ad4`。当前已知状态：

- Go 后端可在 `19641` 启动，PostgreSQL/Redis 和完整 schema 初始化可复现。
- 数据库使用首发前 schema v1 完整基线；应用只校验版本，不执行自动迁移。
- Portal 已合并为单一前端项目，API facade、领域类型、设计系统和运行时基础设施均位于 `apps/portal/src`。
- 后端按身份、计费和 AI 等业务域组织代码，对外只部署一个服务。
- OpenAPI 已由统一后端导出到 `contracts/openapi.yaml`，Portal 类型生成链已接通。

完整缺口、验收标准和待确认的产品决策见 [`docs/PROJECT_STATUS.md`](docs/PROJECT_STATUS.md)。

## 目录结构

```
./
├── go.mod                      # module xiaodou/dai
├── package.json                # 单一 Portal 前端依赖与脚本
├── Makefile
├── cmd/
│   ├── openapi/main.go         # 导出统一 OpenAPI 契约
│   └── server/main.go          # 唯一二进制入口
├── internal/
│   ├── config/                 # 统一配置
│   ├── auth/                   # JWT / 黑名单 / 会话
│   ├── billing/                # 有符号 USD 余额账本
│   │   ├── ledger/             #   余额唯一读写入口（负数即欠费）
│   │   └── outbox/             #   AI 扣费的可靠结算队列
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
├── internal/db/
│   ├── init.sql                # 合并后的唯一完整 schema
│   └── changes/                # 首次发布后的人工升级 SQL 与规范
├── libs/                       # 后端公共库
├── apps/
│   └── portal/                 # 单一前端应用与全部前端模块
│       ├── scripts/             # OpenAPI 类型生成与 freshness 检查
│       └── src/
│           ├── api/              # 统一请求层、facade、领域类型、生成类型
│           ├── platform/         # shell、鉴权、路由、公共业务能力
│           ├── shared/ui/        # DsUI 设计系统
│           ├── features/         # 可复用领域工作区
│           └── views/            # userType 路由页面
└── contracts/
    └── openapi.yaml             # 当前后端导出的单一契约快照
```

## 本地开发

需要 Go、Bun 和 Docker Compose。D-AI 后端监听 `http://localhost:19641`；本地 PostgreSQL/Redis 分别映射到 `15432/16379`，避免与其他项目的默认端口冲突。

```bash
# 首次准备
go mod download
bun install --frozen-lockfile

# 自动创建 config.yaml；空数据卷会初始化 PostgreSQL，再启动后端
make dev

# 另开终端启动前端开发服务器
make dev-frontend

# 停止本地依赖
make deps-down

# schema 调整后重建本地数据库（会清空 D-AI 本地数据）
make db-recreate
```

开发阶段使用过旧 schema 版本 `2` 到 `9` 的本地数据卷必须执行一次
`make db-recreate`；项目不保留这些首发前版本的升级链。

后端可用 `curl http://localhost:19641/health` 和 `curl http://localhost:19641/ready` 检查。Portal 开发服务器使用同一后端的 Vite 代理。
本地开发管理员和数据库维护规则见 [`docs/DATABASE.md`](docs/DATABASE.md)。

## 测试

```bash
# 拉起 PostgreSQL/Redis 并跑全量（含计费集成测试）
make test

# 只跑不依赖外部服务的测试；计费路径会 skip
make test-unit

make test-frontend
make typecheck
```

`make test` 设置 `DAI_TEST_DATABASE_STRICT=1`：数据库配置了却连不上时直接失败，
不会退化成 skip。计费正确性只在这些测试真的跑起来时才成立，一个全靠 skip 换来的
绿色是没有意义的。

## 构建

```bash
# 生成 release/dai，并附带 release/sql 数据库初始化和人工升级 SQL
make build

# 仅后端（当前可用于验证 Go 编译）
make build-server

# 生成生产 Docker 使用的 Linux amd64 二进制
make build-linux-amd64
```

## 文档

- [`docs/PROJECT_STATUS.md`](docs/PROJECT_STATUS.md)：项目现状、已完成重构、剩余验收项和实施顺序。
- [`docs/DATABASE.md`](docs/DATABASE.md)：完整 schema、人工变更和版本校验规则。
- [`apps/portal/README.md`](apps/portal/README.md)：Portal 目录、API facade 和契约生成说明。

## 架构原则

- **单进程单二进制**：身份、计费和 AI 领域统一运行，计费进程内调用
- **单数据库**：全 public schema，单事务覆盖计费全链路
- **人工 schema 维护**：新库执行完整 `init.sql`；首次发布后按版本人工执行升级 SQL；应用启动只校验 schema version
- **HTTP 层**：薄 chi + Huma（code-first），成功 = 强类型 2xx body，错误 = RFC 7807 `application/problem+json`
- **前端 embed**：`go:embed` 静态文件，真正单文件部署
