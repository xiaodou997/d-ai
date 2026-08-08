# D-AI 数据库维护

## 当前基线

D-AI 尚未发布第一个正式版本，不保留开发阶段的历史升级链：

- `internal/db/init.sql` 是当前唯一完整结构，schema 版本为 `1`。
- 初始化脚本只允许在空 PostgreSQL schema 中执行，不能用于覆盖或修复已有数据库。
- 应用启动只校验 `dai_schema_metadata.version`，不会执行 DDL 或升级 SQL。
- `internal/db/changes/` 暂无升级脚本，只保留首次发布后的人工变更规范。
- 结构调整必须直接更新完整基线并保持 `internal/db/schema.go` 中的期望版本一致。

开发阶段已有的旧本地数据卷使用过 schema 版本 `2` 到 `9`，需要执行一次
`make db-recreate`，不能通过修改版本号继续使用。

## 本地使用

```bash
# 空数据卷首次启动时，PostgreSQL 自动执行 init.sql 和本地 dev_seed.sql
make dev

# 查看当前 schema 版本
make db-version

# schema 调整后清空本地 PostgreSQL/Redis 数据卷并按最新基线重建
make db-recreate
```

`make db-recreate` 会删除 D-AI Compose 的本地数据，只允许在开发环境使用。

本地 Compose 会额外执行 `internal/db/dev_seed.sql`，初始化四类开发账号：

| userType | 用户名 | 密码 |
|---|---|---|
| 1 | `dai_admin` | `DaiAdmin123!` |
| 2 | `dai_platform_admin` | `DaiAdmin123!` |
| 3 | `dai_tenant` | `DaiAdmin123!` |
| 4 | `u_dai_user` | `DaiAdmin123!` |

这些凭据只用于本机开发。生产初始化不得执行 `dev_seed.sql`。

## 生产初始化

构建产物包含运行二进制和数据库 SQL 附件：

```text
release/
├── dai
└── sql/
    ├── init.sql
    └── changes/
```

首次部署时，数据库管理员使用 `psql`、DBeaver、DataGrip 或其他 PostgreSQL
连接工具，在空 schema 中执行 `release/sql/init.sql`。应用随后以只具备运行期所需
权限的账号启动。

## 首次发布后的结构变更

首次发布后，任何需要保留已有数据的结构变化都必须同时完成：

1. 在 `internal/db/changes/` 新增单向 SQL，命名为 `NNNN_YYYYMMDD_description.sql`。
2. `NNNN` 使用升级后的目标 schema 版本，版本必须连续且不可重复。
3. SQL 开头获取 advisory lock 并校验来源版本，在单个事务的最后更新目标版本和 `updated_at`。
4. 同步修改 `internal/db/init.sql`，使新数据库直接得到相同的最终结构。
5. 更新 `internal/db/schema.go` 中的 `ExpectedSchemaVersion`。
6. 在空数据库验证完整基线，在目标版本的数据库副本验证升级 SQL。
7. 发布时备份数据库，在维护窗口按版本顺序人工执行，再启动新应用。

例如数据库从版本 `3` 升级到版本 `6`，必须依次执行：

```text
0004_YYYYMMDD_description.sql
0005_YYYYMMDD_description.sql
0006_YYYYMMDD_description.sql
```

应用版本与数据库版本不一致时会拒绝启动；它不会自动修复，也不能通过直接修改
`dai_schema_metadata.version` 跳过升级。

## 统一账号模型

所有可登录身份存储在 `iam_accounts`，由 `user_type` 区分超级管理员、平台管理员、
租户用户和终端用户。`iam_tenants` 是独立的租户业务实体。

- `user_id` 在所有账号类型中全局唯一。
- `username` 去除首尾空白后按小写全局唯一。
- 终端用户固定使用 `u_` 命名空间。
- 管理员和租户用户不能占用 `u_` 命名空间。
