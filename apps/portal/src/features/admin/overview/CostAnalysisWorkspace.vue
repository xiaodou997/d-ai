<script setup lang="ts">
import { computed, onMounted } from "vue";
import { Banknote, CircleAlert } from "lucide-vue-next";
import { PortalContentCard, PortalMetricGrid, PortalPagePanel } from "@/platform";
import { DsEmpty, DsMetricCard, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";
import OverviewDataWarning from "./OverviewDataWarning.vue";
import OverviewRangeControls from "./OverviewRangeControls.vue";
import OverviewTrendChart from "./OverviewTrendChart.vue";
import { useAdminOverviewData } from "./useAdminOverviewData";
import { formatNumber, formatUSD, trendLabels } from "./overviewUtils";

const data = useAdminOverviewData(["summary", "models", "tenants", "trend", "upstreams"], "30d");
const { failedSections, selectedRangeId, selectedRange, loading, lastUpdatedAt, summary, models, tenants, tenantIncluded, trend, upstreams, refresh, changeRange } = data;
const cost = computed(() => Number(summary.value.total_catalog_base_usd) || 0);
const revenue = computed(() => Number(summary.value.total_tenant_payable_usd) || 0);
const margin = computed(() => revenue.value - cost.value);
const marginRate = computed(() => revenue.value ? `${((margin.value / revenue.value) * 100).toFixed(1)}%` : "0%");
const unitCost = computed(() => Number(summary.value.total_requests) ? cost.value / Number(summary.value.total_requests) : 0);
const failedWaste = computed(() => { const failed = Number(summary.value.failed_requests) || 0; const total = Number(summary.value.total_requests) || 0; return total ? cost.value * failed / total : 0; });
const modelColumns: DsTableColumn[] = [
  { key: "model_code", title: "模型" }, { key: "request_count", title: "请求", width: 100, align: "right" },
  { key: "total_tokens", title: "Token", width: 120, align: "right" }, { key: "total_catalog_base_usd", title: "参考成本", width: 130, align: "right" }, { key: "total_tenant_payable_usd", title: "毛利", width: 120, align: "right" }
];
const tenantColumns: DsTableColumn[] = [
  { key: "tenant_id", title: "租户" }, { key: "request_count", title: "请求", width: 100, align: "right" }, { key: "total_catalog_base_usd", title: "成本", width: 120, align: "right" }, { key: "total_tenant_payable_usd", title: "应收", width: 120, align: "right" }
];
const upstreamColumns: DsTableColumn[] = [
  { key: "target_name", title: "上游资源" }, { key: "provider_code", title: "供应商", width: 120 }, { key: "request_count", title: "请求", width: 100, align: "right" }, { key: "catalog_base_usd", title: "成本", width: 120, align: "right" }, { key: "tenant_payable_usd", title: "应收", width: 120, align: "right" }
];
function tenantLabel(id: string) { return tenantIncluded.value.tenants?.[id]?.tenant_name || id; }
onMounted(() => { void refresh(); });
</script>
<template>
  <div class="overview-page"><PortalPagePanel :icon="Banknote" :breadcrumbs="[{ label: '概览' }, { label: '成本分析' }]" description="从平台利润、单位成本和成本归因观察全平台经营效率。">
    <template #actions><OverviewRangeControls :model-value="selectedRangeId" :loading="loading" :updated-at="lastUpdatedAt" @update:model-value="changeRange" @refresh="refresh" /></template>
    <div class="overview-body"><OverviewDataWarning :sections="failedSections" /><div class="cost-note">当前成本口径为命中上游价格快照的参考成本；毛利 = 租户应收 − 参考成本。数据范围：{{ selectedRange.label }}。</div>
      <PortalMetricGrid><DsMetricCard label="平台参考成本" :value="formatUSD(cost)" hint="上游价格快照" /><DsMetricCard label="租户应收" :value="formatUSD(revenue)" hint="平台结算收入" /><DsMetricCard label="毛利率" :value="marginRate" :hint="`毛利 ${formatUSD(margin)}`" /><DsMetricCard label="单请求成本" :value="formatUSD(unitCost)" hint="成本 / 总请求" /><DsMetricCard label="失败请求浪费" :value="formatUSD(failedWaste)" hint="估算可回收成本" /></PortalMetricGrid>
      <div class="signal"><CircleAlert :size="18" /><div><strong>平台经营信号</strong><span v-if="failedWaste > 0">失败请求约造成 {{ formatUSD(failedWaste) }} 参考成本，建议优先检查高失败模型和上游。</span><span v-else>当前时间范围未发现明显成本浪费信号。</span></div></div>
      <PortalContentCard title="成本、收入与毛利趋势" description="观察增长来自业务规模还是单位成本变化。"><OverviewTrendChart :labels="trendLabels(trend)" :series="[{ label: '参考成本', values: trend.map(i => i.catalog_base_usd), color: 'var(--ds-danger)' }, { label: '租户应收', values: trend.map(i => i.tenant_payable_usd), color: 'var(--ds-accent)' }, { label: '用户扣款', values: trend.map(i => i.user_charged_usd), color: 'var(--ds-positive)' }]" :value-formatter="formatUSD" /></PortalContentCard>
      <div class="overview-grid"><PortalContentCard title="模型成本排行" description="识别高成本模型和单位成本优化机会。"><DsTable v-if="models.length" :columns="modelColumns" :rows="models" row-key="model_code" :frame="false"><template #cell-request_count="{ row }">{{ formatNumber(row.request_count) }}</template><template #cell-total_tokens="{ row }">{{ formatNumber(row.total_tokens) }}</template><template #cell-total_catalog_base_usd="{ row }">{{ formatUSD(row.total_catalog_base_usd) }}</template><template #cell-total_tenant_payable_usd="{ row }">{{ formatUSD((Number(row.total_tenant_payable_usd)||0)-(Number(row.total_catalog_base_usd)||0)) }}</template></DsTable><DsEmpty v-else title="暂无模型数据" description="当前范围没有可分析记录。" /></PortalContentCard>
      <PortalContentCard title="租户成本与毛利" description="按租户定位成本集中和低毛利客户。"><DsTable v-if="tenants.length" :columns="tenantColumns" :rows="tenants" row-key="tenant_id" :frame="false"><template #cell-tenant_id="{ row }"><span class="resource-name">{{ tenantLabel(row.tenant_id) }}</span></template><template #cell-request_count="{ row }">{{ formatNumber(row.request_count) }}</template><template #cell-total_catalog_base_usd="{ row }">{{ formatUSD(row.total_catalog_base_usd) }}</template><template #cell-total_tenant_payable_usd="{ row }">{{ formatUSD(row.total_tenant_payable_usd) }}</template></DsTable><DsEmpty v-else title="暂无租户数据" description="当前范围没有可分析记录。" /></PortalContentCard></div>
      <PortalContentCard title="上游成本结构" description="按供应商和账号池识别主要成本来源。"><DsTable v-if="upstreams.length" :columns="upstreamColumns" :rows="upstreams" row-key="target_id" :frame="false"><template #cell-target_name="{ row }"><span class="resource-name">{{ row.target_name || row.provider_code || row.target_id }}</span></template><template #cell-request_count="{ row }">{{ formatNumber(row.request_count) }}</template><template #cell-catalog_base_usd="{ row }">{{ formatUSD(row.catalog_base_usd) }}</template><template #cell-tenant_payable_usd="{ row }">{{ formatUSD(row.tenant_payable_usd) }}</template></DsTable><DsEmpty v-else title="暂无上游数据" description="当前范围没有可分析记录。" /></PortalContentCard>
    </div></PortalPagePanel></div>
</template>
<style scoped>.overview-page{min-height:100%}.overview-body{display:flex;flex-direction:column;gap:20px;padding:24px}.cost-note{padding:10px 12px;border:1px solid var(--ds-line);border-radius:var(--ds-radius-control);background:var(--ds-panel-muted);color:var(--ds-muted);font-size:12px;line-height:1.6}.overview-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:20px}.signal{display:flex;gap:12px;align-items:flex-start;padding:14px 16px;border:1px solid color-mix(in srgb,var(--ds-warning) 30%,var(--ds-line));border-radius:var(--ds-radius-control);background:var(--ds-panel-muted);color:var(--ds-ink-soft)}.signal strong,.signal span{display:block}.signal span{margin-top:4px;color:var(--ds-muted);font-size:13px}.resource-name{font-weight:600;color:var(--ds-ink-soft)}@media(max-width:980px){.overview-grid{grid-template-columns:1fr}}</style>
