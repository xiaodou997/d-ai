# Billing Outbox 积压与 parked row Runbook

`bill_charge_outbox` 是 AI 用量事实和账本扣费之间的可靠投递边界。用量行与 outbox
在同一事务提交；消费者随后把一条 charge 在一个 savepoint 中落到账户，并在同一
savepoint 里把用量从 `pending` 推进到 `settled`。因此，排查和恢复必须围绕
`request_id` 这个幂等键进行，不能删除、复制或改写成另一个请求。

## 状态语义

| 状态 | 含义 | 自动行为 |
| --- | --- | --- |
| `pending` | 已记录扣费意图，尚未成功落账 | 消费者每轮领取；单行失败会保留在队列中 |
| `done` | 账户扣费与用量结算在同一事务成功 | 不再领取；`settled_at` 已写入 |
| `failed` | 连续达到 10 次失败后 parked | 不再自动重试，必须先修复根因再受控 requeue |

失败在 savepoint 内回滚，所以 `failed` 行不会代表已经扣过一次钱；它代表这笔钱
尚未收回。`request_id` 唯一约束和 outbox/usage 的同事务写入保证重试不会产生第二笔
扣费。

## 多副本消费者保证

每个 `worker` 副本都可以运行一个 outbox consumer。消费者使用
`SELECT ... FOR UPDATE SKIP LOCKED` 在 PostgreSQL 中领取 `pending` 行；领取、账本扣费、
用量状态更新和 outbox=`done` 状态提交在同一事务内完成。因此并发副本不会重复处理同一行，
也不会因为队首坏数据阻塞后续行。`request_id` 唯一约束是第二道幂等保护，不能用进程内锁替代。

发布或扩容后，应使用两条独立 worker 实例并发执行一次 drain smoke，确认每条 request_id
最终只有一个 `done` 行、账户余额只减少一次；相关回归测试为
`TestConcurrentConsumersSettleEachChargeOnce`。

## 指标与告警

指标从管理监听的 `/metrics` 暴露（默认 `http://127.0.0.1:19642/metrics`）：

- `dai_billing_outbox_pending`：当前待结算行数。
- `dai_billing_outbox_failed`：parked 行数；正常应为 0。
- `dai_billing_outbox_oldest_pending_seconds`：最老 `pending` 行年龄。
- `dai_billing_outbox_applied_total`：成功落账的累计数量。
- `dai_scheduler_task_consecutive_failures{task="billing_reconciliation"}`：资金不变量对账任务连续失败数。

建议告警：

```promql
# parked row 是未收回的钱，任何非零都需要人工确认
dai_billing_outbox_failed > 0

# pending 持续超过两个消费者周期，说明结算停摆或数据库被阻塞
dai_billing_outbox_pending > 0
  and dai_billing_outbox_oldest_pending_seconds > 120

# 对账任务失败时，不能只看队列数字；同时升级资金一致性事件
dai_scheduler_task_consecutive_failures{task="billing_reconciliation"} > 0
```

`oldest_pending_seconds` 短暂尖峰可以随消费者恢复而回落；持续增长或
`failed > 0` 不应通过重启和删除队列来“消失”。

## 第一步：只读确认范围

先保存告警时间、实例、发布版本和 incident ID。所有排查 SQL 使用只读事务：

```sql
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;

SELECT status,
       COUNT(*) AS rows,
       MIN(created_at) AS oldest_created_at,
       MAX(now() - created_at) AS oldest_age,
       MAX(attempts) AS max_attempts
FROM bill_charge_outbox
GROUP BY status
ORDER BY status;

SELECT o.id, o.request_id, o.status, o.attempts, o.created_at,
       o.tenant_id, o.user_id, o.tenant_micro, o.user_micro,
       o.last_error, o.settled_at,
       u.billing_status, u.settlement_error, u.tenant_payable, u.user_charged,
       (ta.account_id IS NOT NULL) AS tenant_account_exists,
       (ua.account_id IS NOT NULL) AS user_account_exists
FROM bill_charge_outbox o
LEFT JOIN ai_usage_logs u ON u.request_id = o.request_id
LEFT JOIN bill_accounts ta ON ta.account_id = o.tenant_id
LEFT JOIN bill_accounts ua ON ua.account_id = o.user_id
WHERE o.status IN ('pending', 'failed')
ORDER BY o.id
LIMIT 100;

COMMIT;
```

若数据库本身连接异常，再查看是否有锁等待；不要在业务高峰执行会锁表的诊断：

```sql
SELECT pid, usename, application_name, state,
       wait_event_type, wait_event,
       now() - query_start AS query_age,
       LEFT(query, 240) AS query
FROM pg_stat_activity
WHERE datname = current_database()
  AND pid <> pg_backend_pid()
  AND state <> 'idle'
ORDER BY query_start;
```

