<script setup lang="ts">
import { computed, ref, watch } from "vue";

import { applyPortalTheme, type PortalThemeName } from "../../theme";

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

watch(
  () => props.theme,
  (theme) => applyPortalTheme(theme),
  { immediate: true }
);

const sidebarGroups = computed<AppShellNavItem[]>(() => props.nav);

// 侧栏折叠态（壳层持有，持久化到 localStorage）
const SIDEBAR_COLLAPSED_KEY = "ds-sidebar-collapsed";
const mobileNavigationOpen = ref(false);
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
      :user="user"
      :user-menu="userMenu"
      :navigation-open="mobileNavigationOpen"
      :sidebar-collapsed="sidebarCollapsed"
      @logout="emit('logout')"
      @toggle-navigation="mobileNavigationOpen = !mobileNavigationOpen"
      @toggle-sidebar="sidebarCollapsed = !sidebarCollapsed"
    >
      <template v-if="$slots['topbar-actions']" #actions>
        <slot name="topbar-actions" />
      </template>
    </DsTopbar>
    <div class="ds-app-shell__body" :class="{ 'is-sidebar-collapsed': sidebarCollapsed }">
      <button
        v-if="mobileNavigationOpen"
        type="button"
        class="ds-app-shell__nav-backdrop"
        aria-label="关闭导航"
        @click="mobileNavigationOpen = false"
      />
      <aside
        class="ds-app-shell__sidebar"
        :class="{ 'is-mobile-open': mobileNavigationOpen }"
        @click="mobileNavigationOpen = false"
      >
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

.ds-app-shell__nav-backdrop {
  display: none;
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
    position: fixed;
    top: 56px;
    bottom: 0;
    left: 0;
    z-index: 25;
    display: block;
    width: min(300px, 86vw);
    height: auto;
    transform: translateX(-100%);
    background: var(--ds-panel);
    box-shadow: var(--ds-shadow-panel);
    pointer-events: none;
    transition: transform 160ms ease;
  }

  .ds-app-shell__sidebar.is-mobile-open {
    transform: translateX(0);
    pointer-events: auto;
  }

  .ds-app-shell__nav-backdrop {
    position: fixed;
    inset: 56px 0 0;
    z-index: 24;
    display: block;
    border: none;
    background: color-mix(in srgb, var(--ds-ink) 24%, transparent);
  }

  .ds-app-shell__content {
    padding: 16px;
  }

  .ds-app-shell__canvas {
    min-height: calc(100dvh - 56px - 32px);
  }
}
</style>
