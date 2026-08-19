<script setup lang="ts">
import { computed, onMounted } from "vue";
import { Activity, AlertTriangle, LayoutDashboard } from "lucide-vue-next";

import { PortalContentCard, PortalMetricGrid, PortalPagePanel } from "@/platform";
import { DsEmpty, DsMetricCard, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";
import OverviewRangeControls from "@/features/admin/overview/OverviewRangeControls.vue";
import OverviewTrendChart from "@/features/admin/overview/OverviewTrendChart.vue";
import { useAdminOverviewData } from "@/features/admin/overview/useAdminOverviewData";
import { formatMs, formatNumber, statusLabel, statusTone, successRate, systemStatusLabel, systemStatusTone, trendLabels } from "@/features/admin/overview/overviewUtils";

const data = useAdminOverviewData(["summary", "models", "errors", "system", "modules", "proxy"], "24h");
const { failedSections, selectedRangeId, loading, lastUpdatedAt, selectedRange, summary, models, errors, trend, system, modules, proxyNodes, refresh, changeRange } = data;

const activeModuleCount = computed(() => modules.value.filter((item) => item.active).length);
const activeProxyCount = computed(() => proxyNodes.value.filter((item) => item.status === "active" && item.healthStatus !== "unhealthy").length);
const statusItems = computed(() => [
  { label: "PostgreSQL", value: system.value?.db.status, detail: "数据库连接" },
  { label: "Redis", value: system.value?.redis.status, detail: "缓存与运行态" },
  { label: "路由健康", value: system.value?.health.open_count ? "open" : system.value?.health.half_open_count ? "half_open" : "closed", detail: `${system.value?.health.total_tracked ?? 0} 个目标` },
  { label: "系统模块", value: activeModuleCount.value === modules.value.length && modules.value.length > 0 ? "active" : "warning", detail: `${activeModuleCount.value}/${modules.value.length} 个启用` },
  { label: "代理出口", value: activeProxyCount.value > 0 ? "active" : "disabled", detail: `${activeProxyCount.value}/${proxyNodes.value.length} 个可用` }
]);

const modelColumns: DsTableColumn[] = [
  { key: "model_code", title: "模型" },
  { key: "request_count", title: "请求数", width: 110, align: "right" },
  { key: "total_tokens", title: "Token", width: 120, align: "right" }
];
const errorColumns: DsTableColumn[] = [
  { key: "created_at", title: "时间", width: 165 },
  { key: "requested_model", title: "模型" },
  { key: "request_status", title: "状态", width: 100 },
  { key: "request_id", title: "请求 ID", width: 150 }
];

function formatTimestamp(value?: number) {
  return value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "—";
}

onMounted(() => { void refresh(); });
</script>

<template>
  <div class="overview-page">
    <PortalPagePanel :icon="LayoutDashboard" :breadcrumbs="[{ label: '概览' }, { label: '仪表盘' }]" description="用一页快速判断平台运行状态、调用情况和待处理异常。">
      <template #actions>
        <OverviewRangeControls :model-value="selectedRangeId" :loading="loading" :updated-at="lastUpdatedAt" @update:model-value="changeRange" @refresh="refresh" />
      </template>

      <div class="overview-body">
        <div v-if="failedSections.length" class="overview-warning"><AlertTriangle :size="15" />部分数据暂时无法加载，已展示可用结果。</div>

        <PortalMetricGrid>
          <DsMetricCard label="请求量" :value="formatNumber(summary.total_requests)" :hint="`${selectedRange.label}总调用`" />
          <DsMetricCard label="成功率" :value="successRate(summary)" :hint="`${formatNumber(summary.successful_requests)} 次成功`" />
          <DsMetricCard label="Token 使用量" :value="formatNumber(summary.total_tokens)" hint="输入与输出 Token 合计" />
          <DsMetricCard label="平均响应" :value="formatMs(summary.avg_request_total_ms)" :hint="`首响 ${formatMs(summary.avg_first_response_byte_ms)}`" />
        </PortalMetricGrid>

        <div class="overview-grid overview-grid--main">
          <PortalContentCard title="系统状态" description="只展示需要快速判断的核心状态，详细排查进入健康监控。">
            <template #actions><DsTag :tone="systemStatusTone(system)">{{ systemStatusLabel(system) }}</DsTag></template>
            <div class="status-list">
              <div v-for="item in statusItems" :key="item.label" class="status-row">
                <div class="status-row__main"><Activity :size="15" /><div><strong>{{ item.label }}</strong><small>{{ item.detail }}</small></div></div>
                <DsTag :tone="statusTone(item.value)">{{ statusLabel(item.value) }}</DsTag>
              </div>
            </div>
          </PortalContentCard>

          <PortalContentCard title="请求趋势" description="按当前时间范围观察调用量与失败波动。">
            <OverviewTrendChart
              :labels="trendLabels(trend)"
              :series="[
                { label: '请求', values: trend.map((item) => item.request_count), color: 'var(--ds-info)' },
                { label: '成功', values: trend.map((item) => item.success_count), color: 'var(--ds-positive)' },
                { label: '失败', values: trend.map((item) => item.failed_count), color: 'var(--ds-danger)' }
              ]"
              :value-formatter="formatNumber"
            />
          </PortalContentCard>
        </div>

        <div class="overview-grid overview-grid--bottom">
          <PortalContentCard title="最近异常" description="优先展示最近需要排查的失败请求。">
            <DsTable v-if="errors.length" :columns="errorColumns" :rows="errors" row-key="request_id" :frame="false">
              <template #cell-created_at="{ row }">{{ formatTimestamp(row.created_at) }}</template>
              <template #cell-request_status="{ row }"><DsTag :tone="row.request_status === 'success' ? 'positive' : 'danger'">{{ row.request_status }}</DsTag></template>
              <template #cell-requested_model="{ row }">{{ row.requested_model || row.model_code || "—" }}</template>
            </DsTable>
            <DsEmpty v-else title="暂无异常" description="当前时间范围内没有可展示的失败请求。" />
          </PortalContentCard>

          <PortalContentCard title="热门模型" description="按请求量查看当前平台主要使用模型。">
            <DsTable v-if="models.length" :columns="modelColumns" :rows="models" row-key="model_code" :frame="false">
              <template #cell-request_count="{ row }">{{ formatNumber(row.request_count) }}</template>
              <template #cell-total_tokens="{ row }">{{ formatNumber(row.total_tokens) }}</template>
            </DsTable>
            <DsEmpty v-else title="暂无模型调用" description="当前时间范围内还没有使用数据。" />
          </PortalContentCard>
        </div>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.overview-page { min-height: 100%; }
