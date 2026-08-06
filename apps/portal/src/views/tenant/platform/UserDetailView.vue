<!--
  租户端终端用户详情页:按路由参数 userId 聚合基础资料、充值、AI 配置与风险信号。
  重构:迁移至 DsUI 一体面板(PortalPagePanel:图标徽章+面包屑,面包屑「终端用户」可点回列表),
       面板 body 承载四个 user-overview 区块。
-->
<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { UserRound } from "lucide-vue-next";

import { UserAiPolicyDrawer } from "@/features/ai/user-management";
import type { UserAiPolicyTarget } from "@/features/ai/user-management/model";
import { PortalPagePanel } from "@/platform";

import UserOverviewActivityGrid from "./user-overview/components/UserOverviewActivityGrid.vue";
import UserOverviewControlGrid from "./user-overview/components/UserOverviewControlGrid.vue";
import UserOverviewHero from "./user-overview/components/UserOverviewHero.vue";
import UserOverviewMetrics from "./user-overview/components/UserOverviewMetrics.vue";
import { useTenantUserOverview } from "./user-overview/useTenantUserOverview";

const route = useRoute();
const router = useRouter();

const userId = computed(() => (typeof route.params.userId === "string" ? route.params.userId : ""));
const serviceAvailability = computed(() => ({
  ai: true
}));

const overview = useTenantUserOverview(userId, serviceAvailability);
const aiPolicyDrawerOpen = ref(route.query.policy === "1");
const aiPolicyUser = computed<UserAiPolicyTarget | null>(() => {
  if (!userId.value) return null;
  return {
    userId: userId.value,
    username: overview.user.value?.username ?? `用户 ${userId.value}`
  };
});

function goBack() {
  void router.push("/tenant/users/directory");
}

function openAiConfig() {
  if (!userId.value || !serviceAvailability.value.ai) return;
  aiPolicyDrawerOpen.value = true;
}

function closeAiPolicy() {
  aiPolicyDrawerOpen.value = false;
  if (route.query.policy !== "1") return;
  const { policy: _policy, ...query } = route.query;
  void router.replace({ query });
}

watch(
  () => route.query.policy,
  (policy) => {
    if (policy === "1") aiPolicyDrawerOpen.value = true;
  }
);
</script>

<template>
  <div class="page-container overview-page">
    <PortalPagePanel
      fill
      :icon="UserRound"
      :breadcrumbs="[
        { label: '用户与权限' },
        { label: '用户管理', to: '/tenant/users/directory' },
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
          @open-ai-config="openAiConfig"
        />
      </div>
    </PortalPagePanel>

    <UserAiPolicyDrawer
      :open="aiPolicyDrawerOpen"
      :user="aiPolicyUser"
      @close="closeAiPolicy"
    />
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
