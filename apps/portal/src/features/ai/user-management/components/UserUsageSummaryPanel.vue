<!--
  使用统计面板 — 按模型聚合并展示单个终端用户的请求数、Token 与消费金额。
  重构:el-table → DsTable(:frame="false",mono/右对齐列),统计卡与请求逻辑保持不变。
-->
<script setup lang="ts">
import { computed, onUnmounted, shallowRef, watch } from "vue";
import { ElMessage } from "element-plus";
import { formatTokenCount, formatUSD } from "@/platform/ai/usage";
import { DsTable, type DsTableColumn } from "@/shared/ui";

import { listTenantUsageSummary } from "../../usage/api";
import type { TenantUsageSummaryRow } from "../../usage/model";
import type { TenantEndUserItem } from "@/api/types/tenant";
import type { UserUsageFilters } from "../model";

// DsTable 列:model_code 为标识符用 mono;请求/Token/金额列右对齐走 #cell-* 格式化
const columns: DsTableColumn[] = [
  { key: "model_code", title: "模型", mono: true },
  { key: "request_count", title: "请求数", width: 120, align: "right" },
  { key: "prompt_tokens", title: "输入 Token", width: 140, align: "right" },
  { key: "completion_tokens", title: "输出 Token", width: 140, align: "right" },
  { key: "total_tokens", title: "总 Token", width: 140, align: "right" },
  { key: "amount_usd", title: "用户消费（USD）", width: 150, align: "right" }
];

const props = defineProps<{
  user: TenantEndUserItem | null;
  filters: UserUsageFilters;
  reloadKey: number;
}>();

const loading = shallowRef(false);
const rows = shallowRef<TenantUsageSummaryRow[]>([]);

let generation = 0;
let controller: AbortController | undefined;

const totals = computed(() => rows.value.reduce(
  (acc, row) => ({
    requests: acc.requests + Number(row.request_count || 0),
    tokens: acc.tokens + Number(row.total_tokens || 0),
    amountUSD: acc.amountUSD + Number(row.total_user_charged_usd || 0)
  }),
  { requests: 0, tokens: 0, amountUSD: 0 }
));

function clear() {
  rows.value = [];
}

async function load() {
  if (!props.user) {
    generation += 1;
    controller?.abort();
    clear();
    return;
  }

  controller?.abort();
  const nextController = new AbortController();
  controller = nextController;
  const requestGeneration = ++generation;
  loading.value = true;
  try {
    const response = await listTenantUsageSummary({
      user_id: props.user.userId,
      model_code: props.filters.modelCode || undefined,
      request_status: props.filters.requestStatus || undefined,
      request_source: props.filters.requestSource || undefined,
      date_from: props.filters.dateRange?.[0] ? new Date(props.filters.dateRange[0]).toISOString() : undefined,
      date_to: props.filters.dateRange?.[1] ? new Date(props.filters.dateRange[1]).toISOString() : undefined
    }, nextController.signal);
    if (nextController.signal.aborted || requestGeneration !== generation) return;
    rows.value = response.items ?? [];
  } catch (error) {
    if (!isAbortError(error) && requestGeneration === generation) {
      ElMessage.error(error instanceof Error ? error.message : "加载用户使用统计失败");
    }
  } finally {
    if (!nextController.signal.aborted && requestGeneration === generation) loading.value = false;
  }
}

watch(
  [() => props.user?.userId, () => props.reloadKey],
  () => {
    void load();
  },
  { immediate: true }
);

onUnmounted(() => {
  generation += 1;
  controller?.abort();
});

function isAbortError(error: unknown) {
  return error instanceof DOMException
    ? error.name === "AbortError"
    : Boolean(error && typeof error === "object" && "name" in error && error.name === "AbortError");
}
</script>

<template>
  <section class="user-usage-summary">
    <div class="stats-grid">
      <div class="stat-card"><span>模型数</span><strong>{{ rows.length }}</strong><small>当前过滤范围</small></div>
      <div class="stat-card"><span>请求数</span><strong>{{ totals.requests.toLocaleString() }}</strong><small>当前过滤范围</small></div>
      <div class="stat-card"><span>总 Token</span><strong>{{ formatTokenCount(totals.tokens) }}</strong><small>当前过滤范围</small></div>
      <div class="stat-card"><span>消费金额</span><strong class="accent">{{ formatUSD(totals.amountUSD) }}</strong><small>按模型聚合</small></div>
    </div>

    <DsTable
      :frame="false"
      :columns="columns"
      :rows="rows"
      row-key="model_code"
      :loading="loading"
      class="summary-table"
      empty-title="暂无使用统计"
      empty-description="当前过滤范围内没有模型调用数据"
    >
      <template #cell-request_count="{ row }">{{ Number(row.request_count || 0).toLocaleString() }}</template>
      <template #cell-prompt_tokens="{ row }">{{ formatTokenCount(row.total_prompt_tokens) }}</template>
      <template #cell-completion_tokens="{ row }">{{ formatTokenCount(row.total_completion_tokens) }}</template>
      <template #cell-total_tokens="{ row }">{{ formatTokenCount(row.total_tokens) }}</template>
      <template #cell-amount_usd="{ row }"><strong class="accent">{{ formatUSD(row.total_user_charged_usd) }}</strong></template>
    </DsTable>
  </section>
</template>

<style scoped>
.user-usage-summary { display: flex; flex-direction: column; gap: 16px; padding-top: 4px; }
.stats-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
.stat-card { display: flex; min-width: 0; flex-direction: column; gap: 5px; padding: 2px 14px; border-left: 1px solid var(--ds-line); }
.stat-card:first-child { padding-left: 0; border-left: 0; }
.stat-card span, .stat-card small { color: var(--ds-muted); font-size: 12px; }
.stat-card strong { color: var(--ds-ink); font-size: 21px; font-weight: 700; font-variant-numeric: tabular-nums; }
.accent { color: var(--ds-accent-hover) !important; }
.summary-table { width: 100%; }
@media (max-width: 1100px) { .stats-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 640px) { .stats-grid { grid-template-columns: 1fr; } }
</style>
