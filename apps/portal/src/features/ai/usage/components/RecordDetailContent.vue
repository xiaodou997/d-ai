<script setup lang="ts">
import { computed } from "vue";
import { DsTable, DsTag, type DsTableColumn } from "@/shared/ui";
import { chargeLabel, chargeReason, microUSD, tokenCount, type RequestRecord } from "../recordsApi";
import { deliveryLabel, ms, multiplier, object, receiptFacts, sourceLabel, timestamp, type RecordRole } from "../recordPresentation";
import RecordFacts from "./RecordFacts.vue";
const props = defineProps<{ record: RequestRecord; role: RecordRole }>();
const fact = (label: string, value: unknown, copy = false) => ({ label, value: value == null || value === "" ? "—" : String(value), copy: copy && value ? String(value) : undefined });
const overview = computed(() => {
  const r = props.record;
  return [fact("请求时间", timestamp(r.created_at)), fact("请求 ID", r.request_id, true), fact("请求模型", r.profile?.requested_model || r.model), fact("来源", sourceLabel(r.source)), fact("流式", r.stream ? "是" : "否"), fact("执行情况", !r.execution_available ? "执行详情已过保留期" : r.is_error ? "明确错误" : ["disconnected", "write_failed"].includes(r.delivery) ? "客户端交付中断" : r.delivery === "complete" ? "请求完成" : "未确认完整交付"), fact("响应交付", deliveryLabel(r.delivery)), fact("结算情况", chargeLabel(r)), ...(props.role === "admin" ? [fact("Trace ID", r.admin_context?.trace_id, true)] : [])];
});
const identity = computed(() => {
  const r = props.record, p = r.profile;
  return [
    ...(props.role === "admin" ? [fact("租户", p?.tenant_name || r.tenant_id), fact("租户 ID", r.tenant_id, true)] : []),
    ...(props.role !== "customer" ? [fact("调用用户", p?.user_name || r.user_id || "租户调用"), fact("用户 ID", r.user_id, true)] : []),
    fact("API Key", p?.api_key_name || (p?.api_key_id ? "名称不可用" : "非 Key 调用")), fact("Key ID", p?.api_key_id, true), fact("Key 尾号", p?.api_key_last_four ? `•••• ${p.api_key_last_four}` : undefined), fact("Key 归属", p?.key_owner_type === "user" ? "用户" : p?.key_owner_type === "tenant" ? "租户" : undefined), fact("认证方式", p?.auth_method), fact("分组", p?.group_name_snapshot || p?.group_id), fact("分组 ID", p?.group_id, true)
  ];
});
const amounts = computed(() => {
  const c = props.record.charge, refunded = c.state === "refunded";
  return [
    ...(props.role !== "customer" ? [fact("租户应付", microUSD(c.tenant_due_micro)), fact("租户实际扣款", microUSD(c.tenant_charged_micro)), fact("租户已退 / 净扣", `${microUSD(refunded ? c.tenant_charged_micro : 0)} / ${microUSD(refunded ? 0 : c.tenant_charged_micro)}`)] : []),
    fact("用户应付", microUSD(c.user_due_micro)), fact(props.role === "customer" ? "我的实际扣款" : "用户实际扣款", microUSD(c.user_charged_micro)), fact("用户已退 / 净扣", `${microUSD(refunded ? c.user_charged_micro : 0)} / ${microUSD(refunded ? 0 : c.user_charged_micro)}`),
    fact("API Key 额度消耗 / 退回", `${microUSD(c.api_key_used_micro)} / ${microUSD(refunded ? c.api_key_used_micro : 0)}`), fact("订阅额度消耗 / 退回", `${microUSD(c.subscription_used_micro)} / ${microUSD(refunded ? c.subscription_used_micro : 0)}`), fact("计费来源", c.source === "subscription" ? "订阅" : c.source === "payg" ? "按量计费" : c.source), fact("结算时间", timestamp(c.posted_at)), fact("计费原因", chargeReason(c.reason)),
    ...(props.role === "admin" ? [fact("上游参考成本", microUSD(c.reference_cost_micro))] : [])
  ];
});
const usage = computed(() => {
  const r = props.record, p = r.profile;
  return [fact("输入 Token", tokenCount(r.tokens.input)), fact("输出 Token", tokenCount(r.tokens.output)), fact("缓存读", tokenCount(r.tokens.cache_read)), fact("缓存写", tokenCount(r.tokens.cache_write)), fact("推理 Token", tokenCount(r.tokens.reasoning)), fact("总 Token", tokenCount(r.tokens.total)), ...(r.media_units != null ? [fact("媒体用量", `${r.media_units} ${r.media_unit_type}`), fact("规格", p?.resolution)] : []), fact("分组倍率", multiplier(p?.group_default_user_multiplier_snapshot)), fact("用户覆盖倍率", multiplier(p?.user_multiplier_override_snapshot)), fact("有效用户倍率", multiplier(p?.effective_user_multiplier_snapshot))];
});
const prices = computed(() => receiptFacts(props.record, props.role));
const hasMatchedPricing = computed(() => {
  const pricing = object(props.record.pricing), calculation = object(pricing.calculation);
  const line = object(pricing.user_breakdown || calculation.user_payable);
  return Object.keys(object(line.price_lines)).length > 0;
});
const timing = computed(() => {
  const r = props.record, a = r.admin_context;
  return [fact("首 Token", ms(r.first_token_ms)), fact("请求总耗时", ms(r.total_ms)), ...(props.role === "admin" ? [fact("网关准备", ms(a?.request_setup_ms)), fact("上游响应头", ms(a?.final_attempt_header_ms)), fact("首个响应字节", ms(a?.first_response_byte_ms)), fact("响应尾程", ms(a?.response_tail_ms))] : [])];
});
const routeFacts = computed(() => {
  const a = props.record.admin_context;
  return [fact("逻辑模型", a?.resolved_logical_model || props.record.model), fact("上游模型", a?.upstream_model), fact("响应模型", a?.public_response_model), fact("上游账号", a?.upstream_account_name || a?.upstream_account_id), fact("供应商", a?.provider_code), fact("端点 ID", a?.endpoint_id, true), fact("调度规则", a?.matched_dispatch_rule_summary), fact("入口 / 上游协议", `${a?.client_protocol || '—'} / ${a?.provider_format || '—'}`), fact("协议转换", a?.protocol_conversion_enabled ? "已启用" : "未启用"), fact("上游终态", a?.provider_terminal_state), fact("取消来源", a?.cancellation_origin), fact("User-Agent", a?.client_user_agent)];
});
const attempts = computed(() => (props.record.attempts || []).map((raw, index) => {
  const a = object(raw);
  const outcome = String(a.outcome || a.AvailabilityOutcome || "unknown");
  return { id: index, ordinal: a.ordinal ?? index + 1, account: a.account_name || a.AccountID || a.PoolID || "—", model: a.UpstreamModel || a.ModelCode || "—", priority: a.TargetPriority ?? "—", result: ({ success: "成功", failed: "失败", canceled: "取消", rejected: "计划拒绝", circuit_open: "熔断跳过", unknown: "未知" } as Record<string, string>)[outcome] || outcome, skipped: Number(a.ordinal) < 0, http: a.HTTPStatus || "—", timing: ms(typeof a.TotalMs === "number" ? a.TotalMs : null), reason: a.ErrorMsg || a.SelectionReason || "—" };
}));
const attemptColumns: DsTableColumn[] = [{ key: "ordinal", title: "顺序", width: 60 }, { key: "account", title: "账号 / 资源", width: 180, wrap: true }, { key: "model", title: "模型", width: 140 }, { key: "priority", title: "优先级", width: 65 }, { key: "result", title: "尝试结果", width: 100 }, { key: "http", title: "HTTP", width: 65 }, { key: "timing", title: "耗时", width: 110 }, { key: "reason", title: "说明", width: 230, wrap: true }];
const errorFacts = computed(() => { const r = props.record; return [fact("错误说明", r.error?.message || "诊断未采集或已过保留期"), fact("错误码", r.error?.code), fact("错误来源", r.error?.origin), fact("错误阶段", r.error?.stage), fact("HTTP 状态", r.http_status), ...(props.role === "admin" ? [fact("内部诊断", r.internal_detail), fact("结算诊断", r.charge.processing_detail)] : [])]; });
const refundFacts = computed(() => [fact("退款时间", timestamp(props.record.charge.refunded_at)), fact("退款原因", props.record.charge.refund_reason), ...(props.role === "admin" ? [fact("操作人 ID", props.record.charge.refund_operator, true)] : [])]);
</script>
<template>
  <div class="record-detail-content">
    <section class="record-detail-hero record-detail-wide">
      <div><span>请求详情</span><h2>{{ record.profile?.requested_model || record.model }}</h2><code>{{ record.request_id }}</code></div>
      <div class="record-detail-hero__tags"><DsTag tone="info">{{ sourceLabel(record.source) }}</DsTag><DsTag :tone="record.is_error ? 'danger' : 'positive'">{{ record.is_error ? '请求错误' : '请求完成' }}</DsTag><DsTag v-if="record.stream" tone="accent">流式</DsTag></div>
    </section>
    <RecordFacts class="record-detail-wide" title="请求概览" featured :facts="overview" />
    <RecordFacts title="调用主体" :facts="identity" />
    <RecordFacts title="用量与倍率" note="展示上游报告的用量；总 Token 遵循当前系统的协议语义，缓存和推理不能再次重复相加。未报告的字段不是零。" :facts="usage" />
    <RecordFacts title="费用与额度" :note="`金额单位 USD。实扣、退款与额度分别展示；Key 和订阅额度不能与账户扣款相加。${role === 'admin' ? '参考成本不代表实际采购成本。' : ''}`" :facts="amounts" />
    <RecordFacts title="性能" note="未记录的时间节点显示为 —。" :facts="timing" />
    <RecordFacts class="record-detail-wide" title="请求时计价依据" featured :note="hasMatchedPricing ? '仅展示本次请求实际命中的档位、用量和单价。' : '该请求未保存命中档位明细。'" :facts="prices" />
    <RecordFacts v-if="record.is_error || record.error || (role === 'admin' && record.charge.processing_detail)" class="record-detail-wide" title="错误与诊断" :facts="errorFacts" />
    <RecordFacts v-if="role === 'admin'" class="record-detail-wide" title="路由与尝试" :facts="routeFacts">
      <details class="detail-expand"><summary>上游尝试（{{ attempts.length }}）</summary><DsTable v-if="attempts.length" :columns="attemptColumns" :rows="attempts" row-key="id" :frame="false"><template #cell-result="{ row }">{{ row.skipped ? '跳过 · ' : '' }}{{ row.result }}</template></DsTable><p v-else>未记录上游尝试，或尝试详情已过保留期。</p></details>
    </RecordFacts>
    <RecordFacts v-if="record.charge.refunded_at" class="record-detail-wide" title="退款记录" :facts="refundFacts" />
    <details v-if="role === 'admin' && record.evidence" class="detail-expand record-detail-wide"><summary>原始计费证据与快照</summary><pre>{{ JSON.stringify({ evidence: record.evidence, pricing: record.pricing }, null, 2) }}</pre></details>
  </div>
