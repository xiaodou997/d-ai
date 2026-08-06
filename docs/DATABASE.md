# D-AI 数据库维护

## 决策

D-AI 是尚未发布的新合并项目，不承担原 URM Service 和 AI Service 的线上数据库升级兼容。数据库采用完整基线模式：

- `internal/db/init.sql` 是全新数据库的唯一完整结构。
- 应用启动只连接数据库并校验 `dai_schema_metadata.version`，不会执行任何 DDL。
- `internal/db/changes/` 只保存确实需要在已有环境人工执行的单向变更。
- 结构调整必须同时更新完整基线；不能只新增变更脚本。
- 本地开发可以直接重建数据库，优先验证最终结构，而不是验证历史升级路径。

## 本地使用

```bash
# 首次启动依赖；空数据卷会执行 init.sql 和仅限本地的 dev_seed.sql
make dev-setup

# 查看应用要求的 schema 版本
make db-version

# 结构调整后清空本地数据并从完整基线重建
make db-recreate
```

`make db-recreate` 会删除 D-AI Compose 的 PostgreSQL 和 Redis 本地卷，只适用于开发环境。

本地 Compose 会额外执行 `internal/db/dev_seed.sql`。`make dev-seed` 可在已有本地数据卷上重复执行，初始化以下四类开发账号：

| userType | 用户名 | 密码 |
|---|---|---|
| 1 | `dai_admin` | `DaiAdmin123!` |
| 2 | `dai_platform_admin` | `DaiAdmin123!` |
| 3 | `dai_tenant` | `DaiAdmin123!` |
| 4 | `dai_user` | `DaiAdmin123!` |

这些凭据只用于本机开发。生产初始化不得挂载或执行 `dev_seed.sql`，应用进程也不会自动创建这些账号。

## 已有环境变更

当环境中已有需要保留的数据时，按以下顺序操作：

1. 在 `internal/db/changes/` 编写带日期的单向 SQL。
2. 同步修改 `internal/db/init.sql` 到相同最终状态。
3. 递增 schema version，并更新 `internal/db/schema.go` 的 `ExpectedSchemaVersion`。
4. 在数据库副本验证人工脚本，在空库验证完整基线。
5. 维护窗口中备份、执行、检查，再启动新应用。

应用版本与数据库版本不匹配时会拒绝启动；它不会尝试自动修复数据库。
