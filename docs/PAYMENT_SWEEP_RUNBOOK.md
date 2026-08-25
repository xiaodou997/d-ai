# 支付 Sweep 故障 Runbook

支付 sweep 负责两类补偿：过期订单查单/关单，以及订单创建超过 5 分钟仍未收到最终回调时的查单入账。它由 Scheduler 通过 PostgreSQL advisory lock 以单副本执行，但订单级 retry 状态存储在 `pay_orders`，因此进程重启和副本切换不会丢失退避窗口。

## 指标与告警

指标从管理监听的 `/metrics` 暴露，默认地址为 `http://127.0.0.1:19642/metrics`：

- `dai_payment_sweep_retry_orders`：仍处于 `created`、`paying` 或 `expired` 且至少发生过一次 sweep 失败的订单数。
- `dai_payment_sweep_due_retry_orders`：下一次重试时间已到的失败订单数。
- `dai_payment_sweep_oldest_retry_seconds`：最早一次失败距现在的秒数。
- `dai_payment_sweep_stats_errors_total`：读取支付重试健康统计失败次数。
- `dai_scheduler_task_consecutive_failures{task="payment_sweep"}`：sweep 调度任务连续失败次数。

建议告警（阈值可按业务量调整）：

```promql
# 有失败订单持续 10 分钟，说明 provider 或补偿链路需要关注
dai_payment_sweep_retry_orders > 0

# 失败订单超过 15 分钟未恢复；超过 2 小时建议升级为高优先级事件
dai_payment_sweep_oldest_retry_seconds > 900

# 到期重试积压且 scheduler 本身连续失败
dai_payment_sweep_due_retry_orders > 0
  and dai_scheduler_task_consecutive_failures{task="payment_sweep"} > 0
```

`retry_orders` 在没有失败订单时必须回到 `0`；`stats_errors_total` 增长表示数据库统计查询或连接池本身异常，不应把它误判为微信 provider 故障。

## 排查顺序

1. 查看 `dai_scheduler_task_consecutive_failures{task="payment_sweep"}`、Scheduler `/health` 和应用日志，确认是跨副本锁跳过、数据库连接失败、任务超时，还是 provider 调用失败。
2. 检查微信支付 API、证书/APIv3 密钥、网络出口和服务端时间；不要因为单笔订单失败就手工把订单改成 `paid`。
3. 在只读事务中查看 retry 队列分布：

   ```sql
   SELECT status,
          COUNT(*) AS orders,
          MIN(sweep_last_attempt_at) AS oldest_attempt,
          MIN(sweep_next_attempt_at) AS next_attempt,
          MAX(sweep_attempts) AS max_attempts
   FROM pay_orders
   WHERE status IN ('created', 'paying', 'expired')
     AND sweep_attempts > 0
   GROUP BY status
   ORDER BY oldest_attempt;
   ```

4. 恢复 provider 或数据库后，等待下一次持久化退避到期；不需要清空 retry 字段或重启全部副本。成功查单入账、成功关单会自动清除 retry 状态。
5. 已知单笔订单需要立即核实时，使用管理端“同步充值订单”动作，让 `PaymentService` 复用同一查单/入账状态机；先核对 provider 交易号和金额。

## 处理禁忌与恢复确认

- 不直接把 `status` 改成 `paid`，也不直接写入 `transaction_id` 或 `balance_order_id`；这会绕过账本和幂等约束。
- 不直接把大量订单的 `sweep_next_attempt_at` 改成当前时间来“加速恢复”，避免再次打爆 provider。若确需人工加速，应先获得 provider 恢复证据并分批操作、保留审计记录。
- provider 恢复后确认 `dai_payment_sweep_due_retry_orders` 下降、`dai_payment_sweep_oldest_retry_seconds` 回落，且成功订单的 `sweep_attempts` 已清零。
- 若 retry 数量继续增长而 Scheduler 连续失败，按数据库连接池/ advisory lock/管理监听故障处理；若 Scheduler 正常但 provider 错误持续，按外部支付渠道事件升级。

数据库结构升级必须按顺序执行：

1. `internal/db/changes/0017_20260824_payment_sweep_backoff.sql`
2. `internal/db/changes/0018_20260825_payment_sweep_health_index.sql`
