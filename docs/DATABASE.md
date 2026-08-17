# D-AI 数据库维护

## 当前基线

- `internal/db/init.sql` 是当前唯一完整结构，schema 版本为 `3`。
- 初始化脚本只允许在空 PostgreSQL schema 中执行，不能用于覆盖或修复已有数据库。
- 应用启动只校验 `dai_schema_metadata.version`，不会执行 DDL 或升级 SQL。
- `internal/db/changes/` 存放首次发布后的人工升级 SQL。
- 结构调整必须直接更新完整基线并保持 `internal/db/schema.go` 中的期望版本一致。

开发阶段已有的旧本地数据卷使用过 schema 版本 `2` 到 `9`，需要执行一次
`make db-recreate`，不能通过修改版本号继续使用。

## 余额模型

账户余额是 `bill_accounts.balance_micro` 这一个**有符号** BIGINT，单位 micro-USD
（$1 = 1,000,000）。它刻意没有 `CHECK`：**负数就是欠费**，不存在独立的欠费列。

```text
准入   balance_micro > 0
结算   balance_micro -= 成本      （可以变负，永不阻断）
充值   balance_micro += 金额      （负余额被同一次加法抵平）
```

这条规则的完整性来自「只有一个数」。此前余额由 `bill_credit_packages.remaining_credits`
和 `iam_tenants/iam_accounts.current_overdraft` 三个非负列共同表达，正负被拆开后没有
任何约束能表达「余额」，全仓 11 处各自拼装，其中两处还漏了过期过滤——「负余额仍能
发起请求」反复复发的根因就在这里。

配套约定：

- **只能通过 `internal/billing/ledger` 读写余额。** 任何地方再出现自行 SUM 额度包
  或读欠费列的 SQL，就是把第二个真相源引了回来。
- `bill_credit_lots` 记录每笔充值的来源和有效期，是**分摊明细，不是余额**。
  不变量：`SUM(未过期未撤销批次的 granted - consumed) = GREATEST(balance_micro, 0)`。
- 批次状态不落库，全部可推导：`expired_at` / `revoked_at` / `consumed >= granted`。
- 过期是 scheduler 每 5 分钟执行的一次**真实扣减**（`ledger.ExpireDueLots`），不是
  查询时的时间过滤，所以余额是一个只在被写入时才变化的确定值。
- 开户由 `bill_provision_account()` 触发器保证：新建租户或 `user_type = 4` 账号时
  自动建 `bill_accounts` 行。准入读不到账户会 fail-closed，所以这个不变量做成结构性
  约束，而不是依赖 6 处创建代码各自记得。

AI 运行时的扣费不直接改余额：请求结束时把扣费意图和 `ai_usage_logs` 写在**同一个
事务**里（`bill_charge_outbox`），由 `internal/billing/outbox` 的后台消费者用
`FOR UPDATE SKIP LOCKED` 落账。用量记录存在就意味着这笔钱一定会被扣到，同时同一租户
的并发请求不再因为账户行锁而串行结算。

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
