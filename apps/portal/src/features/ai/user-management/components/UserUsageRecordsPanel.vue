<script setup lang="ts">
import { computed, onUnmounted, shallowRef, watch } from "vue";
import { ElMessage } from "element-plus";
import { formatCredits } from "@/platform/ai/usage";

import { listTenantUsageRecords } from "../../usage/api";
import TenantUsageDetailDrawer from "../../usage/components/TenantUsageDetailDrawer.vue";
import TenantUsageTable from "../../usage/components/TenantUsageTable.vue";
import { EMPTY_TENANT_USAGE_STATS, type TenantUsageRow, type TenantUsageStats } from "../../usage/model";
import type { TenantEndUserItem } from "@/api/types/tenant";
import type { UserUsageFilters } from "../model";

const props = defineProps<{
  user: TenantEndUserItem | null;
  filters: UserUsageFilters;
  reloadKey: number;
}>();

const page = shallowRef(1);
const pageSize = shallowRef(20);
const total = shallowRef(0);
const loading = shallowRef(false);
const rows = shallowRef<TenantUsageRow[]>([]);
const stats = shallowRef<TenantUsageStats>({ ...EMPTY_TENANT_USAGE_STATS });
const selectedRecord = shallowRef<TenantUsageRow | null>(null);
const detailOpen = shallowRef(false);

let generation = 0;
let controller: AbortController | undefined;

const successRate = computed(() => stats.value.total_requests
  ? `${((stats.value.success_count / stats.value.total_requests) * 100).toFixed(1)}%`
  : "-");

function query() {
  return {
    limit: pageSize.value,
    offset: (page.value - 1) * pageSize.value,
    user_id: props.user?.userId,
    model_code: props.filters.modelCode || undefined,
    request_status: props.filters.requestStatus || undefined,
    request_source: props.filters.requestSource || undefined,
    date_from: props.filters.dateRange?.[0] ? new Date(props.filters.dateRange[0]).toISOString() : undefined,
    date_to: props.filters.dateRange?.[1] ? new Date(props.filters.dateRange[1]).toISOString() : undefined
  };
}

function clear() {
  rows.value = [];
  stats.value = { ...EMPTY_TENANT_USAGE_STATS };
  total.value = 0;
  selectedRecord.value = null;
  detailOpen.value = false;
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
    const response = await listTenantUsageRecords(query(), nextController.signal);
    if (nextController.signal.aborted || requestGeneration !== generation) return;
    rows.value = (response.records ?? []).map((record) => ({
      ...record,
      userLabel: props.user?.username || props.user?.email || record.user_id || record.external_user_id || "-"
    }));
    stats.value = response.stats ?? { ...EMPTY_TENANT_USAGE_STATS };
    total.value = response.total ?? 0;
  } catch (error) {
    if (!isAbortError(error) && requestGeneration === generation) {
      ElMessage.error(error instanceof Error ? error.message : "加载用户使用记录失败");
    }
  } finally {
    if (!nextController.signal.aborted && requestGeneration === generation) loading.value = false;
  }
}

async function changePage(value: number) {
  page.value = value;
  await load();
}

async function changePageSize(value: number) {
  pageSize.value = value;
  page.value = 1;
  await load();
}

function openDetail(row: TenantUsageRow) {
  selectedRecord.value = row;
  detailOpen.value = true;
}

watch(
  [() => props.user?.userId, () => props.reloadKey],
  () => {
    page.value = 1;
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
  <section class="user-usage-records">
    <div class="stats-grid">
      <div class="stat-card">
        <span>请求数</span><strong>{{ stats.total_requests.toLocaleString() }}</strong>
        <small>{{ stats.success_count.toLocaleString() }} 成功 / {{ stats.failed_count.toLocaleString() }} 失败</small>
      </div>
      <div class="stat-card">
        <span>成功率</span><strong>{{ successRate }}</strong><small>当前过滤范围</small>
      </div>
      <div class="stat-card">
        <span>总 Token</span><strong>{{ formatCredits(stats.total_tokens) }}</strong><small>当前过滤范围</small>
      </div>
      <div class="stat-card">
        <span>消费积分</span><strong class="accent">{{ formatCredits(stats.total_user_charged_credits) }}</strong><small>均延 {{ Math.round(stats.avg_latency_ms) }} ms</small>
      </div>
    </div>

    <TenantUsageTable
      :loading="loading"
      :page="page"
      :page-size="pageSize"
      :rows="rows"
      :show-user="false"
      :total="total"
      @page-change="changePage"
      @page-size-change="changePageSize"
      @select="openDetail"
    />

    <TenantUsageDetailDrawer :open="detailOpen" :row="selectedRecord" @close="detailOpen = false" />
  </section>
</template>

<style scoped>
.user-usage-records { display: flex; flex-direction: column; gap: 16px; padding-top: 4px; }
.stats-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
.stat-card { display: flex; min-width: 0; flex-direction: column; gap: 5px; padding: 2px 14px; border-left: 1px solid var(--ds-line); }
.stat-card:first-child { padding-left: 0; border-left: 0; }
.stat-card span, .stat-card small { color: var(--ds-muted); font-size: 12px; }
.stat-card strong { color: var(--ds-ink); font-size: 21px; font-weight: 700; font-variant-numeric: tabular-nums; }
.stat-card strong.accent { color: var(--ds-accent-hover); }
@media (max-width: 1100px) { .stats-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 640px) { .stats-grid { grid-template-columns: 1fr; } }
</style>
