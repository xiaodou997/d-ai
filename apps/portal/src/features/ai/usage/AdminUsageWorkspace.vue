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

import UsageExplorerWorkspace from "./components/UsageExplorerWorkspace.vue";
import UsageRangeSelector from "./components/UsageRangeSelector.vue";
import { adminUsageApi } from "./api";
import { useAdminUsageExplorer } from "./composables/useAdminUsageExplorer";
import type { AdminUsageRow, UsageFilters } from "./model";
import { restoreUsageRecordRouteQuery } from "./usageNavigation";
import { useAuthStore } from "@/stores/auth";

const route = useRoute();
const router = useRouter();

const {
  WORKBENCH_RANGE_OPTIONS,
  applyFilters,
  changePage,
  changePageSize,
  changeRange,
  customRange,
  explorerHighlights,
  explorerMetrics,
  filterChips,
  filters,
  isPlatformAdmin,
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
          v-model:custom-range="customRange"
          @update:custom-range="() => { if (customRange) void changeRange('custom') }"
          @update:model-value="changeRange"
        />
      </template>

      <div class="usage-body">
        <UsageExplorerWorkspace
          v-model:filters="filtersModel"
          :filter-chips="filterChips"
          :highlights="explorerHighlights"
          :is-platform-admin="isPlatformAdmin"
          :loading="logsLoading || summaryLoading"
          :logs="logs"
          :metrics="explorerMetrics"
          :pagination="pagination"
          :show-overview="false"
          :show-metrics="true"
          :summary-note="summaryNote"
          @page-change="changePage"
          @page-size-change="changePageSize"
          @refresh="refresh"
          @reset-filters="resetFilters"
          @search="applyFilters"
          @select-record="openDetail"
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
  min-width: 0;
  min-height: 0;
  gap: 20px;
  padding: 24px;
}
</style>
