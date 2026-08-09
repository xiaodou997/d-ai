<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";
import { PortalShellLayout, usePortalShellScaffold } from "@/platform";

import { portalEnv, themeForUserType } from "@/env";
import { profilePathForUserType } from "@/modules/portalModules";
import { useAuthStore } from "@/stores/auth";
import { useMenuStore } from "@/stores/menus";

const route = useRoute();
const authStore = useAuthStore();
const menuStore = useMenuStore();

const userMenu = computed(() => [
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
    subtitle: labels[authStore.userType] || ""
  };
});

// 动态主题：根据 userType 切换
const theme = computed(() => themeForUserType(authStore.userType));

const { handleLogout } = usePortalShellScaffold({
  authStore: authStore as any,
  menuStore: menuStore as any,
  routePath: () => route.path,
  watchUserType: true
});
</script>

<template>
  <PortalShellLayout
    :theme="theme"
    :app-version="portalEnv.appVersion"
    :nav="menuStore.items"
    :user="user"
    :user-menu="userMenu"
    @logout="handleLogout"
  />
</template>
