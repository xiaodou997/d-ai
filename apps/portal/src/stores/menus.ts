import { ref, watch } from "vue";
import { defineStore } from "pinia";

import { buildUnifiedNav } from "../menus/unifiedMenus";
import { useAuthStore } from "./auth";
import type { AppShellNavItem } from "@dai/app-core";

/**
 * 统一 Menu Store —— 根据 userType 动态生成菜单
 */
export const useMenuStore = defineStore("unified-menus", () => {
  const items = ref<AppShellNavItem[]>([]);
  const currentPath = ref("");

  function refresh(path?: string) {
    if (path) currentPath.value = path;
    const authStore = useAuthStore();
    items.value = buildUnifiedNav(authStore.userType || 4);
  }

  // userType 变化时自动刷新菜单
  const authStore = useAuthStore();
  watch(
    () => authStore.userType,
    () => refresh(currentPath.value)
  );

  return { items, refresh };
});
