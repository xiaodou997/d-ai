<!--
  租户端终端用户详情页:按路由参数 userId 聚合基础资料、充值、AI 配置与风险信号。
  重构:迁移至 DsUI 一体面板(PortalPagePanel:图标徽章+面包屑,面包屑「终端用户」可点回列表),
       面板 body 承载四个 user-overview 区块。
-->
<script setup lang="ts">
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import { UserRound } from "lucide-vue-next";

import { PortalPagePanel } from "@/platform";

import { portalEnv } from "@/env";
import { useAuthStore } from "@/stores/auth";
import UserOverviewActivityGrid from "./user-overview/components/UserOverviewActivityGrid.vue";
import UserOverviewControlGrid from "./user-overview/components/UserOverviewControlGrid.vue";
import UserOverviewHero from "./user-overview/components/UserOverviewHero.vue";
import UserOverviewMetrics from "./user-overview/components/UserOverviewMetrics.vue";
import { useTenantUserOverview } from "./user-overview/useTenantUserOverview";

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();

const userId = computed(() => (typeof route.params.userId === "string" ? route.params.userId : ""));
const serviceAvailability = computed(() => ({
  ai: hasServiceAccess("ai")
}));

const overview = useTenantUserOverview(userId, serviceAvailability);

function hasServiceAccess(service: "ai") {
  const clientID = portalEnv.serviceClientIds?.[service];
  return Boolean(clientID && authStore.hasClientAccess(clientID));
}

function goBack() {
  void router.push("/users");
}

function openAiConfig() {
  if (!userId.value || !serviceAvailability.value.ai) return;
  void router.push({
    name: "ai-user-management",
    params: { userId: userId.value }
  });
}

</script>

<template>
  <div class="page-container overview-page">
    <PortalPagePanel
      fill
      :icon="UserRound"
      :breadcrumbs="[
        { label: '租户运营' },
        { label: '用户运营' },
        { label: '终端用户', to: '/users' },
        { label: '用户详情' }
      ]"
      description="聚合基础资料、充值、AI 配置与风险信号"
    >
      <div class="overview-page__body">
        <UserOverviewHero
          :user-id="userId"
          :user="overview.user.value"
          :loading="overview.loading.value"
          :last-activity-time="overview.lastActivityTime.value"
          :group-accessible-count="overview.groupSummary.value.accessible"
          :warnings="overview.warnings.value"
          :ai-available="overview.serviceAvailability.value.ai"
          @back="goBack"
          @open-ai-config="openAiConfig"
          @refresh="overview.refresh"
        />

        <UserOverviewMetrics
          :user="overview.user.value"
          :recharge-total="overview.rechargeTotal.value"
          :latest-recharge-time="overview.rechargeRecords.value[0]?.createdTime ?? null"
          :ai-usage-stats="overview.aiUsageStats.value"
          :group-summary="overview.groupSummary.value"
          :risk-signals="overview.riskSignals.value"
          :activity-window-label="overview.activityWindowLabel"
          :ai-available="overview.serviceAvailability.value.ai"
        />

        <UserOverviewActivityGrid
          :loading="overview.loading.value"
          :recharge-records="overview.rechargeRecords.value"
          :recharge-total="overview.rechargeTotal.value"
          :ai-usage-logs="overview.aiUsageLogs.value"
          :activity-window-label="overview.activityWindowLabel"
          :ai-available="overview.serviceAvailability.value.ai"
        />

        <UserOverviewControlGrid
          :loading="overview.loading.value"
          :ai-policy="overview.aiPolicy.value"
          :accessible-groups="overview.accessibleGroups.value"
          :group-summary="overview.groupSummary.value"
          :risk-signals="overview.riskSignals.value"
          :ai-available="overview.serviceAvailability.value.ai"
        />
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.overview-page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

/* body 无内边距,24px 容器承载四个区块并负责纵向滚动 */
.overview-page__body {
  flex: 1;
  min-height: 0;
  padding: 24px;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 20px;
}
</style>
