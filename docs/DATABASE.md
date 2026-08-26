# D-AI 数据库维护

## 当前基线

- `internal/db/init.sql` 是当前唯一完整结构，schema 版本为 `23`。
- 初始化脚本只允许在空 PostgreSQL schema 中执行，不能用于覆盖或修复已有数据库。
- 应用启动只校验 `dai_schema_metadata.version`，不会执行 DDL 或升级 SQL。
- `internal/db/changes/` 存放首次发布后的人工升级 SQL。
- 结构调整必须直接更新完整基线并保持 `internal/db/schema.go` 中的期望版本一致。
- 运行角色与账务表所有权契约见 [`docs/DATABASE_OWNERSHIP.md`](DATABASE_OWNERSHIP.md)；生产必须配置独立 billing pool，并在角色 provisioning 后再执行 revoke/owner 变更。

已有数据库必须从当前版本开始，按编号连续执行 `internal/db/changes/` 中的升级脚本；
不能通过直接修改版本号跳过升级。不需要保留数据的本地开发库可以使用
`make db-recreate` 按最新基线重建。

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
- `internal/billing/invariants.Check` 是统一只读资金核查入口，检查账户余额与活跃批次守恒、批次状态、
  充值撤销、Outbox/用量链接、退款冲正、订阅订单和订阅额度边界。它既可在真实 PostgreSQL 测试中运行，
  也可由后续对账任务在一个事务快照中调用。
- 账本写路径统一遵循「先锁 `bill_accounts`，再锁 `bill_credit_lots`」的顺序；充值、扣费、过期、撤销和退款
  即使并发交错也不会形成反向锁等待。固定种子的随机并发属性测试会持续验证这一约定。
- Scheduler 每 15 分钟在 Repeatable Read 只读快照中运行 `billing/invariants.Check`，并用事务级 advisory lock
  保证多副本只执行一份。管理 `/metrics` 的最低告警组合为：
  `dai_billing_reconciliation_violations > 0`（发现差异）、
  `time() - dai_billing_reconciliation_last_run_timestamp_seconds > 1800`（对账停滞），以及
  `dai_scheduler_task_consecutive_failures{task="billing_reconciliation"} > 0`（任务失败）。差异只允许通过
  保留审计证据的修复流程处理，禁止直接改余额或批次绕过账本。
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
的并发请求不再因为账户行锁而串行结算。积压和 parked row 的处理边界见
`docs/BILLING_OUTBOX_RUNBOOK.md`，禁止删除队列行或绕过原始 `request_id` 重放。

### 透支的边界

结算是后付费的，所以准入拦得住「下一个请求」，拦不住「已经在跑的请求」。最坏透支是：

```text
最大透支 ≈ 单账户在途并发上限 × 单请求最高成本
```

因此 `runtime.default_in_flight_per_account`（默认 32，可用
`DAI_RUNTIME_DEFAULT_IN_FLIGHT_PER_ACCOUNT` 覆盖）不是流量整形，而是**让这个乘积有限**。
在 `ai_runtime_limit_policies` 里为某个 scope 显式配置了 `concurrency_limit` 时，
该 scope 使用配置值而不再叠加默认值。设为 `0` 会关掉默认上限，透支随之重新变得无界。

### 失败请求的计费口径

- 已经调用到上游 → 按上游返回的 token 照常计费（成本已真实发生，平台不替用户垫）。
- 完全没调用到上游（`attempts == 0`）→ 全额置零。
- 图片/视频这类「交付物」计量在失败时置零，与 token 不同。

这三条由 `internal/ai/adapters/postgres/pricebook_billing_test.go` 中的
`TestFailedRequest*` / `TestUnattemptedRequestIsFree` 锁定。

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
    ├── changes/
    ├── rollback/
    └── schema_release.sh
```

首次部署时，数据库管理员使用 `psql`、DBeaver、DataGrip 或其他 PostgreSQL
连接工具，在空 schema 中执行 `release/sql/init.sql`。应用随后以只具备运行期所需
权限的账号启动。需要升级已有库时，使用同一发布附件中的
`release/sql/schema_release.sh`，详见 [`docs/SCHEMA_RELEASE_RUNBOOK.md`](SCHEMA_RELEASE_RUNBOOK.md)。

## 首次发布后的结构变更

首次发布后，任何需要保留已有数据的结构变化都必须同时完成：

1. 在 `internal/db/changes/` 新增单向 SQL，命名为 `NNNN_YYYYMMDD_description.sql`。
2. `NNNN` 使用升级后的目标 schema 版本，版本必须连续且不可重复。
3. SQL 开头获取 advisory lock 并校验来源版本，在单个事务的最后更新目标版本和 `updated_at`。
4. 同步修改 `internal/db/init.sql`，使新数据库直接得到相同的最终结构。
5. 更新 `internal/db/schema.go` 中的 `ExpectedSchemaVersion`。
6. 在空数据库验证完整基线，在目标版本的数据库副本验证升级 SQL。
7. 发布时备份数据库，在维护窗口按版本顺序人工执行，再启动新应用。

例如数据库从版本 `3` 升级到版本 `7`，必须依次执行：

```text
0004_YYYYMMDD_description.sql
0005_YYYYMMDD_description.sql
0006_YYYYMMDD_description.sql
0007_YYYYMMDD_description.sql
```

应用版本与数据库版本不一致时会拒绝启动；它不会自动修复，也不能通过直接修改
`dai_schema_metadata.version` 跳过升级。

### 支付补偿退避

schema v17 为 `pay_orders` 增加 `sweep_attempts`、`sweep_next_attempt_at`、
`sweep_last_attempt_at` 和 `sweep_last_error`，schema v18 为重试健康统计增加部分索引，
schema v19 修复历史 v1→v18 链遗漏的 `ai_usage_logs.billing_status` 索引，schema v20 增加 billing/payment 跨域只读投影视图，schema v21 增加租户管理与分析只读投影视图，schema v22 增加管理员终端用户只读投影视图，schema v23 增加运营仪表盘只读投影视图。
支付 provider 或补偿入账失败会按
1 分钟起步、指数增长、最多 1 小时的退避写入下一次尝试时间；非终态的
`USERPAYING/NOTPAY` 查单结果使用 5 分钟延后但不增加失败次数。成功入账或关单会清理
这些字段。这样 scheduler 的 5 分钟任务超时和 advisory lock 只负责单轮执行租约，订单
重试节奏由数据库持久化，进程重启或副本切换不会把 provider 故障放大为每分钟请求。

升级已有数据库时按顺序执行 `internal/db/changes/0017_20260824_payment_sweep_backoff.sql`、
`internal/db/changes/0018_20260825_payment_sweep_health_index.sql`、
`internal/db/changes/0019_20260826_repair_billing_status_index.sql` 和
`internal/db/changes/0020_20260826_cross_domain_read_models.sql` 和
`internal/db/changes/0021_20260826_tenant_read_models.sql` 和
`internal/db/changes/0022_20260826_user_read_models.sql` 和
`internal/db/changes/0023_20260826_system_read_models.sql`；不要直接修改
schema 版本号跳过脚本。

## 统一账号模型

所有可登录身份存储在 `iam_accounts`，由 `user_type` 区分超级管理员、平台管理员、
租户用户和终端用户。`iam_tenants` 是独立的租户业务实体。

- `user_id` 在所有账号类型中全局唯一。
- `username` 去除首尾空白后按小写全局唯一。
- 终端用户固定使用 `u_` 命名空间。
- 管理员和租户用户不能占用 `u_` 命名空间。
