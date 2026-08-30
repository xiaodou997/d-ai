# D-AI 可观测性与 SLO Runbook

本手册只描述 D-AI 已发布的指标、日志和追踪信号，以及发生告警后的只读排查顺序。任何涉及账本、支付订单、Outbox 或数据库权限的修复，都必须回到对应的业务 runbook 执行，禁止直接修改生产数据。

## 信号入口

- 管理监听器：`/metrics`、`/health`、`/healthz`、`/ready`。
- HTTP 指标：`dai_http_requests_total`、`dai_http_request_duration_ms`、`dai_http_requests_in_flight`。`route` 使用 chi 路由模板，未知或未匹配请求统一标记为 `unmatched`，不会把用户 ID 等原始路径写进标签。
- AI/上游指标：`dai_ai_requests_total`、`dai_ai_request_duration_ms`、`dai_ai_pipeline_errors_total`、`dai_ai_circuit_breaker_open`。
- 计费和支付指标：`dai_billing_outbox_*`、`dai_payment_sweep_*`、`dai_billing_reconciliation_*`。
- 异步与审计指标：`dai_async_task_*`、`dai_ai_audit_inbox_*`。
- 数据库池指标：`dai_db_pool_*{pool="runtime|billing"}`，包括 acquired、idle、total、max、constructing、acquires、canceled acquires、empty acquires 和 acquire duration。
- 日志字段：`request_id`、`trace_id`、`span_id`、`method`、`path`、`status`、`latency`。入站 W3C Trace Context 会被提取并传递给请求 span、AI span 和结构化日志。

## 默认追踪策略

当配置了 `OTEL_EXPORTER_OTLP_ENDPOINT`（或更具体的 `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`）时，默认使用 parent-based trace-id ratio sampler，默认比例为 `0.1`。可通过标准变量覆盖：

```text
OTEL_TRACES_SAMPLER=parentbased_traceidratio
OTEL_TRACES_SAMPLER_ARG=0.1
OTEL_EXPORTER_OTLP_INSECURE=false
```

本地未配置 OTLP endpoint 时使用 no-op exporter，但仍保留入站传播语义。生产不应使用 `always_on`，除非是短时受控排障窗口。

## 建议 SLO 与告警

以下阈值是初始可执行基线，部署后按实际流量调整：

```promql
# API 5xx 错误率 > 1%，持续 10 分钟
sum(rate(dai_http_requests_total{status=~"5.."}[5m]))
/
sum(rate(dai_http_requests_total[5m])) > 0.01

# API P95 延迟 > 2 秒，持续 10 分钟
histogram_quantile(0.95,
  sum by (le) (rate(dai_http_request_duration_ms_bucket[5m]))) > 2000

# 数据库池连续耗尽：已获取连接达到最大连接数且仍有空池等待
sum by (pool) (dai_db_pool_acquired_connections)
>= sum by (pool) (dai_db_pool_max_connections)
and sum by (pool) (rate(dai_db_pool_empty_acquires_total[5m])) > 0

# Billing Outbox 有失败或积压
dai_billing_outbox_failed > 0
or dai_billing_outbox_oldest_pending_seconds > 300

# 异步审计死信
dai_ai_audit_inbox_dead > 0
```

## 排查顺序

1. 先按 `trace_id` 和 `request_id` 查看结构化日志，确认路由、状态码和失败阶段。
2. 再查看 `dai_http_*`、AI 上游和数据库池指标，区分入口延迟、上游失败、连接池耗尽或业务拒绝。
3. 涉及账务 Outbox、支付扫单、审计 inbox 或异步任务时，按照 [BILLING_OUTBOX_RUNBOOK.md](BILLING_OUTBOX_RUNBOOK.md)、[PAYMENT_SWEEP_RUNBOOK.md](PAYMENT_SWEEP_RUNBOOK.md) 和对应任务手册执行。
4. 只有在确认依赖恢复、积压停止增长且关键指标连续两个观察窗口正常后，才关闭告警。

## 发布前检查

- 管理监听器未暴露在公网业务端口；Prometheus 通过受控管理网络或认证代理访问。
- `/ready` 能够检查 runtime PostgreSQL、billing PostgreSQL（如独立配置）和 Redis。
- 生产追踪 sampler 使用 parent-based 配置，OTLP endpoint 和 TLS 配置与部署环境一致。
- 告警规则、联系人和本手册一起纳入发布制品或运维配置仓库。
