<script setup lang="ts">
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import { PortalGithubLink, PortalShellLayout, usePortalShellScaffold } from "@/platform";
import AnnouncementTopbarAction from "@/features/announcements/AnnouncementTopbarAction.vue";

import { portalEnv, themeForUserType } from "@/env";
import { profilePathForUserType } from "@/modules/portalModules";
import { useAuthStore } from "@/stores/auth";
import { useMenuStore } from "@/stores/menus";

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();
const menuStore = useMenuStore();

const userMenu = computed(() => authStore.isTenantOperations ? [] : [
  { id: "profile", label: "个人中心", to: profilePathForUserType(authStore.userType) }
]);
const user = computed(() => {
  const labels: Record<number, string> = {
    1: "超级管理员",
    2: "平台管理员",
    3: "租户",
    4: "终端用户"
  };
  return {
    name: authStore.username || "用户",
    subtitle: authStore.isTenantOperations ? `代运维 · ${authStore.tenantName}` : labels[authStore.userType] || ""
  };
});

// 动态主题：根据 userType 切换
const theme = computed(() => themeForUserType(authStore.userType));

const { handleLogout: performLogout } = usePortalShellScaffold({
  authStore,
  menuStore,
  routePath: () => route.path,
  watchUserType: true
});

async function exitTenantOperations() {
  authStore.exitTenantOperations();
  await router.replace("/admin/organization/tenants");
}

async function handleLogout() {
  if (authStore.isTenantOperations) {
    await exitTenantOperations();
    return;
  }
  await performLogout();
}
</script>

<template>
  <PortalShellLayout
    :theme="theme"
    :app-version="portalEnv.appVersion"
    brand="D-AI"
    brand-icon-url="/brand/dai-icon.png"
    :nav="menuStore.items"
    :user="user"
    :user-menu="userMenu"
    :logout-label="authStore.isTenantOperations ? '退出代运维' : '退出登录'"
    @logout="handleLogout"
  >
    <template #topbar-actions>
      <div v-if="authStore.isTenantOperations" class="tenant-operations-banner">
        <span>正在代运维：{{ authStore.tenantName }}</span>
        <button type="button" @click="exitTenantOperations">退出</button>
      </div>
      <PortalGithubLink />
      <AnnouncementTopbarAction />
    </template>
  </PortalShellLayout>
</template>

<style scoped>
.tenant-operations-banner {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px;
  border: 1px solid var(--ds-warning);
  border-radius: var(--ds-radius-control);
  background: var(--ds-warning-soft);
  color: var(--ds-warning);
  font-size: 13px;
  font-weight: 600;
}

.tenant-operations-banner button {
  border: 0;
  padding: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  text-decoration: underline;
  cursor: pointer;
}
</style>
