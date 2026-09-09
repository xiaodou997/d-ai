<script setup lang="ts">
import { computed, onMounted } from "vue";
import { BarChart3 } from "lucide-vue-next";
import { PortalContentCard, PortalMetricGrid, PortalPagePanel } from "@/platform";
import { DsEmpty, DsMetricCard, DsTable, type DsTableColumn } from "@/shared/ui";
import OverviewAccountTable from "./OverviewAccountTable.vue";
import OverviewDataWarning from "./OverviewDataWarning.vue";
import OverviewQualityStrip from "./OverviewQualityStrip.vue";
import OverviewRangeControls from "./OverviewRangeControls.vue";
import OverviewTrendChart from "./OverviewTrendChart.vue";
import { useAdminOverviewSnapshot } from "./useAdminOverviewSnapshot";
import { formatNumber, formatUSDStat, trendLabels } from "./overviewUtils";

const data = useAdminOverviewSnapshot("business", "30d");
const { selectedRangeId, selectedRange, snapshot, loading, lastUpdatedAt, error, refresh, changeRange } = data;
const summary = computed(() => snapshot.value.summary);
const failedSections = computed(() => error.value ? ["summary" as const] : []);
const tenantColumns: DsTableColumn[] = [
  { key: "tenant_id", title: "租户" }, { key: "request_count", title: "请求", width: 110, align: "right" },
  { key: "total_tokens", title: "Token", width: 110, align: "right" }, { key: "total_tenant_payable_usd", title: "应收", width: 110, align: "right" }
];
const modelColumns: DsTableColumn[] = [
  { key: "model_code", title: "模型" }, { key: "request_count", title: "请求", width: 110, align: "right" },
  { key: "total_tokens", title: "Token", width: 110, align: "right" }, { key: "total_tenant_payable_usd", title: "应收", width: 110, align: "right" }
];
const tenantLabels = computed(() => snapshot.value.included?.tenants ?? {});
function tenantLabel(id: string) { return tenantLabels.value[id]?.tenant_name || id; }
onMounted(() => { void refresh(); });
</script>

<template>
  <div class="overview-page"><PortalPagePanel :icon="BarChart3" :breadcrumbs="[{ label: '概览' }, { label: '业务概览' }]" description="观察平台业务规模、客户结构和主要需求来源。">
    <template #actions><OverviewRangeControls :model-value="selectedRangeId" :loading="loading" :updated-at="lastUpdatedAt" @update:model-value="changeRange" @refresh="refresh" /></template>
    <div class="overview-body">
      <OverviewDataWarning :sections="failedSections" />
      <PortalMetricGrid>
        <DsMetricCard label="活跃租户" :value="formatNumber(summary.active_tenants)" :hint="selectedRange.label" />
        <DsMetricCard label="活跃用户" :value="formatNumber(summary.active_users)" :hint="selectedRange.label" />
        <DsMetricCard label="新增租户" :value="formatNumber(summary.new_tenants)" :hint="`${selectedRange.label}内创建`" />
        <DsMetricCard label="新增用户" :value="formatNumber(summary.new_users)" :hint="`${selectedRange.label}内创建`" />
        <DsMetricCard label="平台请求量" :value="formatNumber(summary.total_requests)" hint="当前业务规模" />
        <DsMetricCard label="Token 使用量" :value="formatNumber(summary.total_tokens)" hint="输入、输出与缓存合计" />
        <DsMetricCard label="租户应收" :value="formatUSDStat(summary.total_tenant_payable_usd)" hint="平台结算收入" />
      </PortalMetricGrid>
      <OverviewQualityStrip :summary="summary" />
      <PortalContentCard title="业务增长趋势" description="请求量、成功调用和租户应收按日变化。">
        <OverviewTrendChart :labels="trendLabels(snapshot.trends)" :series="[
          { label: '请求', values: snapshot.trends.map((item) => item.request_count), color: 'var(--ds-accent)' },
          { label: '成功', values: snapshot.trends.map((item) => item.success_count), color: 'var(--ds-positive)' },
          { label: 'Token', values: snapshot.trends.map((item) => item.total_tokens), color: 'var(--ds-info)' }
        ]" :value-formatter="formatNumber" />
        <div class="trend-divider">应收趋势</div>
        <OverviewTrendChart :labels="trendLabels(snapshot.trends)" :series="[{ label: '租户应收', values: snapshot.trends.map((item) => item.tenant_payable_usd), color: 'var(--ds-info)' }]" :value-formatter="formatUSDStat" />
      </PortalContentCard>
      <OverviewAccountTable :accounts="snapshot.accounts" title="账号质量概览" description="业务规模增长必须与账号成功率、缓存率一起观察。" />
      <div class="overview-grid">
        <PortalContentCard title="租户使用排行" description="按请求量和应收查看业务贡献。">
          <DsTable v-if="snapshot.tenants.length" :columns="tenantColumns" :rows="snapshot.tenants" row-key="tenant_id" :frame="false">
            <template #cell-tenant_id="{ row }">{{ tenantLabel(row.tenant_id) }}</template>
            <template #cell-request_count="{ row }">{{ formatNumber(row.request_count) }}</template>
            <template #cell-total_tokens="{ row }">{{ formatNumber(row.total_tokens) }}</template>
            <template #cell-total_tenant_payable_usd="{ row }">{{ formatUSDStat(row.total_tenant_payable_usd) }}</template>
          </DsTable><DsEmpty v-else title="暂无租户数据" description="当前时间范围内没有可分析的租户调用记录。" />
        </PortalContentCard>
        <PortalContentCard title="模型需求排行" description="识别平台当前最主要的模型需求。">
          <DsTable v-if="snapshot.models.length" :columns="modelColumns" :rows="snapshot.models" row-key="model_code" :frame="false">
            <template #cell-request_count="{ row }">{{ formatNumber(row.request_count) }}</template>
            <template #cell-total_tokens="{ row }">{{ formatNumber(row.total_tokens) }}</template>
            <template #cell-total_tenant_payable_usd="{ row }">{{ formatUSDStat(row.total_tenant_payable_usd) }}</template>
          </DsTable><DsEmpty v-else title="暂无模型数据" description="当前时间范围内没有可分析的模型调用记录。" />
        </PortalContentCard>
      </div>
    </div>
  </PortalPagePanel></div>
</template>

<style scoped>
.overview-page { min-height: 100%; }
.overview-body { display: flex; flex-direction: column; gap: 20px; padding: 24px; }
.overview-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px; }
.trend-divider { margin: 18px 0 8px; color: var(--ds-muted); font-size: 11px; font-weight: 700; }
@media (max-width: 980px) { .overview-grid { grid-template-columns: 1fr; } }
</style>
