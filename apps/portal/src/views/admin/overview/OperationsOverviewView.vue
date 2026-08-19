<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { Gauge } from "lucide-vue-next";

import { PortalContentCard, PortalMetricGrid, PortalPagePanel } from "@/platform";
import { DsEmpty, DsMetricCard, DsTable, DsTabs, DsTag, type DsTableColumn } from "@/shared/ui";
import OverviewDataWarning from "@/features/admin/overview/OverviewDataWarning.vue";
import OperationsHealthPanel from "@/features/admin/overview/OperationsHealthPanel.vue";
import OverviewRangeControls from "@/features/admin/overview/OverviewRangeControls.vue";
import OverviewTrendChart from "@/features/admin/overview/OverviewTrendChart.vue";
import { useAdminOverviewData } from "@/features/admin/overview/useAdminOverviewData";
import { formatMs, formatNumber, statusLabel, statusTone, trendLabels } from "@/features/admin/overview/overviewUtils";

type OperationsTab = "overview" | "health";

const route = useRoute();
const router = useRouter();
const tabs = [
  { key: "overview", label: "运行概览" },
  { key: "health", label: "健康详情" }
];
const activeTab = computed<OperationsTab>(() => route.query.tab === "health" ? "health" : "overview");

const overviewData = useAdminOverviewData(["summary", "trend", "system", "errors", "proxy"], "24h");
const { failedSections, selectedRangeId, loading, lastUpdatedAt, selectedRange, summary, trend, system, errors, proxyNodes, refresh, changeRange } = overviewData;

const healthData = useAdminOverviewData(["system", "modules", "proxy"], "24h");
const {
  failedSections: healthFailedSections,
  loading: healthLoading,
  lastUpdatedAt: healthUpdatedAt,
  system: healthSystem,
  modules: healthModules,
  proxyNodes: healthProxyNodes,
  refresh: refreshHealth
} = healthData;
const overviewLoaded = ref(false);
const healthLoaded = ref(false);

const activeLoading = computed(() => activeTab.value === "health" ? healthLoading.value : loading.value);
const activeUpdatedAt = computed(() => activeTab.value === "health" ? healthUpdatedAt.value : lastUpdatedAt.value);

async function loadOverview(force = false) {
  if ((overviewLoaded.value || loading.value) && !force) return;
  await refresh();
  overviewLoaded.value = true;
}

async function loadHealth(force = false) {
  if ((healthLoaded.value || healthLoading.value) && !force) return;
  await refreshHealth();
  healthLoaded.value = true;
}

function selectTab(key: string) {
  const query = { ...route.query };
  if (key === "health") query.tab = "health";
  else delete query.tab;
  void router.replace({ query });
}

function refreshActive() {
  if (activeTab.value === "health") void loadHealth(true);
  else void loadOverview(true);
}

watch(activeTab, (tab) => {
  if (tab === "health") void loadHealth();
  else void loadOverview();
}, { immediate: true });

const requestsPerHour = computed(() => {
  const hours = Math.max(selectedRange.value.hours, 1);
  return Math.round((Number(summary.value.total_requests) || 0) / hours);
});
const errorRate = computed(() => {
  const total = Number(summary.value.total_requests) || 0;
  return total ? `${((Number(summary.value.failed_requests) * 100) / total).toFixed(1)}%` : "0%";
});
const healthyProxyCount = computed(() => proxyNodes.value.filter((node) => node.status === "active" && node.healthStatus !== "unhealthy").length);
const routeState = computed(() => {
  if (!system.value) return "unknown";
  if (system.value.health.open_count > 0) return "open";
  if (system.value.health.half_open_count > 0) return "half_open";
  return "closed";
});

const errorColumns: DsTableColumn[] = [
  { key: "created_at", title: "时间", width: 150 },
  { key: "model", title: "模型" },
  { key: "request_status", title: "请求状态", width: 110 },
  { key: "request_id", title: "请求 ID", width: 150 }
];
const proxyColumns: DsTableColumn[] = [
  { key: "name", title: "代理节点" },
  { key: "proxyType", title: "类型", width: 90 },
  { key: "endpoint", title: "出口地址" },
  { key: "healthStatus", title: "健康", width: 100 }
];

function formatErrorTime(value?: number) {
  return value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "—";
}

function errorModel(row: { model_code: string; requested_model?: string }) {
  return row.requested_model || row.model_code || "—";
}

function proxyTone(status?: string) {
  return statusTone(status === "healthy" ? "healthy" : status === "unhealthy" ? "unhealthy" : status);
}
</script>

