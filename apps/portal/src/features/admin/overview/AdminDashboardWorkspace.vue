<script setup lang="ts">
import { computed, onMounted } from "vue";
import { LayoutDashboard, TrendingUp } from "lucide-vue-next";

import { PortalContentCard, PortalMetricGrid, PortalPagePanel } from "@/platform";
import { DsEmpty, DsMetricCard, DsTable, type DsTableColumn } from "@/shared/ui";
import OverviewAccountTable from "./OverviewAccountTable.vue";
import OverviewDataWarning from "./OverviewDataWarning.vue";
import OverviewQualityStrip from "./OverviewQualityStrip.vue";
import OverviewRangeControls from "./OverviewRangeControls.vue";
import OverviewTrendChart from "./OverviewTrendChart.vue";
import { useAdminOverviewSnapshot } from "./useAdminOverviewSnapshot";
import { formatNumber, formatUSDStat, rateText, trendLabels } from "./overviewUtils";

const data = useAdminOverviewSnapshot("dashboard", "24h");
const { selectedRangeId, selectedRange, snapshot, loading, lastUpdatedAt, error, refresh, changeRange } = data;
const summary = computed(() => snapshot.value.summary);
const accounts = computed(() => snapshot.value.accounts);
const models = computed(() => snapshot.value.models);
const tenants = computed(() => snapshot.value.tenants);
const failedSections = computed(() => error.value ? ["summary" as const] : []);

const modelColumns: DsTableColumn[] = [
  { key: "model_code", title: "模型" }, { key: "request_count", title: "请求", width: 100, align: "right" },
  { key: "total_tokens", title: "Token", width: 100, align: "right" }, { key: "total_tenant_payable_usd", title: "应收", width: 100, align: "right" }
];
const tenantColumns: DsTableColumn[] = [
  { key: "tenant_id", title: "租户" }, { key: "request_count", title: "请求", width: 100, align: "right" },
  { key: "total_tokens", title: "Token", width: 100, align: "right" }, { key: "total_tenant_payable_usd", title: "应收", width: 100, align: "right" }
];

const tenantLabels = computed(() => snapshot.value.included?.tenants ?? {});
function tenantLabel(id: string) { return tenantLabels.value[id]?.tenant_name || id; }

onMounted(() => { void refresh(); });
</script>

