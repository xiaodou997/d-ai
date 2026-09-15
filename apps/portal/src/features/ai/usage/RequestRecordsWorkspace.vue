<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from "vue";
import { useRoute } from "vue-router";
import { ElMessage } from "element-plus";
import { ScrollText } from "lucide-vue-next";
import { useAuthStore } from "@/stores/auth";
import { PortalPagePanel, PortalMetricGrid } from "@/platform";
import { DsTabs, DsTable, DsDrawer, DsFilterBar, DsTag, type DsTableColumn } from "@/shared/ui";
import { recordsApi, chargeReason, chargeLabel, microUSD, tokenCount, type RequestRecord, type RecordKind, type RecordSummary, type RecordQuery } from "./recordsApi";

import { WORKBENCH_RANGE_OPTIONS, buildWorkbenchRangeWindow, getWorkbenchRangeOption, type WorkbenchRangeId } from "@/components/workbench/workbenchRanges";

const props = withDefaults(defineProps<{ initialTab?: RecordKind; requestId?: string; userId?: string }>(), { initialTab: "requests" });
const auth = useAuthStore();
const route = useRoute();
const admin = computed(() => [1, 2].includes(Number(auth.userInfo?.userType)));
const customer = computed(() => Number(auth.userInfo?.userType) === 4);
const tab = ref<RecordKind>(props.initialTab);
const model = ref("");
const source = ref("");
const tenantName = ref("");
const userName = ref("");
const group = ref("");
const apiKeyName = ref("");
const range = ref<WorkbenchRangeId>("today");
const customRange = ref<[Date, Date] | null>(null);
const appliedQuery = ref<RecordQuery>({});
const appliedRangeLabel = ref("今天");
const rows = ref<RequestRecord[]>([]);
const summary = ref<RecordSummary | null>(null);
const busy = ref(false);
const detailBusy = ref(false);
const detail = ref<RequestRecord | null>(null);
const open = ref(false);
const debugContent = ref("");
const debugBusy = ref(false);
const refundReason = ref("");
const refundBusy = ref(false);
const cursor = ref<string | undefined>();
const nextCursor = ref<string | undefined>();
const history = ref<Array<string | undefined>>([]);
let controller: AbortController | undefined;
let detailController: AbortController | undefined;
const receiptRows = computed(() => {
  const object = (value: unknown): Record<string, unknown> => value && typeof value === "object" ? value as Record<string, unknown> : {};
  const pricing = object(detail.value?.pricing), snapshot = object(pricing.snapshot);
  const entry = object(pricing.retail_price || snapshot.RetailEntry);
  const rows: Array<{ label: string; value: string }> = [];
  for (const [label, raw] of [["用户计费倍率", pricing.user_multiplier ?? snapshot.EffectiveUserMultiplier], ["订阅额度倍率", pricing.subscription_multiplier], ["租户结算倍率", pricing.tenant_multiplier]] as const) {
    if (typeof raw === "number") rows.push({ label, value: `× ${raw}` });
  }
  const tiers = Array.isArray(entry.TokenPriceTiers) ? entry.TokenPriceTiers.map(object) : [];
  const input = detail.value?.tokens.input;
  if (input != null) {
    const tier = tiers.find(item => item.up_to_input_tokens == null || input <= Number(item.up_to_input_tokens));
    if (tier) for (const [label, field] of [["输入基础价", "input_per_token"], ["输出基础价", "output_per_token"], ["缓存读基础价", "cache_read_per_token"], ["缓存写基础价", "cache_write_per_token"]]) {
      if (typeof tier[field] === "number") rows.push({ label, value: `$${(Number(tier[field]) * 1_000_000).toLocaleString("en-US", { maximumFractionDigits: 6 })} / 百万 Token` });
    }
  }
  return rows;
});
const tabs = [{ key: "requests", label: "请求记录" }, { key: "errors", label: "错误请求" }, { key: "settlements", label: "费用明细" }];
const columns = computed<DsTableColumn[]>(() => [
  { key: "time", title: "时间", width: 150 },
  { key: "model", title: "模型" },
  ...(!customer.value ? [{ key: "subject", title: "调用主体" }] : []),
  { key: "tokens", title: "已报告用量", width: 180 },
  { key: "charge", title: tab.value === "errors" ? "费用说明" : "实际费用", width: 160 },
  ...(tab.value === "errors" ? [{ key: "error", title: "错误说明" }] : [{ key: "timing", title: "耗时", width: 110 }]),
  { key: "detail", title: "详情", width: 70 }
]);
const metrics = computed(() => [
  { label: "请求次数", value: (summary.value?.requests || 0).toLocaleString(), hint: appliedRangeLabel.value },
  { label: "明确错误", value: (summary.value?.errors || 0).toLocaleString(), hint: "请求报错免收费用" },
  { label: "客户端中断", value: (summary.value?.interruptions || 0).toLocaleString(), hint: "按取消前用量结算" },
  { label: "实际扣款", value: microUSD(customer.value ? summary.value?.user_charged_micro : summary.value?.tenant_charged_micro), hint: `已退款 ${microUSD(customer.value ? summary.value?.user_refunded_micro : summary.value?.tenant_refunded_micro)}` }
]);
function query(): RecordQuery | null {
  const option = getWorkbenchRangeOption(range.value);
  const window = buildWorkbenchRangeWindow(option);
  if (range.value === "custom" && (!customRange.value || customRange.value[0] >= customRange.value[1])) {
    ElMessage.warning("请选择有效的起止时间");
    return null;
  }
  return {
    user_id: props.userId,
    model: model.value.trim() || undefined,
    tenant_name: admin.value ? tenantName.value.trim() || undefined : undefined,
    user_name: !customer.value && !props.userId ? userName.value.trim() || undefined : undefined,
    group: group.value.trim() || undefined,
    api_key_name: apiKeyName.value.trim() || undefined,
    source: source.value || undefined,
    from: range.value === "custom" ? customRange.value![0].toISOString() : window.date_from,
    to: range.value === "custom" ? customRange.value![1].toISOString() : window.date_to,
    limit: 20
  };
}
function resetFilters() {
  model.value = source.value = tenantName.value = userName.value = group.value = apiKeyName.value = "";
  range.value = "today"; customRange.value = null;
  void load(true);
}
async function load(reset = false) {
  if (reset || !appliedQuery.value.from) {
    const nextQuery = query();
    if (!nextQuery) return;
    appliedQuery.value = nextQuery;
    appliedRangeLabel.value = getWorkbenchRangeOption(range.value).label;
  }
  if (reset) { cursor.value = undefined; history.value = []; }
  controller?.abort(); const active = new AbortController(); controller = active; busy.value = true;
  try {
    const [page, total] = await Promise.all([recordsApi.list(tab.value, { ...appliedQuery.value, cursor: cursor.value }, active.signal), recordsApi.summary(appliedQuery.value, active.signal)]);
    if (active.signal.aborted) return;
    rows.value = page.records || []; nextCursor.value = page.next_cursor; summary.value = total;
  } catch (error) { if (!active.signal.aborted) ElMessage.error(error instanceof Error ? error.message : "读取记录失败"); }
  finally { if (!active.signal.aborted) busy.value = false; }
}
function changeTab(key: string) { tab.value = key as RecordKind; void load(true); }
function next() { if (!nextCursor.value) return; history.value.push(cursor.value); cursor.value = nextCursor.value; void load(); }
function previous() { if (!history.value.length) return; cursor.value = history.value.pop(); void load(); }
async function show(id: string) {
  detailController?.abort(); const active = new AbortController(); detailController = active;
  detail.value = null; debugContent.value = ""; open.value = true; detailBusy.value = true; refundReason.value = "";
  try { const row = await recordsApi.detail(id, active.signal); if (!active.signal.aborted) detail.value = row; }
  catch (error) { if (!active.signal.aborted) ElMessage.error(error instanceof Error ? error.message : "读取详情失败"); }
  finally { if (!active.signal.aborted) detailBusy.value = false; }
}
async function readDebug() {
  if (!detail.value) return;
  debugBusy.value = true;
  try { debugContent.value = JSON.stringify(await recordsApi.debug(detail.value.request_id), null, 2); }
  catch { debugContent.value = "未开启调试记录，或内容已过保留期。"; }
  finally { debugBusy.value = false; }
}
async function refund() {
  if (!detail.value || !refundReason.value.trim()) return;
  refundBusy.value = true;
  try { const id = detail.value.request_id; await recordsApi.refund(id, refundReason.value.trim()); await show(id); await load(); ElMessage.success("费用与相应额度已退回"); }
  catch (error) { ElMessage.error(error instanceof Error ? error.message : "退款失败"); }
  finally { refundBusy.value = false; }
}
function sourceLabel(value: string) { return ({ api_key: "API", web_chat: "网页对话", web_image: "网页生图", web_video: "网页视频" } as Record<string, string>)[value] || value; }
function date(value: string) { return new Date(value).toLocaleString("zh-CN", { hour12: false }); }
onMounted(() => { void load(); const id = props.requestId || String(route.params.requestId || ""); if (id) void show(id); });
onBeforeUnmount(() => { controller?.abort(); detailController?.abort(); });
</script>

