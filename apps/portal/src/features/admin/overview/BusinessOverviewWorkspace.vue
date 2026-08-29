<script setup lang="ts">
import { computed, onMounted } from "vue";
import { BarChart3 } from "lucide-vue-next";

import { PortalContentCard, PortalMetricGrid, PortalPagePanel } from "@/platform";
import { DsEmpty, DsMetricCard, DsTable, type DsTableColumn } from "@/shared/ui";
import OverviewDataWarning from "@/features/admin/overview/OverviewDataWarning.vue";
import OverviewRangeControls from "@/features/admin/overview/OverviewRangeControls.vue";
import OverviewTrendChart from "@/features/admin/overview/OverviewTrendChart.vue";
import { useAdminOverviewData } from "@/features/admin/overview/useAdminOverviewData";
import { formatNumber, formatUSD, successRate, trendLabels } from "@/features/admin/overview/overviewUtils";
import { PortalIdentityCell, resolveIdentityTenantLabel, resolveIdentityTenantMeta } from "@/platform/ai/identity";

const data = useAdminOverviewData(["summary", "models", "tenants", "trend", "global"], "30d");
const { failedSections, selectedRangeId, selectedRange, loading, lastUpdatedAt, summary, models, tenants, tenantIncluded, trend, global, refresh, changeRange } = data;

const businessHint = computed(() => global.value ? `${selectedRange.value.label}平台业务数据` : "等待业务数据");
const modelColumns: DsTableColumn[] = [
  { key: "model_code", title: "模型" },
  { key: "request_count", title: "请求数", width: 110, align: "right" },
  { key: "total_tokens", title: "Token", width: 120, align: "right" },
  { key: "total_tenant_payable_usd", title: "租户应收", width: 120, align: "right" }
];
const tenantColumns: DsTableColumn[] = [
  { key: "tenant", title: "租户" },
  { key: "request_count", title: "请求数", width: 110, align: "right" },
  { key: "total_tokens", title: "Token", width: 120, align: "right" },
  { key: "total_tenant_payable_usd", title: "应收", width: 110, align: "right" }
];

function tenantLabel(id: string) { return resolveIdentityTenantLabel(id, tenantIncluded.value); }
function tenantMeta(id: string) { return resolveIdentityTenantMeta(id, tenantIncluded.value); }

onMounted(() => { void refresh(); });
</script>

<template>
  <div class="overview-page">
    <PortalPagePanel :icon="BarChart3" :breadcrumbs="[{ label: '概览' }, { label: '业务概览' }]" description="观察平台整体使用结构、业务增长和主要模型/租户。">
      <template #actions><OverviewRangeControls :model-value="selectedRangeId" :loading="loading" :updated-at="lastUpdatedAt" @update:model-value="changeRange" @refresh="refresh" /></template>
      <div class="overview-body">
        <OverviewDataWarning :sections="failedSections" />
        <PortalMetricGrid>
          <DsMetricCard label="活跃租户" :value="`${formatNumber(global?.activeTenants)} 个`" :hint="businessHint" />
          <DsMetricCard label="新增终端用户" :value="`${formatNumber(global?.newUsers)} 名`" :hint="`${selectedRange.label}注册`" />
          <DsMetricCard label="平台请求量" :value="formatNumber(summary.total_requests)" :hint="`${successRate(summary)} 成功率`" />
          <DsMetricCard label="Token 使用量" :value="formatNumber(summary.total_tokens)" hint="输入与输出合计" />
        </PortalMetricGrid>

        <PortalContentCard title="业务调用趋势" :description="`${selectedRange.label}请求与成功调用变化`">
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
          <PortalContentCard title="热门模型" description="按请求量和 Token 查看平台主要使用模型。">
            <DsTable v-if="models.length" :columns="modelColumns" :rows="models" row-key="model_code" :frame="false">
              <template #cell-request_count="{ row }">{{ formatNumber(row.request_count) }}</template>
              <template #cell-total_tokens="{ row }">{{ formatNumber(row.total_tokens) }}</template>
              <template #cell-total_tenant_payable_usd="{ row }">{{ formatUSD(row.total_tenant_payable_usd) }}</template>
            </DsTable>
            <DsEmpty v-else title="暂无模型数据" description="当前时间范围内没有可分析的调用记录。" />
          </PortalContentCard>

          <PortalContentCard title="租户使用排行" description="按租户请求量和 Token 使用量排序。">
            <DsTable v-if="tenants.length" :columns="tenantColumns" :rows="tenants" row-key="tenant_id" :frame="false">
              <template #cell-tenant="{ row }"><PortalIdentityCell :label="tenantLabel(row.tenant_id)" :meta="tenantMeta(row.tenant_id)" /></template>
              <template #cell-request_count="{ row }">{{ formatNumber(row.request_count) }}</template>
              <template #cell-total_tokens="{ row }">{{ formatNumber(row.total_tokens) }}</template>
              <template #cell-total_tenant_payable_usd="{ row }">{{ formatUSD(row.total_tenant_payable_usd) }}</template>
            </DsTable>
            <DsEmpty v-else title="暂无租户数据" description="当前时间范围内没有可分析的租户调用记录。" />
          </PortalContentCard>
        </div>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.overview-page { min-height: 100%; }
.overview-body { display: flex; flex-direction: column; gap: 20px; padding: 24px; }
.overview-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px; }
@media (max-width: 980px) { .overview-grid { grid-template-columns: 1fr; } }
</style>
