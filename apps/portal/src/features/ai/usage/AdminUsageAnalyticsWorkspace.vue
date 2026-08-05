<!--
  管理端 AI 网关用量分析工作台。
  本页聚合趋势、成本与资源结构；请求级筛选和详情排障留在 AdminUsageWorkspace。
-->
<script setup lang="ts">
import { shallowRef } from "vue";
import { useRouter } from "vue-router";
import { ElMessage } from "element-plus";
import { BarChart3 } from "lucide-vue-next";

import { PortalPagePanel } from "@dai/app-core";
import { DsTabs } from "@dai/ui";

import UsageAnalyticsPanel from "./components/UsageAnalyticsPanel.vue";
import UsageRangeSelector from "./components/UsageRangeSelector.vue";
import UsageUpstreamPanel from "./components/UsageUpstreamPanel.vue";
import UsageUserRankingPanel from "./components/UsageUserRankingPanel.vue";
import { adminUsageApi } from "./api";
import { useAdminUsageExplorer } from "./composables/useAdminUsageExplorer";
import type { AdminUsageRow } from "./model";
import { buildUsageRecordsRouteQuery } from "./usageNavigation";
import { useAuthStore } from "../../../stores/auth";

type AnalyticsTab = "overview" | "ranking" | "upstream";

const router = useRouter();
const activeTab = shallowRef<AnalyticsTab>("overview");

const {
  WORKBENCH_RANGE_OPTIONS,
  analyticsMetrics,
  changeRange,
  failedLogs,
  isPlatformAdmin,
  loadRanking,
  loadUpstream,
  modelDistribution,
  periodLabel,
  rankingLimit,
  rankingLoading,
  rankingRows,
  rankingTotal,
  requestTrendSeries,
  selectedRangeId,
  slowLogs,
  summaryLoading,
  tokenTrendSeries,
  trendLoading,
  trendRows,
  unitDistribution,
  upstreamLoading,
  upstreamRows,
  changeRankingLimit
} = useAdminUsageExplorer({
  api: adminUsageApi,
  auth: useAuthStore(),
  scope: "analytics",
  onError: (message) => ElMessage.error(message)
});

const tabOptions = [
  { key: "overview", label: "用量概览" },
  { key: "ranking", label: "用户排行" },
  { key: "upstream", label: "上游资源" }
];

function openDetail(row: AdminUsageRow) {
  void router.push({ name: "ai-usage-detail", params: { requestId: row.request_id } });
}

async function handleTabChange(key: string) {
  const tab = key as AnalyticsTab;
  activeTab.value = tab;
  if (tab === "ranking") await loadRanking();
  if (tab === "upstream") await loadUpstream();
}

async function handleRangeChange(range: typeof selectedRangeId.value) {
  await changeRange(range);
  if (activeTab.value === "ranking") await loadRanking();
  if (activeTab.value === "upstream") await loadUpstream();
}

function openRecords(filters: Record<string, string> = {}) {
  void router.push({
    name: "ai-usage",
    query: buildUsageRecordsRouteQuery(selectedRangeId.value, filters)
  });
}

function handleSelectUser(userId: string) {
  openRecords({ user_id: userId });
}
</script>

<template>
  <div class="page-container usage-view">
    <PortalPagePanel
      fill
      :icon="BarChart3"
      :breadcrumbs="[
        { label: '智能服务' },
        { label: '日志审计' },
        { label: '用量分析' }
      ]"
      :description="`${periodLabel}内查看请求趋势、计费结构与上游资源表现。`"
    >
      <template #actions>
        <UsageRangeSelector
          :model-value="selectedRangeId"
          :options="WORKBENCH_RANGE_OPTIONS"
          @update:model-value="handleRangeChange"
        />
      </template>

      <div class="usage-body">
        <DsTabs
          :tabs="tabOptions"
          :model-value="activeTab"
          @update:model-value="handleTabChange"
        />

        <UsageAnalyticsPanel
          v-if="activeTab === 'overview'"
          :failed-logs="failedLogs"
          :loading="summaryLoading || trendLoading"
          :metrics="analyticsMetrics"
          :model-distribution="modelDistribution"
          :request-trend-series="requestTrendSeries"
          :rows="trendRows"
          :slow-logs="slowLogs"
          :token-trend-series="tokenTrendSeries"
          :unit-distribution="unitDistribution"
          @select-record="openDetail"
        />

        <UsageUserRankingPanel
          v-else-if="activeTab === 'ranking'"
          :filter-chips="[]"
          :is-platform-admin="isPlatformAdmin"
          :limit="rankingLimit"
          :loading="rankingLoading"
          :rows="rankingRows"
          :total="rankingTotal"
          @refresh="loadRanking"
          @select-user="handleSelectUser"
          @switch-to-records="openRecords"
          @update-limit="changeRankingLimit"
        />

        <UsageUpstreamPanel
          v-else
          :filter-chips="[]"
          :loading="upstreamLoading"
          :rows="upstreamRows"
          @refresh="loadUpstream"
        />
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.usage-view {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
}

.usage-body {
  display: grid;
  flex: 1;
  min-height: 0;
  gap: 20px;
  padding: 24px;
}
</style>
