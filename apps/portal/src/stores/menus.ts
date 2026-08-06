import { ref, watch } from "vue";
import { defineStore } from "pinia";

import { buildPortalNav } from "../modules/portalModules";
import { useAuthStore } from "./auth";
import type { AppShellNavItem } from "@/platform";

/**
 * Menu and routes share the portal module registry.
 */
export const useMenuStore = defineStore("unified-menus", () => {
  const items = ref<AppShellNavItem[]>([]);
  const currentPath = ref("");

  function refresh(path?: string) {
    if (path) currentPath.value = path;
    const authStore = useAuthStore();
    items.value = buildPortalNav(authStore.userType || 4, currentPath.value || "/overview");
  }

  // userType 变化时自动刷新菜单
  const authStore = useAuthStore();
  watch(
    () => authStore.userType,
    () => refresh(currentPath.value)
  );

  return { items, refresh };
});