<template>
  <div class="overview-page">
    <PortalPagePanel :icon="Gauge" :breadcrumbs="[{ label: '概览' }, { label: '运维监控' }]" description="集中观察运行趋势、异常请求和基础设施健康状态。">
      <template #actions><OverviewRangeControls :model-value="selectedRangeId" :show-range="activeTab === 'overview'" :loading="activeLoading" :updated-at="activeUpdatedAt" @update:model-value="changeRange" @refresh="refreshActive" /></template>
      <div class="overview-body">
        <div class="operations-tabs"><DsTabs :tabs="tabs" :model-value="activeTab" @update:model-value="selectTab" /></div>

        <template v-if="activeTab === 'overview'">
          <OverviewDataWarning :sections="failedSections" />
          <PortalMetricGrid>
            <DsMetricCard label="平均吞吐" :value="`${formatNumber(requestsPerHour)} /小时`" hint="按当前时间范围折算" />
            <DsMetricCard label="错误率" :value="errorRate" :hint="`${formatNumber(summary.failed_requests)} 次失败请求`" />
            <DsMetricCard label="平均总耗时" :value="formatMs(summary.avg_request_total_ms)" hint="从请求开始到完成" />
            <DsMetricCard label="平均首响" :value="formatMs(summary.avg_first_response_byte_ms)" hint="首字节响应时间" />
          </PortalMetricGrid>

          <PortalContentCard title="请求运行趋势" description="从吞吐、成功和失败三个维度观察运行态变化。">
            <OverviewTrendChart
              :labels="trendLabels(trend)"
              :series="[
                { label: '请求', values: trend.map((item) => item.request_count), color: 'var(--ds-accent)' },
                { label: '成功', values: trend.map((item) => item.success_count), color: 'var(--ds-positive)' },
                { label: '失败', values: trend.map((item) => item.failed_count), color: 'var(--ds-danger)' }
              ]"
              :value-formatter="formatNumber"
            />
          </PortalContentCard>

          <div class="overview-grid">
            <PortalContentCard title="运行时压力" description="只保留影响服务可用性的关键状态。">
              <div class="runtime-status-list">
                <div class="runtime-status-item">
                  <span><b>路由健康</b><small>{{ system?.health.total_tracked ?? 0 }} 个目标正在跟踪</small></span>
                  <DsTag :tone="statusTone(routeState)">{{ statusLabel(routeState) }}</DsTag>
                </div>
                <div class="runtime-status-item">
                  <span><b>PostgreSQL</b><small>请求与配置数据存储</small></span>
                  <DsTag :tone="statusTone(system?.db.status)">{{ statusLabel(system?.db.status) }}</DsTag>
                </div>
                <div class="runtime-status-item">
                  <span><b>Redis</b><small>缓存与路由健康状态</small></span>
                  <DsTag :tone="statusTone(system?.redis.status)">{{ statusLabel(system?.redis.status) }}</DsTag>
                </div>
                <div class="runtime-status-item">
                  <span><b>代理出口</b><small>{{ healthyProxyCount }}/{{ proxyNodes.length }} 个节点可用</small></span>
                  <DsTag :tone="healthyProxyCount > 0 || !proxyNodes.length ? 'positive' : 'danger'">{{ healthyProxyCount > 0 || !proxyNodes.length ? '正常' : '异常' }}</DsTag>
                </div>
              </div>
            </PortalContentCard>

            <PortalContentCard title="最近错误" description="优先展示最新的失败请求，详细排查进入使用记录。">
              <DsTable v-if="errors.length" :columns="errorColumns" :rows="errors.slice(0, 6)" row-key="request_id" :frame="false">
                <template #cell-created_at="{ row }">{{ formatErrorTime(row.created_at) }}</template>
                <template #cell-model="{ row }">{{ errorModel(row) }}</template>
                <template #cell-request_status="{ row }"><DsTag tone="danger">{{ row.request_status || "失败" }}</DsTag></template>
              </DsTable>
              <DsEmpty v-else title="暂无错误记录" description="当前时间范围内没有需要关注的失败请求。" />
            </PortalContentCard>
          </div>

          <PortalContentCard title="代理出口节点" description="运行概览只展示节点可用性，详细健康状态可切换到健康详情。">
            <DsTable v-if="proxyNodes.length" :columns="proxyColumns" :rows="proxyNodes" row-key="id" :frame="false">
              <template #cell-proxyType="{ row }">{{ row.proxyType.toUpperCase() }}</template>
              <template #cell-healthStatus="{ row }"><DsTag :tone="proxyTone(row.healthStatus)">{{ statusLabel(row.healthStatus) }}</DsTag></template>
            </DsTable>
            <DsEmpty v-else title="暂无代理出口节点" description="当前未配置代理出口节点，系统会直接访问上游。" />
          </PortalContentCard>
        </template>

        <template v-else>
          <OverviewDataWarning :sections="healthFailedSections" />
          <OperationsHealthPanel :system="healthSystem" :modules="healthModules" :proxy-nodes="healthProxyNodes" />
        </template>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.overview-page { min-height: 100%; }
.overview-body { display: flex; flex-direction: column; gap: 20px; padding: 24px; }
.operations-tabs { padding-bottom: 4px; border-bottom: 1px solid var(--ds-line); }
.overview-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px; }
.runtime-status-list { display: flex; flex-direction: column; gap: 2px; }
.runtime-status-item { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 13px 0; border-bottom: 1px solid var(--ds-line); }
.runtime-status-item:first-child { padding-top: 0; }
.runtime-status-item:last-child { padding-bottom: 0; border-bottom: 0; }
.runtime-status-item span { display: flex; min-width: 0; flex-direction: column; gap: 4px; }
.runtime-status-item b { color: var(--ds-ink); font-size: 13px; }
.runtime-status-item small { color: var(--ds-muted); font-size: 11px; }
@media (max-width: 980px) { .overview-grid { grid-template-columns: 1fr; } }
</style>
