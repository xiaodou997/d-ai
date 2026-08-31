<!--
  租户端 AI 使用记录工作台:查看本租户每一次 AI API 调用的消耗明细。
  重构:迁移至新设计系统一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行,
       指标卡 PortalMetricGrid/DsMetricCard 与表格置于同卡 body 的 24px 容器内,
       筛选带置于面板 filters 区,表格统一 DsTable、分页统一 DsPagination 始终渲染);
       业务逻辑与请求参数不变。租户端没有按 requestId 的详情接口,
       单条调用详情保留抽屉(TenantUsageDetailDrawer),不跳独立详情页。
-->
<script setup lang="ts">
import { computed } from "vue";
import { Refresh } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import { ScrollText } from "lucide-vue-next";
import { PortalMetricGrid, PortalPagePanel } from "@/platform";
import { formatNumber, formatUSD } from "@/platform/ai/usage";

import { tenantUsageApi } from "./api";
import TenantUsageDetailDrawer from "./components/TenantUsageDetailDrawer.vue";
import TenantUsageFilters from "./components/TenantUsageFilters.vue";
import TenantUsageTable from "./components/TenantUsageTable.vue";
import { useTenantUsage } from "./composables/useTenantUsage";

const {
  changePage,
  changePageSize,
  detailOpen,
  filters,
  loading,
  openDetail,
  page,
  pageSize,
  reset,
  rows,
  search,
  selectedRecord,
  stats,
  successRate,
  total,
  users
} = useTenantUsage({ api: tenantUsageApi, onError: (message) => ElMessage.error(message) });

// 顶部指标卡:口径与原 PortalMetricGrid 自定义卡片一致
const metrics = computed(() => [
  {
    label: "总请求数",
    value: stats.value.total_requests.toLocaleString(),
    hint: `${stats.value.success_count.toLocaleString()} 成功 / ${stats.value.failed_count.toLocaleString()} 失败`
  },
  { label: "成功率", value: successRate.value, hint: "当前过滤范围" },
  { label: "总 Token", value: formatNumber(stats.value.total_tokens), hint: "当前过滤范围" },
  { label: "平台应收", value: formatUSD(stats.value.total_tenant_payable_usd), hint: "本租户应付平台" },
  {
    label: "用户扣款",
    value: formatUSD(stats.value.total_user_charged_usd),
    hint: `终端用户消费 · 均延 ${Math.round(stats.value.avg_latency_ms)} ms`
  }
]);
</script>

<template>
  <div class="page-container tenant-usage-page">
    <PortalPagePanel
      fill
      :icon="ScrollText"
      :breadcrumbs="[
        { label: '智能服务' },
        { label: '用量与分析' },
        { label: '使用记录' }
      ]"
      description="每行对应一次 AI API 调用,支持按时间、用户、模型、状态过滤。"
    >
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="reset">重置刷新</el-button>
      </template>

      <template #filters>
        <TenantUsageFilters v-model="filters" :loading="loading" :users="users" @search="search" />
      </template>

      <!-- 面板 body 无内边距,用 24px 容器承载指标卡与整宽表格 -->
      <div class="usage-body">
        <PortalMetricGrid :metrics="metrics" min-col-width="180px" />
        <TenantUsageTable
          :loading="loading"
          :page="page"
          :page-size="pageSize"
          :rows="rows"
          :total="total"
          @page-change="changePage"
          @page-size-change="changePageSize"
          @select="openDetail"
        />
      </div>
    </PortalPagePanel>

    <TenantUsageDetailDrawer :open="detailOpen" :row="selectedRecord" @close="detailOpen = false" />
  </div>
</template>

<style scoped>
.tenant-usage-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
  flex: 1;
  min-height: 0;
}

/* 面板 body 无内边距,24px 容器承载指标卡与表格(fill 模式下随之伸展) */
.usage-body {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 24px;
  flex: 1;
  min-width: 0;
  min-height: 0;
}
</style>
