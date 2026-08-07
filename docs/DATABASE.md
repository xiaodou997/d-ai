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
| 4 | `u_dai_user` | `DaiAdmin123!` |

这些凭据只用于本机开发。生产初始化不得挂载或执行 `dev_seed.sql`，应用进程也不会自动创建这些账号。

## 统一账号模型

Schema 8 起，所有可登录身份统一存储在 `iam_accounts`，由 `user_type` 区分超级管理员、平台管理员、租户用户和终端用户。`iam_tenants` 仍是独立的租户业务实体。

- `user_id` 在所有账号类型中全局唯一，业务表继续使用它建立稳定关联。
- `username` 去除首尾空白后按小写全局唯一；登录匹配不区分大小写。
- 终端用户固定使用 `u_` 命名空间，创建和邀请注册会自动补齐前缀。
- 管理员和租户用户不能占用 `u_` 命名空间。
- 终端用户的昵称、头像、内部备注、冻结积分和透支状态仍在同一账号记录中，只对 `user_type = 4` 有业务含义。

`20260807_unify_accounts.sql` 将 schema 7 的三张旧账号表合并到该模型。管理员用户名保持不变，终端用户名补齐单个 `u_`；与管理员同名的租户用户增加 `t_`。脚本会在复制前检查最终用户名和 `user_id` 冲突，失败时整笔事务回滚。

## 已移除的 AI 应用层

Schema 9 彻底删除已下线的 AI 应用、托管提示词和应用运行密钥能力，包括相关数据表、`/v1/run` 运行入口及共享业务表中的应用专属字段。迁移会删除旧的应用会话和应用密钥异步任务；普通模型 API Key、模型工作区会话、使用记录和计费数据不受影响。

## 已有环境变更

当环境中已有需要保留的数据时，按以下顺序操作：

1. 在 `internal/db/changes/` 编写带日期的单向 SQL。
2. 同步修改 `internal/db/init.sql` 到相同最终状态。
3. 递增 schema version，并更新 `internal/db/schema.go` 的 `ExpectedSchemaVersion`。
4. 在数据库副本验证人工脚本，在空库验证完整基线。
5. 维护窗口中备份、执行、检查，再启动新应用。

应用版本与数据库版本不匹配时会拒绝启动；它不会尝试自动修复数据库。