<template>
  <div class="overview-page">
    <PortalPagePanel :icon="LayoutDashboard" :breadcrumbs="[{ label: '概览' }, { label: '仪表盘' }]" description="从经营规模、收入质量和账号产出判断平台当前状态。">
      <template #actions><OverviewRangeControls :model-value="selectedRangeId" :loading="loading" :updated-at="lastUpdatedAt" @update:model-value="changeRange" @refresh="refresh" /></template>
      <div class="overview-body">
        <OverviewDataWarning :sections="failedSections" />
        <PortalMetricGrid>
          <DsMetricCard label="活跃租户" :value="formatNumber(summary.active_tenants)" :hint="`${selectedRange.label}内有请求`" />
          <DsMetricCard label="活跃用户" :value="formatNumber(summary.active_users)" :hint="`${selectedRange.label}内有请求`" />
          <DsMetricCard label="请求总量" :value="formatNumber(summary.total_requests)" :hint="`成功 ${formatNumber(summary.successful_requests)}`" />
          <DsMetricCard label="Token 使用量" :value="formatNumber(summary.total_tokens)" hint="输入、输出与缓存合计" />
          <DsMetricCard label="租户应收" :value="formatUSDStat(summary.total_tenant_payable_usd)" hint="平台结算收入" />
          <DsMetricCard label="毛利" :value="formatUSDStat(summary.gross_margin_usd)" :hint="`毛利率 ${rateText(summary.gross_margin_usd, summary.total_tenant_payable_usd)}`" />
        </PortalMetricGrid>

        <OverviewQualityStrip :summary="summary" />

        <div class="overview-grid overview-grid--wide">
          <PortalContentCard title="经营趋势" description="请求量、成功调用和平台应收的变化。">
            <OverviewTrendChart :labels="trendLabels(snapshot.trends)" :series="[
              { label: '请求', values: snapshot.trends.map((item) => item.request_count), color: 'var(--ds-accent)' },
              { label: '成功', values: snapshot.trends.map((item) => item.success_count), color: 'var(--ds-positive)' },
              { label: 'Token', values: snapshot.trends.map((item) => item.total_tokens), color: 'var(--ds-info)' }
            ]" :value-formatter="formatNumber" />
            <div class="trend-divider">应收趋势</div>
            <OverviewTrendChart :labels="trendLabels(snapshot.trends)" :series="[{ label: '租户应收', values: snapshot.trends.map((item) => item.tenant_payable_usd), color: 'var(--ds-info)' }]" :value-formatter="formatUSDStat" />
          </PortalContentCard>
          <PortalContentCard title="运营信号" description="优先展示值得运营跟进的规模和质量变化。">
            <div class="signal-list">
              <div><TrendingUp :size="16" /><span>主力模型</span><strong>{{ models[0]?.model_code || "—" }}</strong></div>
              <div><TrendingUp :size="16" /><span>最大租户</span><strong>{{ tenants[0] ? tenantLabel(tenants[0].tenant_id) : "—" }}</strong></div>
              <div><TrendingUp :size="16" /><span>最高请求账号</span><strong>{{ accounts[0]?.target_name || accounts[0]?.provider_code || "—" }}</strong></div>
            </div>
          </PortalContentCard>
        </div>

        <OverviewAccountTable :accounts="accounts" />

        <div class="overview-grid">
          <PortalContentCard title="热门模型" description="按请求量查看当前平台需求中心。">
            <DsTable v-if="models.length" :columns="modelColumns" :rows="models.slice(0, 8)" row-key="model_code" :frame="false">
              <template #cell-request_count="{ row }">{{ formatNumber(row.request_count) }}</template>
              <template #cell-total_tokens="{ row }">{{ formatNumber(row.total_tokens) }}</template>
              <template #cell-total_tenant_payable_usd="{ row }">{{ formatUSDStat(row.total_tenant_payable_usd) }}</template>
            </DsTable>
            <DsEmpty v-else title="暂无模型数据" description="当前时间范围内没有可分析的调用记录。" />
          </PortalContentCard>
          <PortalContentCard title="租户排行" description="按请求量查看平台当前业务贡献。">
            <DsTable v-if="tenants.length" :columns="tenantColumns" :rows="tenants.slice(0, 8)" row-key="tenant_id" :frame="false">
              <template #cell-tenant_id="{ row }">{{ tenantLabel(row.tenant_id) }}</template>
              <template #cell-request_count="{ row }">{{ formatNumber(row.request_count) }}</template>
              <template #cell-total_tokens="{ row }">{{ formatNumber(row.total_tokens) }}</template>
              <template #cell-total_tenant_payable_usd="{ row }">{{ formatUSDStat(row.total_tenant_payable_usd) }}</template>
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
.overview-grid--wide { grid-template-columns: minmax(0, 1.35fr) minmax(280px, .65fr); }
.trend-divider { margin: 18px 0 8px; color: var(--ds-muted); font-size: 11px; font-weight: 700; }
.signal-list { display: grid; gap: 4px; }
.signal-list > div { display: grid; grid-template-columns: 20px 1fr auto; align-items: center; gap: 8px; padding: 12px 0; border-bottom: 1px solid var(--ds-line); }
.signal-list > div:last-child { border-bottom: 0; }
.signal-list svg { color: var(--ds-accent); }
.signal-list span { color: var(--ds-muted); font-size: 12px; }
.signal-list strong { overflow: hidden; color: var(--ds-ink); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
@media (max-width: 980px) { .overview-grid, .overview-grid--wide { grid-template-columns: 1fr; } }
</style>
