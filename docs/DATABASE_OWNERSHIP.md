# Database ownership contract

D-AI 仍使用单 PostgreSQL 数据库，但生产运行时不应继续使用超级用户或让所有模块共享同一个可写角色。
权限边界由可执行契约和两个连接池共同落实：主连接使用 runtime role，账务/支付/结算路径使用 billing role。

## Roles

- `dai`（runtime role）：服务 HTTP、AI gateway 和控制面；可读取账务投影及余额/额度批次，写普通运行时事实，并只能向 `bill_charge_outbox` 插入结算意图；原始支付/账务工作流表的 SELECT 已撤销。
- `dai_billing`（billing role）：结算、充值、退款、支付补偿和账本写入角色；拥有账务/支付工作流表。
- 迁移仍由 DBA/migrator 角色执行，不把 DDL 权限授予任一运行角色。

角色密码由部署 secret manager 注入，不写入仓库。生产必须配置独立的
`DAI_BILLING_DATABASE_URL`；应用完成连接切换并验证 billing role 可用后，才在生产库执行
revoke/ownership 变更。角色创建或轮换使用发布附件中的
`provision_db_roles.sh`，脚本会将两个角色固定为 LOGIN、NOINHERIT、NOSUPERUSER、
NOCREATEDB、NOCREATEROLE、NOREPLICATION、NOBYPASSRLS，并拒绝占位符、空白字符、
少于 32 个字符或相同密码。

账务和支付管理列表使用 `billing_recharge_order_projection`、
`payment_order_party_projection` 与 `payment_admin_recharge_order_projection` 只读视图；租户管理与
分析使用 `tenant_management_projection`、`tenant_self_overview_projection` 与
`tenant_usage_projection`，管理员终端用户列表使用 `user_admin_end_user_projection`。
billing/payment 三类视图和 `tenant_income_projection` owner 转给 billing role。账户统计使用
`system_account_stats_projection`，租户收入统计使用 `tenant_income_projection`，
避免运行时查询直接读取跨域账务表。租户/用户/运营的
`system_recharge_projection`、`system_identity_projection`、`system_balance_projection` 和
`system_usage_projection`、`system_account_stats_projection` 视图 owner 转给 runtime role，
避免报表查询直接依赖对方领域表。

## Apply

先确认目标库已完成 schema v25（包含上述只读视图和不可变资金修复审计表）升级；在维护窗口、应用已经配置并验证
billing DSN 后，以数据库 owner/superuser 执行角色 provisioning：

```bash
export DB_ROLE_PROVISION_DATABASE_URL='postgres://dai-admin:${PASSWORD}@db.example/dai?sslmode=require'
export DB_ROLE_PROVISION_RUNTIME_PASSWORD='由 secret manager 注入的高熵密码'
export DB_ROLE_PROVISION_BILLING_PASSWORD='由 secret manager 注入的另一组高熵密码'
deploy/production/provision_db_roles.sh preflight
export DB_ROLE_PROVISION_CONFIRM=APPLY
deploy/production/provision_db_roles.sh apply
```

角色连接验证通过后，停止所有应用实例，在同一维护窗口执行 ownership/revoke 切换：

```bash
export DB_OWNERSHIP_CUTOVER_ADMIN_DATABASE_URL='postgres://dai-admin:${PASSWORD}@db.example/dai?sslmode=require'
export DB_OWNERSHIP_CUTOVER_RUNTIME_DATABASE_URL='postgres://dai:${RUNTIME_PASSWORD}@db.example/dai?sslmode=require'
export DB_OWNERSHIP_CUTOVER_BILLING_DATABASE_URL='postgres://dai_billing:${BILLING_PASSWORD}@db.example/dai?sslmode=require'
export DB_OWNERSHIP_CUTOVER_CONFIRM=APPLY
export DB_OWNERSHIP_CUTOVER_WINDOW=OPEN
deploy/production/cutover_db_ownership.sh preflight
deploy/production/cutover_db_ownership.sh apply

# 启动新版本单实例后执行；health URL 可由 secret manager/部署环境注入
export DB_OWNERSHIP_CUTOVER_HEALTH_URL='https://portal.example.com/ready'
deploy/production/cutover_db_ownership.sh verify
```

发布附件包含 `ownership.sql`、`provision_db_roles.sh`、`apply_db_ownership.sh` 和 `cutover_db_ownership.sh`。provisioning 只创建/轮换运行角色和数据库 CONNECT 权限，不授予表权限；cutover wrapper 会验证三个 DSN 的实际角色、数据库一致性、最小角色属性和无活动会话，再调用 ownership 脚本在同一事务中撤销 runtime 账务 DML、授予 billing DML、转移账务表 owner。直接调用 `apply_db_ownership.sh` 仅用于 CI 探针或已获 DBA 审批的低层操作。

## Contract probe

```bash
SCHEMA_OWNERSHIP_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15432/dai_v25_test?sslmode=disable' \
  bash scripts/check_db_ownership.sh
```

探针使用临时 schema 和临时 NOLOGIN 角色，不接触应用生产角色；它验证：

- runtime 可以读取账务投影，但更新 `bill_accounts` 失败；
- runtime 读取原始 `pay_cash_ledger` 等支付/账务工作流表失败；
- runtime 可以在用量事务中插入 `bill_charge_outbox`；
- runtime 插入租户或终端用户时，security-definer provisioning trigger 仍能创建对应账务账户；
- billing role 可以写入并更新 `bill_accounts`；
- billing role 可以读取 `ai_usage_logs`、`ai_sub_orders` 和 `ai_sub_subscriptions`，供结算与资金不变量对账使用；
- billing role 只能向 `bill_repair_audits` 插入修复证据，数据库触发器拒绝修改或删除历史记录；
- billing role 可执行 `bill_requeue_parked_outbox(...)`，函数只接受状态/金额/账户均匹配的单行 parked 修复并以原 `request_id` 幂等；
- billing role 可以读取账务/支付投影视图，但读取无关 AI catalog 表失败；
- runtime role 可以读取租户与管理员终端用户投影视图；
- runtime role 可以读取账户统计与租户收入投影视图；
- 账务表 owner 和 grant/revoke 契约可重复执行。

该探针已纳入 CI。composition root 已通过 `DAI_BILLING_DATABASE_URL` 装配独立 billing pool，
billing/payment/outbox/订阅扣费事务也已切到该连接；生产应用权限切换前由部署方先执行
`provision_db_roles.sh preflight/apply`，验证两个 DSN 可以登录且角色属性合规，再在维护窗口执行
`apply_db_ownership.sh`。应用恢复流量前必须重新检查 readiness、billing 查询和 outbox 入队。
