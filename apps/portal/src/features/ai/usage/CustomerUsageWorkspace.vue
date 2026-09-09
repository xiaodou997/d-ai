<!--
  用户端 AI 使用记录工作台:查看当前账号最近的智能服务请求、消耗、状态和错误信息。
  重构:迁移至新设计系统一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行,
       指标带置于 body 顶部 24px 容器,筛选/表格/分页同卡,表格统一 DsTable);
       业务逻辑、请求参数与详情抽屉不变(用户端无按 requestId 的详情接口,保留抽屉)。
-->
<script setup lang="ts">
import { computed } from "vue";
import { Refresh } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import { ScrollText } from "lucide-vue-next";

import { PortalMetricGrid, PortalPagePanel } from "@/platform";
import { formatCompactToken, formatMs, formatUSD } from "@/platform/ai/usage";

import { customerUsageApi } from "./api";
import CustomerUsageDetailDrawer from "./components/CustomerUsageDetailDrawer.vue";
import CustomerUsageFilters from "./components/CustomerUsageFilters.vue";
import CustomerUsageTable from "./components/CustomerUsageTable.vue";
import { useCustomerUsage } from "./composables/useCustomerUsage";

const {
  changeServerFilter,
  detailOpen,
  filteredRecords,
  filters,
  loadRecords,
  loading,
  openDetail,
  page,
  pageSize,
  pagedRecords,
  reset,
  search,
  selectedRecord,
  stats,
  successRate
} = useCustomerUsage({ api: customerUsageApi, onError: (message) => ElMessage.error(message) });

const metrics = computed(() => [
  {
    label: "请求数",
    value: stats.value.totalRequests.toLocaleString("zh-CN"),
    hint: `${stats.value.successRequests.toLocaleString("zh-CN")} 成功 / ${stats.value.failedRequests.toLocaleString("zh-CN")} 异常`
  },
  { label: "成功率", value: successRate.value, hint: "基于当前筛选结果" },
  { label: "Token", value: formatCompactToken(stats.value.totalTokens), hint: "输入与输出合计" },
  {
    label: "消费金额",
    value: formatUSD(stats.value.totalAmountUSD),
    hint: `平均请求耗时 ${formatMs(Math.round(stats.value.avgLatency))}`
  }
]);
</script>

<template>
  <div class="page-container customer-usage-page">
    <PortalPagePanel
      fill
      :icon="ScrollText"
      :breadcrumbs="[
        { label: '智能服务' },
        { label: '我的服务' },
        { label: '使用记录' }
      ]"
      description="查看当前账号最近的智能服务请求、消耗、状态和错误信息。"
    >
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="loadRecords">刷新</el-button>
      </template>

      <div class="usage-body">
        <PortalMetricGrid :metrics="metrics" min-col-width="180px" />

        <CustomerUsageFilters
          v-model="filters"
          :loading="loading"
          @reset="reset"
          @search="search"
          @server-change="changeServerFilter"
        />

        <CustomerUsageTable
          :loading="loading"
          :page="page"
          :page-size="pageSize"
          :rows="pagedRecords"
          :total="filteredRecords.length"
          @page-change="page = $event"
          @page-size-change="pageSize = $event; page = 1"
          @select="openDetail"
        />
      </div>
    </PortalPagePanel>

    <CustomerUsageDetailDrawer :open="detailOpen" :row="selectedRecord" @close="detailOpen = false" />
  </div>
</template>

<style scoped>
/* fill 链:页面根 flex 撑满,面板 fill 随之伸展 */
.customer-usage-page { display: flex; min-height: 0; flex: 1; flex-direction: column; gap: 20px; }
.usage-body { display: flex; min-width: 0; min-height: 0; flex: 1; flex-direction: column; gap: 20px; padding: 24px; }
</style>
