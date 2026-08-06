<!--
  管理端 AI 网关请求记录工作台。
  页面只承担筛选、审计与单次请求排障；趋势、成本和资源结构由
  AdminUsageAnalyticsWorkspace 独立承接，避免记录表被统计内容挤出首屏。
-->
<script setup lang="ts">
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ElMessage } from "element-plus";
import { ScrollText } from "lucide-vue-next";

import { PortalPagePanel } from "@/platform";
import { DsTabs } from "@/shared/ui";

import UsageErrorsPanel from "./components/UsageErrorsPanel.vue";
import UsageExplorerWorkspace from "./components/UsageExplorerWorkspace.vue";
import UsageRangeSelector from "./components/UsageRangeSelector.vue";
import { adminUsageApi } from "./api";
import { useAdminUsageExplorer } from "./composables/useAdminUsageExplorer";
import type { AdminUsageRow, UsageFilters, UsageWorkbenchTab } from "./model";
import { restoreUsageRecordRouteQuery } from "./usageNavigation";
import { useAuthStore } from "@/stores/auth";

const route = useRoute();
const router = useRouter();

const {
  WORKBENCH_RANGE_OPTIONS,
  activeTab,
  applyFilters,
  changeErrorPage,
  changeErrorPageSize,
  changePage,
  changePageSize,
  changeRange,
  changeTab,
  errorPagination,
  errorRows,
  errorsLoading,
  explorerHighlights,
  explorerMetrics,
  filterChips,
  filters,
  isPlatformAdmin,
  loadErrors,
  logs,
  logsLoading,
  pagination,
  periodLabel,
  refresh,
  resetFilters,
  selectedRangeId,
  summaryLoading,
  summaryNote
} = useAdminUsageExplorer({
  api: adminUsageApi,
  auth: useAuthStore(),
  scope: "records",
  onError: (message) => ElMessage.error(message)
});

restoreUsageRecordRouteQuery(route.query, filters, (range) => {
  selectedRangeId.value = range;
});

function openDetail(row: AdminUsageRow) {
  void router.push({ name: "ai-usage-detail", params: { requestId: row.request_id } });
}

const filtersModel = computed<UsageFilters>({
  get: () => filters,
  set: (value) => Object.assign(filters, value)
});

const tabOptions = [
  { key: "records", label: "全部请求" },
  { key: "errors", label: "错误请求" }
];

function handleTabChange(key: string) {
  void changeTab(key as UsageWorkbenchTab);
}

function switchToRecords() {
  void changeTab("records");
}
</script>

<template>
  <div class="page-container usage-view">
    <PortalPagePanel
      fill
      :icon="ScrollText"
      :breadcrumbs="[
        { label: 'AI 网关' },
        { label: '使用记录' }
      ]"
      :description="`${periodLabel}内筛选、审计并排查单次 AI 请求。`"
    >
      <template #actions>
        <UsageRangeSelector
          :model-value="selectedRangeId"
          :options="WORKBENCH_RANGE_OPTIONS"
          @update:model-value="changeRange"
        />
      </template>

      <div class="usage-body">
        <DsTabs
          :tabs="tabOptions"
          :model-value="activeTab"
          @update:model-value="handleTabChange"
        />

        <UsageExplorerWorkspace
          v-if="activeTab === 'records'"
          v-model:filters="filtersModel"
          :filter-chips="filterChips"
          :highlights="explorerHighlights"
          :is-platform-admin="isPlatformAdmin"
          :loading="logsLoading || summaryLoading"
          :logs="logs"
          :metrics="explorerMetrics"
          :pagination="pagination"
          :show-overview="false"
          :summary-note="summaryNote"
          @page-change="changePage"
          @page-size-change="changePageSize"
          @refresh="refresh"
          @reset-filters="resetFilters"
          @search="applyFilters"
          @select-record="openDetail"
        />

        <UsageErrorsPanel
          v-else
          :filter-chips="filterChips"
          :is-platform-admin="isPlatformAdmin"
          :loading="errorsLoading"
          :pagination="errorPagination"
          :rows="errorRows"
          :total="errorPagination.total"
          @page-change="changeErrorPage"
          @page-size-change="changeErrorPageSize"
          @refresh="loadErrors"
          @select-record="openDetail"
          @switch-to-records="switchToRecords"
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