## 分流处理

### `pending` 积压、`failed = 0`

1. 查看 `/health` 中的 worker 生命周期、应用日志中的 `billing outbox drain failed`，以及
   `dai_billing_outbox_oldest_pending_seconds` 是否持续增长。
2. 检查 billing pool 是否耗尽、PostgreSQL 是否有锁等待或连接错误；消费者会自动继续重试，
   不要手工修改 `status` 或 `attempts`。
3. 若 `last_error` 显示账户不存在、usage 不存在、金额/用户字段不合法，按 parked row
   流程处理；这不是单纯的暂时积压。
4. 数据库或进程恢复后，确认 `pending` 和最老年龄下降，且 usage 行最终变为 `settled`。

### 存在 `failed` parked row

1. 先按 `request_id` 保存上面的完整行快照、`last_error`、usage 行和账户状态；一行一个
   incident 记录。禁止批量清空 `failed`。
2. 分类根因：
   - `billing account not found`：先由身份/账户生命周期恢复合法 `bill_accounts` 行；
   - `mark usage ... not found` 或 usage 金额不一致：停止 requeue，进入资金不变量对账；
   - 权限/连接错误：先修复 billing role、连接池或发布配置；
   - 用户扣费缺少 `user_id`、金额为负等：修复产生该事实的代码或数据契约，不能用 SQL
     填一个猜测的用户。
3. 根因修复并经过审批后，一次只 requeue 一条；保留 incident ID、操作人、批准人和
   requeue 前快照。requeue 仍使用原 `request_id`，绝不复制出新请求。
4. requeue 后等待消费者成功处理，确认 outbox=`done`、usage=`settled`、账户只发生一次
   对应扣减，并确认 billing reconciliation 指标恢复为 0。

## 受控单行 requeue

没有根因修复和 incident ID 时不要执行。schema v24 提供的
`bill_requeue_parked_outbox` 函数要求 parked 状态、usage 状态和 `attempts` 同时匹配，
并在同一事务内恢复两行、写入不可变审计证据；把 `<REQUEST_ID>`、`<REPAIR_ID>`、
`<INCIDENT_ID>`、`<OPERATOR_ID>` 和 `<REASON>` 替换为经过审批的值，不能使用通配符：

```sql
BEGIN;

-- 先锁定并保存输出，核对金额、租户、用户和 last_error
SELECT o.*, u.billing_status, u.tenant_payable, u.user_charged
FROM bill_charge_outbox o
JOIN ai_usage_logs u ON u.request_id = o.request_id
WHERE o.request_id = '<REQUEST_ID>'
FOR UPDATE;

-- 只有 failed + usage.failed 才允许回到 pending；request_id 不变。
-- 返回值是不可变 bill_repair_audits.repair_id；同一幂等键重试会返回原 repair_id。
SELECT bill_requeue_parked_outbox(
  '<REQUEST_ID>', '<REPAIR_ID>', 'outbox-requeue:<REQUEST_ID>:<INCIDENT_ID>',
  '<OPERATOR_ID>', '<REASON>');

-- 必须看到同一 request_id 的两行都是 pending 后再提交
SELECT o.request_id, o.status, o.attempts,
       u.billing_status, u.settlement_error
FROM bill_charge_outbox o
JOIN ai_usage_logs u ON u.request_id = o.request_id
WHERE o.request_id = '<REQUEST_ID>';

COMMIT;
```

如果任一前置状态不匹配、usage 不存在、账户仍不存在或金额无法解释，执行 `ROLLBACK`
并升级，不得强行把行改成 `done`。函数会把 incident ID 作为幂等键的一部分写入数据库内
不可变审计记录；仍应同步保留外部运维事件，且不允许批量自动 requeue。

## 恢复验收与禁止事项

恢复后至少确认：

```sql
SELECT o.request_id, o.status, o.attempts, o.last_error, o.settled_at,
       u.billing_status, u.settlement_error, u.settled_at AS usage_settled_at
FROM bill_charge_outbox o
JOIN ai_usage_logs u ON u.request_id = o.request_id
WHERE o.request_id = '<REQUEST_ID>';

SELECT account_id, balance_micro
FROM bill_accounts
WHERE account_id IN ('<TENANT_ID>', '<USER_ID>');
```

- 不得把 `failed` 直接改成 `done`，不得删除 parked row，不得修改 `request_id`。
- 不得通过手工增加/减少 `bill_accounts.balance_micro` 让对账变绿。
- 不得在根因未修复时批量重置 `attempts`，否则会重复打满数据库并掩盖真实问题。
- 若业务事实已经无法安全重放，保留 parked row 和错误证据，转入带审计证据的资金修复流程；
  不要静默丢弃这笔应收款。
