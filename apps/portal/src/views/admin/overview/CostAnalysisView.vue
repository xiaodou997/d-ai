<script setup lang="ts">
import { computed, onMounted } from "vue";
import { Banknote } from "lucide-vue-next";

import { PortalContentCard, PortalMetricGrid, PortalPagePanel } from "@/platform";
import { DsEmpty, DsMetricCard, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";
import OverviewRangeControls from "@/features/admin/overview/OverviewRangeControls.vue";
import OverviewTrendChart from "@/features/admin/overview/OverviewTrendChart.vue";
import { useAdminOverviewData } from "@/features/admin/overview/useAdminOverviewData";
import { formatNumber, formatUSD, trendLabels } from "@/features/admin/overview/overviewUtils";

const data = useAdminOverviewData(["summary", "trend", "upstreams"], "30d");
const { selectedRangeId, loading, lastUpdatedAt, summary, trend, upstreams, refresh, changeRange } = data;

const referenceCostRate = computed(() => {
  const payable = Number(summary.value.total_tenant_payable_usd) || 0;
  const catalog = Number(summary.value.total_catalog_base_usd) || 0;
  return payable > 0 ? `${((catalog * 100) / payable).toFixed(1)}%` : "0%";
});

const upstreamColumns: DsTableColumn[] = [
  { key: "target_name", title: "上游资源" },
  { key: "provider_code", title: "供应商", width: 120 },
  { key: "target_kind", title: "类型", width: 100 },
  { key: "request_count", title: "请求数", width: 100, align: "right" },
  { key: "catalog_base_usd", title: "参考成本", width: 120, align: "right" },
  { key: "tenant_payable_usd", title: "租户应收", width: 120, align: "right" }
];

function targetKindLabel(value: string) { return value === "oauth_pool" ? "账号池" : "上游账号"; }
function marginValue() {
  return (Number(summary.value.total_tenant_payable_usd) || 0) - (Number(summary.value.total_catalog_base_usd) || 0);
}

onMounted(() => { void refresh(); });
</script>

<template>
  <div class="overview-page">
    <PortalPagePanel :icon="Banknote" :breadcrumbs="[{ label: '概览' }, { label: '成本分析' }]" description="分析 AI 服务的参考成本、结算金额和不同上游资源的成本结构。">
      <template #actions><OverviewRangeControls :model-value="selectedRangeId" :loading="loading" :updated-at="lastUpdatedAt" @update:model-value="changeRange" @refresh="refresh" /></template>
      <div class="overview-body">
        <div class="cost-note">成本口径：参考成本来自请求命中的上游价格快照；租户应收和用户实际扣款来自同一笔用量结算结果。</div>
        <PortalMetricGrid>
          <DsMetricCard label="上游参考成本" :value="formatUSD(summary.total_catalog_base_usd)" hint="价格表基准，不代表实际供应商账单" />
          <DsMetricCard label="租户应收" :value="formatUSD(summary.total_tenant_payable_usd)" hint="平台向租户结算口径" />
          <DsMetricCard label="用户实际扣款" :value="formatUSD(summary.total_user_charged_usd)" hint="终端用户实际扣款口径" />
          <DsMetricCard label="参考成本率" :value="referenceCostRate" :hint="`参考空间 ${formatUSD(marginValue())}`" />
        </PortalMetricGrid>

        <PortalContentCard title="成本与结算趋势" description="按当前时间范围比较上游参考成本、租户应收和用户实际扣款。">
          <OverviewTrendChart
            :labels="trendLabels(trend)"
            :series="[
              { label: '参考成本', values: trend.map((item) => item.catalog_base_usd), color: 'var(--ds-danger)' },
              { label: '租户应收', values: trend.map((item) => item.tenant_payable_usd), color: 'var(--ds-accent)' },
              { label: '用户扣款', values: trend.map((item) => item.user_charged_usd), color: 'var(--ds-positive)' }
            ]"
            :value-formatter="formatUSD"
          />
        </PortalContentCard>

        <PortalContentCard title="上游成本结构" description="按实际命中的上游账号或账号池汇总，帮助识别主要成本来源。">
          <DsTable v-if="upstreams.length" :columns="upstreamColumns" :rows="upstreams" row-key="target_id" :frame="false">
            <template #cell-target_name="{ row }"><span class="resource-name">{{ row.target_name || row.provider_code || row.target_id }}</span></template>
            <template #cell-target_kind="{ row }"><DsTag tone="info">{{ targetKindLabel(row.target_kind) }}</DsTag></template>
            <template #cell-request_count="{ row }">{{ formatNumber(row.request_count) }}</template>
            <template #cell-catalog_base_usd="{ row }">{{ formatUSD(row.catalog_base_usd) }}</template>
            <template #cell-tenant_payable_usd="{ row }">{{ formatUSD(row.tenant_payable_usd) }}</template>
          </DsTable>
          <DsEmpty v-else title="暂无成本数据" description="当前时间范围内没有可分析的上游使用记录。" />
        </PortalContentCard>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.overview-page { min-height: 100%; }
.overview-body { display: flex; flex-direction: column; gap: 20px; padding: 24px; }
.cost-note { padding: 10px 12px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-control); background: var(--ds-panel-muted); color: var(--ds-muted); font-size: 12px; line-height: 1.6; }
.resource-name { color: var(--ds-ink-soft); font-weight: 600; }
</style>
