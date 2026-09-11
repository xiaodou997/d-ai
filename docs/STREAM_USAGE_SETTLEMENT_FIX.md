# 文本用量结算与 sub2api 流式终态修复

2026-09-11；协议参考：用户提供的 sub2api 工作副本，HEAD `4726bdd08`。

## 行为变更

Chat、Embedding、Rerank 的同步、流式、透传和转换路径仅使用上游明确报告的 token。成功、失败、客户端断开和网关超时均保留已收集的报告值；不在客户端断开后继续后台推理。原价格表、缓存拆分、阶梯、倍率和微美元舍入保持不变。

- 无报告：`token_usage_source=missing`、`billing_status=void`、`billing_reason=missing_upstream_usage`。用户、租户、API Key、订阅额度都不扣减，不创建正金额扣费队列记录。
- 明确为零：`token_usage_source=upstream`，正常零金额记录，不能被估算覆盖。
- 部分字段：仅结算报告字段；累计快照不逐帧相加。Responses 的非零终态整体替换阶段快照；全零终态不覆盖此前非零报告，并记录 `ignored_zero_terminal`。
- `billing_breakdown.usage_evidence` 保留报告字段、协议、事件、终态及无效字段信息。旧 `mixed/estimated` 记录继续可读，历史金额不重算。
- 首个有效输出前的 Responses 元数据不会提交给客户端。只有尚未提交且没有非零报告用量的失败允许按现有预算重试；已报告用量的失败不自动重放。
- `error` 后续的 `response.failed` 可以补充 usage；失败、取消、不完整不能被 `[DONE]`、EOF 或转换器生成的成功结束帧覆盖。
- 超过 4 MiB 的文本审计降级为精简记录，保留错误和尝试诊断；`billing_breakdown.audit_compaction` 保存原始大小及裁剪字段。记录级别和敏感头脱敏设置仍生效，详细错误与上游请求 ID 仅在管理员诊断中使用。

图片、视频、音频及按次收费规则不变。无数据库迁移；API 增加 `missing` 来源及详情中的 `token_usage_source`。不修改 sub2api，不执行退款、补扣或历史数据修复。

## 验证

- 模拟此次约 5.6 MB 请求体、无 usage 的失败事件；不能再生成 187 万输入 token 或 99 输出 token。
- sub2api 的顶层/嵌套失败 usage（6/0、9/2）、错误双事件及中间夹杂元数据、失败后矛盾成功事件、元数据后失败、仅 SSE event 声明类型的 Responses→Chat/Messages 转换、缺失终态、重试隔离和禁止重放已计量请求。
- 空/缺失/无效/零/部分报告、重复及阶段/终态快照、缓存和推理 token 协议差异。
- 真实 PostgreSQL 使用真实 PriceBook、UsageLogger、outbox、ledger，按量和订阅各验证五种结果，并重复完成请求验证幂等；逐项核对余额、API Key 与订阅额度。既有事务回滚与资金不变量测试一同运行。
- 用量前端测试、类型检查、构建，以及 Go vet/build。

测试使用隔离 schema，并设置 `DAI_TEST_DATABASE_STRICT=1`，不会因数据库不可用而静默跳过。相关后端回归：

```sh
DAI_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15432/dai_test?sslmode=disable' \
DAI_TEST_DATABASE_STRICT=1 go test -p 1 \
  ./internal/ai/serving ./internal/ai/formats ./internal/ai/adapters/bridgefmt \
  ./internal/ai/audit ./internal/ai/transport ./internal/ai/observability/metrics \
  ./internal/ai/adapters/postgres ./internal/billing/... \
  -skip '^TestModelCatalogReaderTenantResourceProjection$' -count=1
```

全量检查限制：未修改的 HEAD 基线同样存在 `TestModelCatalogReaderTenantResourceProjection` 的 description/multiplier 扫描顺序错误、迁移 0029 的重复 priority 列、迁移 0038 的 description 视图依赖错误，以及公告数据库测试 schema 版本 4/40 不一致；模块依赖检查已有 `internal/transport -> pgx/v5` 禁止依赖。本修复不修改这些独立问题。并行数据库全量测试还出现过共享内存耗尽，单独串行重跑对应上游模型测试已通过；相关回归使用串行包执行。

## 发布与观察

本改动准备发布，不自动部署。使用正常构建发布流程；不需要新增配置或迁移，也不能通过延长超时解决计量错误。撤回代码版本会恢复旧计费漏洞，不应作为默认回滚方式。

上线后按路由观察：

- `dai_ai_usage_reports_total{source="missing"}`：上游缺失计量的请求。
- `dai_ai_missing_usage_charged_total`：必须保持零；包括余额与额度应扣金额的防线检查。
- `dai_ai_audit_compacted_total`：超大审计的降级数量。
- 既有请求失败率、重试次数、首字节及总耗时。

按发布时刻审计新记录（psql 的 `release_time` 变量设置为实际发布时刻）：

```sql
SELECT request_id, created_at, request_status, billing_reason,
       tenant_payable, user_charged, api_key_quota_cost, retail_base
FROM ai_usage_logs
WHERE created_at >= :'release_time'::timestamptz
  AND capability_type IN ('chat', 'embedding', 'rerank')
  AND token_usage_source = 'missing'
  AND (tenant_payable <> 0 OR user_charged <> 0
       OR api_key_quota_cost <> 0 OR retail_base <> 0);
```

预期返回零行。任何无 usage 但出现费用的记录都应按请求 ID 检查结算队列和用量证据，不直接修改历史账目。
