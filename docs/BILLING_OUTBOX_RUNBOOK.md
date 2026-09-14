# 消费结算运行手册

schema 42 已删除旧 `bill_charge_outbox`、消费日志幂等及 billing window/lease/batch 机制。不要再运行旧 outbox 的重放、清理或修复 SQL。

唯一消费结算队列是 `bill_settlements`：`pending` 表示持久待结算，`posted` 表示实际事务已提交，`waived` 表示免收，`review` 表示冲突待处理，`refunded` 表示已追加反向资金流水。`review` 必须查明原因，不能删除或修改原用量、价格与应收快照来消除差异。

新请求在数据库不可用或最老待结算超过 30 秒时停止准入，后台保留原任务并重试。客户端断开取消上游，只依据取消前采集的证据封存；明确请求错误两层免收。重试只计最终有效尝试。

退款使用 `/api/v2/billing/settlements/{requestID}/refund`，由管理员填写原因；重复调用返回同一结果，不重复退款。请求诊断过期不影响 365 天内的逐笔退款，过期补偿使用独立余额调整。

停机升级、资产快照、独立备份和回滚边界见 [请求与账本切换手册](REQUEST_LEDGER_CUTOVER.md)。2026-09-14 不执行生产维护。
