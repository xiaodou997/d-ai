# Database ownership contract

D-AI 仍使用单 PostgreSQL 数据库，但生产运行时不应继续使用超级用户或让所有模块共享同一个可写角色。
权限边界由可执行契约和两个连接池共同落实：主连接使用 runtime role，账务/支付/结算路径使用 billing role。

## Roles

- `dai`（runtime role）：服务 HTTP、AI gateway 和控制面；可读取账务投影、写普通运行时事实，并只能向 `bill_charge_outbox` 插入结算意图。
- `dai_billing`（billing role）：结算、充值、退款、支付补偿和账本写入角色；拥有账务/支付工作流表。
- 迁移仍由 DBA/migrator 角色执行，不把 DDL 权限授予任一运行角色。

角色和密码由部署 secret manager 创建，不写入仓库。生产必须配置独立的
`DAI_BILLING_DATABASE_URL`；应用完成连接切换并验证 billing role 可用后，才在生产库执行
revoke/ownership 变更。

## Apply

在维护窗口、应用已经配置并验证 billing DSN 后，以数据库 owner/superuser 执行：

```bash
export SCHEMA_OWNERSHIP_DATABASE_URL='postgres://dai-admin:${PASSWORD}@db.example/dai?sslmode=require'
export SCHEMA_OWNERSHIP_RUNTIME_ROLE=dai
export SCHEMA_OWNERSHIP_BILLING_ROLE=dai_billing
export SCHEMA_OWNERSHIP_CONFIRM=APPLY
deploy/production/apply_db_ownership.sh
```

发布附件包含 `ownership.sql` 和 `apply_db_ownership.sh`。脚本会拒绝缺少角色、schema 或账务表的数据库，并在同一事务中撤销 runtime 账务 DML、授予 billing DML、转移账务表 owner。

## Contract probe

```bash
SCHEMA_OWNERSHIP_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15432/dai_v19_test?sslmode=disable' \
  bash scripts/check_db_ownership.sh
```

探针使用临时 schema 和临时 NOLOGIN 角色，不接触应用生产角色；它验证：

- runtime 可以读取账务投影，但更新 `bill_accounts` 失败；
- runtime 可以在用量事务中插入 `bill_charge_outbox`；
- billing role 可以写入并更新 `bill_accounts`；
- 账务表 owner 和 grant/revoke 契约可重复执行。

该探针已纳入 CI。composition root 已通过 `DAI_BILLING_DATABASE_URL` 装配独立 billing pool，
billing/payment/outbox/订阅扣费事务也已切到该连接；生产应用权限切换前仍需由部署方完成
`dai`/`dai_billing` 角色 provisioning，并在维护窗口执行契约脚本。
