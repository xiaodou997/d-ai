<!--
  用户端 AI 使用记录工作台:查看当前账号最近的智能服务请求、消耗、状态和错误信息。
  重构:迁移至新设计系统一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行,
       指标带置于 body 顶部 24px 容器,筛选/表格/分页同卡,表格统一 DsTable);
       业务逻辑、请求参数与详情抽屉不变(用户端无按 requestId 的详情接口,保留抽屉)。
-->
<script setup lang="ts">
import { Refresh } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import { ScrollText } from "lucide-vue-next";

import { PortalMetricGrid, PortalPagePanel } from "@/platform";
import { formatCredits, formatMs, formatTokenCount } from "@/platform/ai/usage";

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

      <template #filters>
        <CustomerUsageFilters
          v-model="filters"
          :loading="loading"
          @reset="reset"
          @search="search"
          @server-change="changeServerFilter"
        />
      </template>

      <!-- 指标带:面板 body 无内边距,用 24px 容器承载 -->
      <div class="usage-metrics">
        <PortalMetricGrid>
          <article class="usage-metric"><span>请求数</span><strong>{{ stats.totalRequests.toLocaleString("zh-CN") }}</strong><small>{{ stats.successRequests }} 成功 / {{ stats.failedRequests }} 异常</small></article>
          <article class="usage-metric"><span>成功率</span><strong :class="successRate === '-' || parseFloat(successRate) >= 95 ? 'is-good' : 'is-warn'">{{ successRate }}</strong><small>基于当前列表</small></article>
          <article class="usage-metric"><span>Token</span><strong>{{ formatTokenCount(stats.totalTokens) }}</strong><small>输入、输出和缓存合计</small></article>
          <article class="usage-metric"><span>消耗积分</span><strong class="is-accent">{{ formatCredits(stats.totalCredits) }}</strong><small>均延 {{ formatMs(Math.round(stats.avgLatency)) }}</small></article>
        </PortalMetricGrid>
      </div>

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
    </PortalPagePanel>

    <CustomerUsageDetailDrawer :open="detailOpen" :row="selectedRecord" @close="detailOpen = false" />
  </div>
</template>

<style scoped>
/* fill 链:页面根 flex 撑满,面板 fill 随之伸展 */
.customer-usage-page { display: flex; min-height: 0; flex: 1; flex-direction: column; gap: 20px; }
/* 指标带:24px 容器,与下方表格以 1px 分隔线分区 */
.usage-metrics { padding: 24px; border-bottom: 1px solid var(--ds-line); }
.usage-metric { display: flex; min-width: 0; flex-direction: column; gap: 6px; padding: 16px 18px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-panel); background: var(--ds-panel); box-shadow: var(--ds-shadow-sm); }
.usage-metric span, .usage-metric small { color: var(--ds-muted); font-size: 12px; }
.usage-metric strong { color: var(--ds-ink); font-size: 26px; font-weight: 650; }
.usage-metric strong.is-good { color: var(--ds-positive); }
.usage-metric strong.is-warn { color: var(--ds-warning); }
.usage-metric strong.is-accent { color: var(--ds-accent-hover); }
</style>