</template>
<style scoped>
.record-detail-content { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 18px; min-width: 0; }
.record-detail-wide { grid-column: 1 / -1; }
.record-detail-hero { display: flex; justify-content: space-between; align-items: center; gap: 20px; padding: 22px 24px; border-radius: var(--ds-radius-panel); color: var(--ds-ink); background: linear-gradient(120deg, var(--ds-accent-soft), var(--ds-panel) 62%); border: 1px solid color-mix(in srgb, var(--ds-accent) 22%, var(--ds-line)); box-shadow: var(--ds-shadow-panel); }
.record-detail-hero span { color: var(--ds-muted); font-size: 11px; }
.record-detail-hero h2 { margin: 5px 0 7px; color: var(--ds-accent); font-family: var(--ds-font-mono); font-size: 20px; }
.record-detail-hero code { color: var(--ds-muted); font-family: var(--ds-font-mono); font-size: 11px; overflow-wrap: anywhere; }
.record-detail-hero__tags { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 8px; }
.detail-expand { margin-top: 16px; font-size: 13px; color: var(--ds-muted); }
.detail-expand summary { cursor: pointer; padding: 8px 0; color: var(--ds-ink); }
.detail-expand pre { white-space: pre-wrap; overflow-wrap: anywhere; max-height: 450px; overflow: auto; font-size: 12px; background: var(--ds-panel-muted); padding: 16px; }
@media (max-width: 900px) {
  .record-detail-content { grid-template-columns: minmax(0, 1fr); }
  .record-detail-wide { grid-column: auto; }
  .record-detail-hero { align-items: flex-start; flex-direction: column; }
  .record-detail-hero__tags { justify-content: flex-start; }
}
</style>
