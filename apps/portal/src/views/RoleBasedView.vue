<script setup lang="ts">
import { computed, defineAsyncComponent, type Component } from "vue";

import { useAuthStore } from "@/stores/auth";

type RoleView =
  | "overview"
  | "profile"
  | "user-management"
  | "account"
  | "recharge-records"
  | "transactions"
  | "groups"
  | "keys"
  | "api-keys"
  | "usage-records";

const props = defineProps<{ view: RoleView }>();
const authStore = useAuthStore();

const adminOverview = defineAsyncComponent(() => import("./admin/ai/DashboardView.vue"));
const tenantOverview = defineAsyncComponent(() => import("./tenant/ai/DashboardView.vue"));
const customerOverview = defineAsyncComponent(() => import("./customer/ai/WorkspaceView.vue"));
const adminProfile = defineAsyncComponent(() => import("./admin/ProfileView.vue"));
const tenantProfile = defineAsyncComponent(() => import("./tenant/ProfileView.vue"));
const customerProfile = defineAsyncComponent(() => import("./customer/ProfileView.vue"));
const adminUsers = defineAsyncComponent(() => import("./admin/platform/EndUsersView.vue"));
const tenantUsers = defineAsyncComponent(() => import("./tenant/platform/UsersView.vue"));
const tenantAccount = defineAsyncComponent(() => import("./tenant/platform/AccountCenterView.vue"));
const customerAccount = defineAsyncComponent(() => import("./customer/platform/AccountView.vue"));
const adminRecharges = defineAsyncComponent(() => import("./admin/platform/RechargeRecordsView.vue"));
const tenantRecharges = defineAsyncComponent(() => import("./tenant/platform/UserRechargeRecordsView.vue"));
const customerRecharges = defineAsyncComponent(() => import("./customer/platform/RechargeView.vue"));
const adminTransactions = defineAsyncComponent(() => import("./admin/platform/TransactionsView.vue"));
const tenantTransactions = defineAsyncComponent(() => import("./tenant/platform/TransactionsView.vue"));
const customerTransactions = defineAsyncComponent(() => import("./customer/platform/TransactionsView.vue"));
const tenantGroups = defineAsyncComponent(() => import("./tenant/ai/GroupManagementView.vue"));
const customerGroups = defineAsyncComponent(() => import("./customer/ai/GroupsView.vue"));
const tenantKeys = defineAsyncComponent(() => import("./tenant/ai/KeysView.vue"));
const customerKeys = defineAsyncComponent(() => import("./customer/ai/KeysView.vue"));
const tenantAPIKeys = defineAsyncComponent(() => import("./tenant/ai/ApiKeysView.vue"));
const customerAPIKeys = defineAsyncComponent(() => import("./customer/ai/ApiKeysView.vue"));
const tenantUsage = defineAsyncComponent(() => import("./tenant/ai/UserConsumptionView.vue"));
const customerUsage = defineAsyncComponent(() => import("./customer/ai/UsageRecordsView.vue"));

const views: Record<RoleView, Partial<Record<number, Component>>> = {
  overview: { 1: adminOverview, 2: adminOverview, 3: tenantOverview, 4: customerOverview },
  profile: { 1: adminProfile, 2: adminProfile, 3: tenantProfile, 4: customerProfile },
  "user-management": { 1: adminUsers, 2: adminUsers, 3: tenantUsers },
  account: { 3: tenantAccount, 4: customerAccount },
  "recharge-records": { 1: adminRecharges, 2: adminRecharges, 3: tenantRecharges, 4: customerRecharges },
  transactions: { 1: adminTransactions, 2: adminTransactions, 3: tenantTransactions, 4: customerTransactions },
  groups: { 3: tenantGroups, 4: customerGroups },
  keys: { 3: tenantKeys, 4: customerKeys },
  "api-keys": { 3: tenantAPIKeys, 4: customerAPIKeys },
  "usage-records": { 3: tenantUsage, 4: customerUsage }
};

const selectedView = computed(() => views[props.view][authStore.userType]);
</script>

<template>
  <component :is="selectedView" v-if="selectedView" />
</template>
