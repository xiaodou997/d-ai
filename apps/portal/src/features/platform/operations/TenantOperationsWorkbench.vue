<!--
  租户运营数据大盘:余额/到账/消费指标 + 用户消费贡献榜 + 近期用户消费。
  重构:页头 TenantWorkbenchHeader(旧 PortalPageHeader 封装) → PortalPagePanel 一体面板
       (图标徽章 + 面包屑标题 + 描述同行,#actions 保留时间窗切换与刷新),
       指标与双栏面板收进同卡 body 的 24px 容器;fill 链:根 flex:1 → PortalPagePanel fill → body 伸展。
       业务逻辑与请求不变;TenantWorkbenchHeader 由租户运营工作台使用,保留原文件。
-->
<script setup lang="ts">
import { LayoutDashboard, RefreshCw } from "lucide-vue-next";
import { useRouter } from "vue-router";
import { PortalPagePanel } from "@/platform";

import TenantWorkbenchRangeTabs from "@/components/workbench/TenantWorkbenchRangeTabs.vue";
import { WORKBENCH_RANGE_OPTIONS } from "@/components/workbench/workbenchRanges";
import TenantBusinessMetrics from "./components/TenantBusinessMetrics.vue";
import TenantConsumptionRanking from "./components/TenantConsumptionRanking.vue";
import TenantRecentConsumption from "./components/TenantRecentConsumption.vue";
import { useTenantOperationsDashboard } from "./composables/useTenantOperationsDashboard";

const router = useRouter();
const dashboard = useTenantOperationsDashboard();
</script>

<template>
  <div class="page-container operations-workbench">
    <PortalPagePanel
      fill
      :icon="LayoutDashboard"
      :breadcrumbs="[{ label: '租户运营' }, { label: '概览' }, { label: '工作台' }]"
      description="关注收到的货款、用户消费和核心用户贡献。"
    >
      <template #actions>
        <TenantWorkbenchRangeTabs
          :model-value="dashboard.selectedRangeId.value"
          :options="WORKBENCH_RANGE_OPTIONS"
          :loading="dashboard.loading.value"
          aria-label="租户经营数据时间范围"
          tone="amber"
          @update:model-value="dashboard.selectRange"
        />
        <el-button
          type="primary"
          size="small"
          class="!ml-0 !h-9 !rounded-lg !px-4 !font-bold"
          :loading="dashboard.loading.value"
          @click="dashboard.refresh"
        >
          <template #icon><RefreshCw :size="15" /></template>
          刷新
        </el-button>
      </template>

      <!-- 面板 body 无内边距,24px 容器承载指标区与双栏面板(fill 模式下随之伸展) -->
      <div class="operations-workbench__body">
        <TenantBusinessMetrics
          :service-balance="dashboard.serviceBalance.value"
          :overview="dashboard.overview.value"
          :range-label="dashboard.selectedRangeLabel.value"
          :loading="dashboard.summaryLoading.value"
          :service-balance-loading="dashboard.serviceBalanceLoading.value"
          @open-settlement="router.push('/tenant/account?tab=balance')"
          @top-up-service-balance="router.push('/tenant/account?action=buy')"
        />

        <div class="operations-workbench__insights">
          <TenantConsumptionRanking
            :items="dashboard.consumptionRanking.value"
            :range-label="dashboard.selectedRangeLabel.value"
            :loading="dashboard.rankingLoading.value"
            @open-details="router.push('/tenant/account?tab=points')"
          />
          <TenantRecentConsumption
            :items="dashboard.recentConsumption.value"
            :range-label="dashboard.selectedRangeLabel.value"
            :loading="dashboard.recentLoading.value"
            @open-details="router.push('/tenant/account?tab=points')"
          />
        </div>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
/* fill 链:页面根 flex:1 + flex column,面板 fill 撑满一屏 */
.operations-workbench {
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}

.operations-workbench__body {
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
  gap: 24px;
  padding: 24px;
}

.operations-workbench__insights {
  display: grid;
  grid-template-columns: minmax(0, 1.05fr) minmax(420px, 0.95fr);
  gap: 18px;
  align-items: start;
}

@media (max-width: 1180px) {
  .operations-workbench__insights {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 640px) {
  .operations-workbench__body {
    gap: 20px;
    padding: 16px;
  }
}
</style>