.overview-body { display: flex; flex-direction: column; gap: 20px; padding: 24px; }
.overview-warning { display: flex; align-items: center; gap: 8px; padding: 10px 12px; border: 1px solid color-mix(in srgb, var(--ds-warning) 30%, var(--ds-line)); border-radius: var(--ds-radius-control); background: var(--ds-warning-soft); color: var(--ds-warning); font-size: 12px; }
.overview-grid { display: grid; gap: 20px; }
.overview-grid--main { grid-template-columns: minmax(0, .9fr) minmax(0, 1.1fr); }
.overview-grid--bottom { grid-template-columns: minmax(0, 1.2fr) minmax(0, .8fr); }
.status-list { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
.status-row { display: flex; align-items: center; justify-content: space-between; gap: 10px; min-width: 0; padding: 11px 12px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-control); background: var(--ds-panel-muted); }
.status-row__main { display: flex; align-items: center; gap: 8px; min-width: 0; color: var(--ds-ink-soft); }
.status-row__main > svg { flex: 0 0 auto; color: var(--ds-accent); }
.status-row__main div { display: flex; flex-direction: column; min-width: 0; }
.status-row strong { overflow: hidden; color: var(--ds-ink-soft); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.status-row small { margin-top: 3px; color: var(--ds-muted); font-size: 10px; }
@media (max-width: 1100px) { .overview-grid--main, .overview-grid--bottom { grid-template-columns: 1fr; } }
@media (max-width: 620px) { .status-list { grid-template-columns: 1fr; } }
</style>