<template>
  <PortalPagePanel fill :icon="ScrollText" :breadcrumbs="[{ label: '智能服务' }, { label: '请求与费用' }]" description="查看调用用量、明确错误与实际费用">
    <template #actions><el-button :loading="busy" @click="load(true)">刷新</el-button></template>
    <div class="records-workspace">
      <PortalMetricGrid :metrics="metrics" min-col-width="170px" />
      <DsTabs :tabs="tabs" :model-value="tab" @update:model-value="changeTab" />
      <DsFilterBar>
        <el-input v-model="model" clearable placeholder="模型 ID（完整编码）" aria-label="模型 ID" @keyup.enter="load(true)" />
        <el-input v-if="admin" v-model="tenantName" clearable placeholder="租户名" aria-label="租户名" @keyup.enter="load(true)" />
        <el-input v-if="!customer && !props.userId" v-model="userName" clearable placeholder="用户名 / 昵称" aria-label="用户名" @keyup.enter="load(true)" />
        <el-input v-model="group" clearable placeholder="分组名称 / ID" aria-label="分组" @keyup.enter="load(true)" />
        <el-input v-model="apiKeyName" clearable placeholder="API Key 名称" aria-label="API Key 名称" @keyup.enter="load(true)" />
        <el-select v-model="source" clearable placeholder="全部来源"><el-option label="API" value="api_key" /><el-option label="网页对话" value="web_chat" /><el-option label="网页生图" value="web_image" /><el-option label="网页视频" value="web_video" /></el-select>
        <el-select v-model="range" aria-label="时间范围"><el-option v-for="option in WORKBENCH_RANGE_OPTIONS" :key="option.id" :value="option.id" :label="option.label" /></el-select>
        <el-date-picker v-if="range === 'custom'" v-model="customRange" type="datetimerange" start-placeholder="开始时间" end-placeholder="结束时间" range-separator="至" />
        <template #actions><el-button @click="resetFilters">重置</el-button><el-button type="primary" :loading="busy" @click="load(true)">查询</el-button></template>
      </DsFilterBar>
      <p v-if="tab === 'errors'" class="records-note">这里只包含明确报错的请求，费用与计费额度均免收。错误诊断保留 30 天。</p>
      <DsTable :columns="columns" :rows="rows" row-key="request_id" :loading="busy" :frame="false" empty-title="当前范围没有记录">
        <template #cell-time="{ row }">{{ date(row.created_at) }}</template>
        <template #cell-model="{ row }"><strong>{{ row.model || '未命中模型' }}</strong><small>{{ sourceLabel(row.source) }} · {{ row.request_id }}</small></template>
        <template #cell-subject="{ row }"><span>{{ row.tenant_id }}</span><small>{{ row.user_id || '租户调用' }}</small></template>
        <template #cell-tokens="{ row }"><span v-if="row.media_units != null">{{ row.media_units }} {{ row.media_unit_type }}</span><span v-else>输入 {{ tokenCount(row.tokens.input) }}</span><small>输出 {{ tokenCount(row.tokens.output) }} · 缓存读 {{ tokenCount(row.tokens.cache_read) }}</small></template>
        <template #cell-charge="{ row }"><strong>{{ microUSD(customer ? row.charge.user_charged_micro : row.charge.tenant_charged_micro) }}</strong><small>{{ chargeLabel(row) }}</small></template>
        <template #cell-error="{ row }"><span>{{ row.error?.message || '错误诊断已过保留期' }}</span><small>{{ row.error?.code }}</small></template>
        <template #cell-timing="{ row }">{{ row.total_ms == null ? '—' : `${row.total_ms} ms` }}</template>
        <template #cell-detail="{ row }"><el-button link type="primary" @click="show(row.request_id)">查看</el-button></template>
      </DsTable>
      <div class="records-pager"><span>第 {{ history.length + 1 }} 页</span><el-button :disabled="busy || !history.length" @click="previous">上一页</el-button><el-button :disabled="busy || !nextCursor" @click="next">下一页</el-button></div>
    </div>
    <DsDrawer :open="open" title="请求与费用详情" width="680px" @update:open="open = $event">
      <p v-if="detailBusy">正在读取详情…</p>
      <div v-else-if="detail" class="record-detail">
        <h3>{{ detail.model }}</h3><small>{{ detail.request_id }}</small>
        <DsTag :tone="detail.is_error ? 'warning' : 'neutral'">{{ chargeLabel(detail) }}</DsTag>
        <p>{{ chargeReason(detail.charge.reason) }}</p>
        <p v-if="!detail.execution_available">执行详情已过保留期，计费依据仍可查询。</p>
        <dl><dt>输入 / 输出</dt><dd>{{ tokenCount(detail.tokens.input) }} / {{ tokenCount(detail.tokens.output) }}</dd>
          <dt>缓存读 / 缓存写</dt><dd>{{ tokenCount(detail.tokens.cache_read) }} / {{ tokenCount(detail.tokens.cache_write) }}</dd>
          <dt v-if="!customer">租户实际扣款</dt><dd v-if="!customer">{{ microUSD(detail.charge.tenant_charged_micro) }}</dd>
          <dt>用户实际扣款</dt><dd>{{ microUSD(detail.charge.user_charged_micro) }}</dd>
          <dt>API Key 额度消耗</dt><dd>{{ microUSD(detail.charge.api_key_used_micro) }}</dd>
          <dt>订阅额度消耗</dt><dd>{{ microUSD(detail.charge.subscription_used_micro) }}</dd>
          <dt v-if="admin">参考成本</dt><dd v-if="admin">{{ microUSD(detail.charge.reference_cost_micro) }}</dd>
          <dt v-if="detail.charge.refunded_at">用户已退回</dt><dd v-if="detail.charge.refunded_at">{{ microUSD(detail.charge.user_charged_micro) }}</dd>
          <dt v-if="detail.charge.refunded_at && !customer">租户已退回</dt><dd v-if="detail.charge.refunded_at && !customer">{{ microUSD(detail.charge.tenant_charged_micro) }}</dd>
          <dt v-if="detail.charge.refunded_at">退款时间</dt><dd v-if="detail.charge.refunded_at">{{ date(detail.charge.refunded_at) }}</dd>
          <dt>响应交付</dt><dd>{{ detail.delivery === 'disconnected' ? '客户端连接中断' : detail.delivery === 'complete' ? '响应已交付' : '未确认完整交付' }}</dd>
        </dl>
        <section v-if="detail.error"><h4>错误说明</h4><p>{{ detail.error.message }}</p><small>{{ detail.error.stage }} · {{ detail.error.code }}</small></section>
        <details v-if="admin && detail.charge.processing_detail"><summary>结算诊断</summary><pre>{{ detail.charge.processing_detail }}</pre></details>
        <details v-if="admin && detail.internal_detail"><summary>内部诊断</summary><pre>{{ detail.internal_detail }}</pre></details>
        <details v-if="admin && detail.attempts?.length"><summary>上游尝试</summary><pre>{{ JSON.stringify(detail.attempts, null, 2) }}</pre></details>
        <section v-if="receiptRows.length"><h4>计费依据</h4><dl><template v-for="row in receiptRows" :key="row.label"><dt>{{ row.label }}</dt><dd>{{ row.value }}</dd></template></dl></section>
        <details v-if="admin && detail.evidence"><summary>完整计费快照</summary><pre>{{ JSON.stringify({ evidence: detail.evidence, pricing: detail.pricing }, null, 2) }}</pre></details>
        <section v-if="admin"><el-button :loading="debugBusy" @click="readDebug">查看限时调试内容</el-button><pre v-if="debugContent">{{ debugContent }}</pre></section>
        <section v-if="admin && detail.charge.state === 'posted'"><h4>全额退款</h4><el-input v-model="refundReason" placeholder="填写退款原因" maxlength="500" /><el-button type="primary" :disabled="!refundReason.trim()" :loading="refundBusy" @click="refund">退回费用与计费额度</el-button></section>
      </div>
    </DsDrawer>
  </PortalPagePanel>
