<script setup lang="ts">
import { computed, ref, watch } from "vue";

import type { PortalThemeName } from "../../theme";

import DsSidebar from "./DsSidebar.vue";
import DsTopbar from "./DsTopbar.vue";

export interface AppShellUser {
  name: string;
  subtitle?: string;
}

export interface AppShellUserMenuItem {
  id: string;
  label: string;
  to: string;
}

export interface AppShellNavItem {
  id: string;
  label: string;
  to?: string;
  icon?: string;
  active?: boolean;
  disabled?: boolean;
  children?: AppShellNavItem[];
}

const props = withDefaults(
  defineProps<{
    theme: PortalThemeName;
    brand?: string;
    brandIconUrl?: string;
    version?: string;
    nav: AppShellNavItem[];
    user: AppShellUser;
    userMenu?: AppShellUserMenuItem[];
  }>(),
  {
    brand: "豆栈 DouStack"
  }
);

const emit = defineEmits<{ logout: [] }>();
defineSlots<{ default(): unknown; footer(): unknown; "topbar-actions"(): unknown }>();

const shellClass = computed(() => `ds-theme-${props.theme}`);

// 顶部一级模块 = nav 顶层节点
const modules = computed(() => props.nav);

// 找出当前激活模块：优先 active，回退第一个
const activeModule = computed<AppShellNavItem | undefined>(() => {
  return modules.value.find((item) => item.active) ?? modules.value[0];
});

// 左栏分组 = 激活模块的子树（目录 / 直达菜单）
const sidebarGroups = computed<AppShellNavItem[]>(() => activeModule.value?.children ?? []);

const activeModuleId = computed(() => activeModule.value?.id ?? "");

// 侧栏折叠态（壳层持有，持久化到 localStorage）
const SIDEBAR_COLLAPSED_KEY = "ds-sidebar-collapsed";
const sidebarCollapsed = ref(
  typeof window !== "undefined" &&
    window.localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === "1"
);
watch(sidebarCollapsed, (value) => {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(SIDEBAR_COLLAPSED_KEY, value ? "1" : "0");
});
</script>

<template>
  <div class="ds-app-shell" :class="shellClass" :data-theme="theme">
    <DsTopbar
      :brand="brand"
      :brand-icon-url="brandIconUrl"
      :modules="modules"
      :active-id="activeModuleId"
      :user="user"
      :user-menu="userMenu"
      :sidebar-collapsed="sidebarCollapsed"
      @logout="emit('logout')"
      @toggle-sidebar="sidebarCollapsed = !sidebarCollapsed"
    >
      <template v-if="$slots['topbar-actions']" #actions>
        <slot name="topbar-actions" />
      </template>
    </DsTopbar>
    <div class="ds-app-shell__body" :class="{ 'is-sidebar-collapsed': sidebarCollapsed }">
      <aside class="ds-app-shell__sidebar">
        <DsSidebar :collapsed="sidebarCollapsed" :groups="sidebarGroups" :version="version" />
      </aside>
      <main class="ds-app-shell__content">
        <div class="ds-app-shell__canvas">
          <slot />
        </div>
      </main>
    </div>
    <slot name="footer" />
  </div>
</template>

<style scoped>
.ds-app-shell {
  display: flex;
  min-height: 100dvh;
  flex-direction: column;
  background: var(--ds-paper);
  color: var(--ds-ink);
}

.ds-app-shell__body {
  display: grid;
  grid-template-columns: 260px minmax(0, 1fr);
  flex: 1;
  min-height: 0;
  transition: grid-template-columns 160ms ease;
}

.ds-app-shell__body.is-sidebar-collapsed {
  grid-template-columns: 64px minmax(0, 1fr);
}

.ds-app-shell__sidebar {
  position: sticky;
  top: 56px;
  align-self: start;
  height: calc(100dvh - 56px);
  overflow: hidden;
}

.ds-app-shell__content {
  min-width: 0;
  min-height: 0;
  /* Pages scroll at the document level; an overflow ancestor breaks nested sticky navigation. */
  overflow: visible;
  padding: 24px 32px 48px;
}

.ds-app-shell__canvas {
  width: 100%;
  /* 内容区随窗口铺开,不设 max-width;宽屏下由表格/卡片自身拉伸 */
  min-width: 0;
  /* 视口高 - 顶栏 56px - 内容上下 padding(24+48),短内容页也能撑到底部 */
  min-height: calc(100dvh - 56px - 72px);
  display: flex;
  flex-direction: column;
}

@media (max-width: 960px) {
  .ds-app-shell__body,
  .ds-app-shell__body.is-sidebar-collapsed {
    grid-template-columns: 1fr;
  }

  .ds-app-shell__sidebar {
    display: none;
  }

  .ds-app-shell__content {
    padding: 16px;
  }

  .ds-app-shell__canvas {
    min-height: calc(100dvh - 56px - 32px);
  }
}
</style>
