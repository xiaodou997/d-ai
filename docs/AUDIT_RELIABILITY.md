# 请求证据与调试可靠性

schema 42 起，请求证据由 `RequestStore` 在访问上游前登记，结束时与明确错误及持久结算任务一起事务封存。消费账目独立于请求诊断，删去诊断后仍可核账和退款。

完整正文默认不保存。管理员通过 `/api/v2/request-debug-sessions` 按租户、Key 或模型限时开启，默认 1 小时、最长 24 小时。正文脱敏、移除内嵌媒体后单独 gzip 保存，原始上限 1 MiB，最多保留 7 天。可选调试写入失败不影响资金事务；必要计费依据持久化失败则不能向客户端确认完成。

旧 `ai_audit_inbox`、`ai_request_payloads`、`ai_audit_blobs` 和审计 worker 已退役，不能继续运行旧的重放／归档操作。迁移步骤见 [切换手册](REQUEST_LEDGER_CUTOVER.md)，验证结果见 [验收记录](implementation/request-ledger-rebuild.md)。