</template>

<style scoped>
.records-workspace, .record-detail { display: grid; gap: 20px; padding: 20px; min-width: 0; }
.records-workspace :deep(.ds-filter-bar > .el-input), .records-workspace :deep(.ds-filter-bar > .el-select) { flex: 1 1 180px; width: auto; min-width: 140px; max-width: 260px; }
.records-workspace :deep(.el-date-editor--datetimerange) { flex: 1 1 360px; max-width: 100%; min-width: 0; }
.record-detail { padding: 0; }
.record-detail > .ds-tag { justify-self: start; }
.records-workspace small, .record-detail small { display: block; color: var(--ds-muted); font-size: 12px; overflow-wrap: anywhere; }
.records-note { color: var(--ds-muted); font-size: 13px; margin: 0; }
.records-pager { display: flex; justify-content: flex-end; align-items: center; gap: 12px; }
.record-detail dl { display: grid; grid-template-columns: 150px 1fr; gap: 12px; }
.record-detail dt { color: var(--ds-muted); }
.record-detail dd { margin: 0; }
.record-detail pre { white-space: pre-wrap; overflow-wrap: anywhere; max-height: 420px; overflow: auto; font-size: 12px; }
.record-detail section { display: grid; gap: 12px; border-top: 1px solid var(--ds-border); padding-top: 16px; }
</style>
