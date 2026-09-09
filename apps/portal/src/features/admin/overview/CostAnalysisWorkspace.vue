<script setup lang="ts">
import { computed, onMounted } from "vue";
import { Banknote, CircleAlert } from "lucide-vue-next";
import { PortalContentCard, PortalMetricGrid, PortalPagePanel } from "@/platform";
import { DsEmpty, DsMetricCard, DsTable, type DsTableColumn } from "@/shared/ui";
import OverviewAccountTable from "./OverviewAccountTable.vue";
import OverviewDataWarning from "./OverviewDataWarning.vue";
import OverviewQualityStrip from "./OverviewQualityStrip.vue";
import OverviewRangeControls from "./OverviewRangeControls.vue";
import OverviewTrendChart from "./OverviewTrendChart.vue";
import { useAdminOverviewSnapshot } from "./useAdminOverviewSnapshot";
import { formatNumber, formatUSDStat, rateText, trendLabels } from "./overviewUtils";

const data = useAdminOverviewSnapshot("cost", "30d");
const { selectedRangeId, selectedRange, snapshot, loading, lastUpdatedAt, error, refresh, changeRange } = data;
const summary = computed(() => snapshot.value.summary);
const failedSections = computed(() => error.value ? ["summary" as const] : []);
const marginRate = computed(() => rateText(summary.value.gross_margin_usd, summary.value.total_tenant_payable_usd));
const failedWaste = computed(() => {
  const total = Number(summary.value.total_requests) || 0;
  return total ? Number(summary.value.total_catalog_base_usd) * (Number(summary.value.failed_requests) / total) : 0;
});
const modelColumns: DsTableColumn[] = [
  { key: "model_code", title: "模型" }, { key: "request_count", title: "请求", width: 100, align: "right" },
  { key: "total_tokens", title: "Token", width: 100, align: "right" }, { key: "total_catalog_base_usd", title: "成本", width: 100, align: "right" },
  { key: "total_tenant_payable_usd", title: "应收", width: 100, align: "right" }
];
const tenantLabels = computed(() => snapshot.value.included?.tenants ?? {});
function tenantLabel(id: string) { return tenantLabels.value[id]?.tenant_name || id; }
onMounted(() => { void refresh(); });
</script>

<template>
  <div class="overview-page"><PortalPagePanel :icon="Banknote" :breadcrumbs="[{ label: '概览' }, { label: '成本分析' }]" description="从成本、应收和毛利观察平台经营效率。">
    <template #actions><OverviewRangeControls :model-value="selectedRangeId" :loading="loading" :updated-at="lastUpdatedAt" @update:model-value="changeRange" @refresh="refresh" /></template>
    <div class="overview-body">
      <OverviewDataWarning :sections="failedSections" />
      <div class="cost-note">成本为命中上游价格快照的参考成本；毛利 = 租户应收 − 参考成本。数据范围：{{ selectedRange.label }}。</div>
      <PortalMetricGrid>
        <DsMetricCard label="平台参考成本" :value="formatUSDStat(summary.total_catalog_base_usd)" hint="上游价格快照" />
        <DsMetricCard label="租户应收" :value="formatUSDStat(summary.total_tenant_payable_usd)" hint="平台结算收入" />
        <DsMetricCard label="用户实际扣款" :value="formatUSDStat(summary.total_user_charged_usd)" hint="终端用户实际扣款" />
        <DsMetricCard label="毛利" :value="formatUSDStat(summary.gross_margin_usd)" :hint="`毛利率 ${marginRate}`" />
        <DsMetricCard label="失败请求浪费" :value="formatUSDStat(failedWaste)" hint="按失败比例估算参考成本" />
      </PortalMetricGrid>
      <OverviewQualityStrip :summary="summary" />
      <div class="signal"><CircleAlert :size="18" /><div><strong>成本经营信号</strong><span v-if="failedWaste > 0">失败请求约造成 {{ formatUSDStat(failedWaste) }} 参考成本，建议查看低成功率账号。</span><span v-else>当前时间范围未发现明显失败成本浪费。</span></div></div>
      <PortalContentCard title="成本、收入与毛利趋势" description="观察增长来自业务规模还是单位成本变化。">
        <OverviewTrendChart :labels="trendLabels(snapshot.trends)" :series="[
          { label: '参考成本', values: snapshot.trends.map((item) => item.catalog_base_usd), color: 'var(--ds-danger)' },
          { label: '租户应收', values: snapshot.trends.map((item) => item.tenant_payable_usd), color: 'var(--ds-accent)' },
          { label: '用户扣款', values: snapshot.trends.map((item) => item.user_charged_usd), color: 'var(--ds-positive)' }
        ]" :value-formatter="formatUSDStat" />
      </PortalContentCard>
      <OverviewAccountTable :accounts="snapshot.accounts" title="账号成本归因" description="把账号成功率、缓存率和成本放在同一张表里，识别低效资源。" />
      <PortalContentCard title="模型成本排行" description="识别主要成本来源。">
        <DsTable v-if="snapshot.models.length" :columns="modelColumns" :rows="snapshot.models" row-key="model_code" :frame="false">
          <template #cell-request_count="{ row }">{{ formatNumber(row.request_count) }}</template>
          <template #cell-total_tokens="{ row }">{{ formatNumber(row.total_tokens) }}</template>
          <template #cell-total_catalog_base_usd="{ row }">{{ formatUSDStat(row.total_catalog_base_usd) }}</template>
          <template #cell-total_tenant_payable_usd="{ row }">{{ formatUSDStat(row.total_tenant_payable_usd) }}</template>
        </DsTable><DsEmpty v-else title="暂无模型成本数据" description="当前范围没有可分析记录。" />
      </PortalContentCard>
      <PortalContentCard title="租户成本与应收" description="按租户定位成本集中和低毛利客户。">
        <DsTable v-if="snapshot.tenants.length" :columns="[
          { key: 'tenant_id', title: '租户' }, { key: 'request_count', title: '请求', width: 110, align: 'right' },
          { key: 'total_tokens', title: 'Token', width: 110, align: 'right' }, { key: 'total_tenant_payable_usd', title: '应收', width: 110, align: 'right' }
        ]" :rows="snapshot.tenants" row-key="tenant_id" :frame="false">
          <template #cell-tenant_id="{ row }">{{ tenantLabel(row.tenant_id) }}</template>
          <template #cell-request_count="{ row }">{{ formatNumber(row.request_count) }}</template>
          <template #cell-total_tokens="{ row }">{{ formatNumber(row.total_tokens) }}</template>
          <template #cell-total_tenant_payable_usd="{ row }">{{ formatUSDStat(row.total_tenant_payable_usd) }}</template>
        </DsTable><DsEmpty v-else title="暂无租户成本数据" description="当前范围没有可分析记录。" />
      </PortalContentCard>
    </div>
  </PortalPagePanel></div>
</template>

<style scoped>
.overview-page { min-height: 100%; }
.overview-body { display: flex; flex-direction: column; gap: 20px; padding: 24px; }
.cost-note { padding: 10px 12px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-control); background: var(--ds-panel-muted); color: var(--ds-muted); font-size: 12px; }
.signal { display: flex; align-items: flex-start; gap: 10px; padding: 12px 14px; border: 1px solid color-mix(in srgb, var(--ds-warning) 30%, var(--ds-line)); border-radius: var(--ds-radius-control); background: var(--ds-warning-soft); color: var(--ds-warning); }
.signal div { display: grid; gap: 4px; }
.signal strong { color: var(--ds-ink); font-size: 12px; }
.signal span { font-size: 12px; line-height: 1.5; }
</style>
