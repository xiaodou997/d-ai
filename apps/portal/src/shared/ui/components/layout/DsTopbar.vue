<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from "vue";
import { RouterLink } from "vue-router";
import { ChevronsLeft, ChevronsRight, Menu, X } from "lucide-vue-next";

interface TopbarUser {
  name: string;
  subtitle?: string;
}

export interface TopbarUserMenuItem {
  id: string;
  label: string;
  to: string;
}

const props = defineProps<{
  brand: string;
  brandIconUrl?: string;
  user: TopbarUser;
  userMenu?: TopbarUserMenuItem[];
  navigationOpen?: boolean;
  /** 侧栏折叠态;传入时品牌区右端渲染折叠按钮,品牌区宽度与侧栏同步(260px/64px) */
  sidebarCollapsed?: boolean;
}>();

const emit = defineEmits<{ logout: []; toggleNavigation: []; toggleSidebar: [] }>();
defineSlots<{ actions(): unknown }>();

const open = ref(false);

function toggle() {
  open.value = !open.value;
}
function close() {
  open.value = false;
}
function onLogout() {
  close();
  emit("logout");
}

// Esc 关闭下拉
function onKeydown(e: KeyboardEvent) {
  if (e.key === "Escape") close();
}
if (typeof window !== "undefined") {
  window.addEventListener("keydown", onKeydown);
  onBeforeUnmount(() => window.removeEventListener("keydown", onKeydown));
}

const initial = computed(() => props.user.name.slice(0, 1) || "U");
</script>

<template>
  <header class="ds-topbar">
    <div class="ds-topbar__brand" :class="{ 'ds-topbar__brand--collapsed': sidebarCollapsed }">
      <div class="ds-topbar__mark">
        <img v-if="brandIconUrl" class="ds-topbar__mark-image" :src="brandIconUrl" alt="" />
        <span v-else>豆</span>
      </div>
      <span class="ds-topbar__brand-name">{{ brand }}</span>
      <button
        v-if="sidebarCollapsed !== undefined"
        type="button"
        class="ds-topbar__collapse"
        :aria-label="sidebarCollapsed ? '展开侧栏' : '收起侧栏'"
        :title="sidebarCollapsed ? '展开侧栏' : '收起侧栏'"
        @click="emit('toggleSidebar')"
      >
        <ChevronsRight v-if="sidebarCollapsed" :size="16" />
        <ChevronsLeft v-else :size="16" />
      </button>
    </div>

    <button
      type="button"
      class="ds-topbar__mobile-nav"
      :aria-label="navigationOpen ? '关闭导航' : '打开导航'"
      :title="navigationOpen ? '关闭导航' : '打开导航'"
      @click="emit('toggleNavigation')"
    >
      <X v-if="navigationOpen" :size="18" />
      <Menu v-else :size="18" />
    </button>

    <div class="ds-topbar__spacer" />

    <div v-if="$slots.actions" class="ds-topbar__actions">
      <slot name="actions" />
    </div>

    <div class="ds-topbar__user">
      <button type="button" class="ds-topbar__user-trigger" @click="toggle">
        <div class="ds-topbar__user-copy">
          <span class="ds-topbar__user-name">{{ user.name }}</span>
          <span v-if="user.subtitle" class="ds-topbar__user-subtitle">{{ user.subtitle }}</span>
        </div>
        <div class="ds-topbar__avatar">{{ initial }}</div>
      </button>

      <template v-if="open">
        <div class="ds-topbar__backdrop" @click="close" />
        <div class="ds-topbar__menu" role="menu">
          <RouterLink
            v-for="item in userMenu ?? []"
            :key="item.id"
            class="ds-topbar__menu-item"
            :to="item.to"
            role="menuitem"
            @click="close"
          >
            {{ item.label }}
          </RouterLink>
          <div v-if="(userMenu ?? []).length" class="ds-topbar__menu-divider" />
          <button type="button" class="ds-topbar__menu-item is-danger" role="menuitem" @click="onLogout">
            退出登录
          </button>
        </div>
      </template>
    </div>
  </header>
</template>

<style scoped>
.ds-topbar {
  position: sticky;
  top: 0;
  z-index: 30;
  display: flex;
  height: 56px;
  align-items: center;
  gap: 28px;
  border-bottom: 1px solid var(--ds-line);
  background: color-mix(in srgb, var(--ds-panel) 82%, transparent);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  padding: 0 20px 0 0;
}

/* 品牌区与侧栏同宽(260px,折叠 64px),右边界与侧栏分隔线对齐 */
.ds-topbar__brand {
  display: flex;
  align-items: center;
  align-self: stretch;
  gap: 10px;
  flex: 0 0 auto;
  width: 260px;
  padding: 0 10px 0 20px;
  border-right: 1px solid var(--ds-line);
  transition: width 160ms ease;
}

.ds-topbar__brand--collapsed {
  width: 64px;
  padding: 0;
  justify-content: center;
}

.ds-topbar__collapse {
  display: grid;
  place-items: center;
  flex: 0 0 auto;
  width: 26px;
  height: 26px;
  margin-left: auto;
  border: none;
  border-radius: var(--ds-radius-control);
  background: transparent;
  color: var(--ds-muted);
  cursor: pointer;
  transition:
    background-color 130ms ease,
    color 130ms ease;
}

.ds-topbar__collapse:hover {
  background: var(--ds-panel-muted);
  color: var(--ds-ink);
}

.ds-topbar__mobile-nav {
  display: none;
  place-items: center;
  width: 32px;
  height: 32px;
  border: none;
  border-radius: var(--ds-radius-control);
  background: transparent;
  color: var(--ds-ink-soft);
  cursor: pointer;
}

.ds-topbar__mobile-nav:hover {
  background: var(--ds-panel-muted);
  color: var(--ds-ink);
}

/* 折叠态只留展开按钮居中(桌面端);移动端侧栏隐藏,品牌区恢复自适应 */
@media (min-width: 961px) {
  .ds-topbar__brand--collapsed .ds-topbar__mark,
  .ds-topbar__brand--collapsed .ds-topbar__brand-name {
    display: none;
  }

  .ds-topbar__brand--collapsed .ds-topbar__collapse {
    margin-left: 0;
  }
}

@media (max-width: 960px) {
  .ds-topbar {
    padding: 0 20px;
  }

  .ds-topbar__brand {
    width: auto;
    padding: 0;
    border-right: none;
  }

  .ds-topbar__collapse {
    display: none;
  }

  .ds-topbar__mobile-nav {
    display: grid;
  }
}

.ds-topbar__mark {
  display: grid;
  height: 28px;
  width: 28px;
  place-items: center;
  border-radius: 8px;
  background: var(--ds-accent);
  color: var(--ds-accent-contrast);
  font-size: 15px;
  font-weight: 700;
}

.ds-topbar__mark-image {
  width: 100%;
  height: 100%;
  border-radius: 7px;
  object-fit: cover;
}

.ds-topbar__brand-name {
  font-size: 15px;
  font-weight: 700;
  letter-spacing: 0;
  color: var(--ds-ink);
}

.ds-topbar__spacer {
  flex: 1;
  min-width: 0;
}

.ds-topbar__user {
  position: relative;
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 0 0 auto;
}

.ds-topbar__actions {
  display: flex;
  align-items: center;
  flex: 0 0 auto;
}

.ds-topbar__user-trigger {
  display: flex;
  align-items: center;
  gap: 10px;
  border: none;
  background: transparent;
  cursor: pointer;
  padding: 4px 6px;
  border-radius: var(--ds-radius-control);
  transition: background-color 140ms ease;
}

.ds-topbar__user-trigger:hover {
  background: var(--ds-panel-muted);
}

.ds-topbar__backdrop {
  position: fixed;
  inset: 0;
  z-index: 40;
}

.ds-topbar__menu {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  z-index: 50;
  min-width: 160px;
  display: flex;
  flex-direction: column;
  padding: 6px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel);
  box-shadow: 0 12px 32px color-mix(in srgb, var(--ds-ink) 12%, transparent);
}

.ds-topbar__menu-item {
  display: flex;
  align-items: center;
  height: 34px;
  padding: 0 10px;
  border: none;
  background: transparent;
  border-radius: 8px;
  color: var(--ds-ink);
  font-size: 13px;
  font-weight: 600;
  text-decoration: none;
  text-align: left;
  cursor: pointer;
  transition:
    background-color 140ms ease,
    color 140ms ease;
}

.ds-topbar__menu-item:hover {
  background: var(--ds-panel-muted);
}

.ds-topbar__menu-item.is-danger {
  color: var(--ds-danger);
}

.ds-topbar__menu-item.is-danger:hover {
  background: var(--ds-danger-soft);
}

.ds-topbar__menu-divider {
  height: 1px;
  margin: 4px 2px;
  background: var(--ds-line);
}

.ds-topbar__user-copy {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  line-height: 1.2;
}

.ds-topbar__user-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--ds-ink);
}

.ds-topbar__user-subtitle {
  font-size: 11.5px;
  color: var(--ds-muted);
}

.ds-topbar__avatar {
  display: grid;
  height: 32px;
  width: 32px;
  place-items: center;
  border-radius: 50%;
  background: var(--ds-accent-soft);
  color: var(--ds-accent);
  font-size: 13px;
  font-weight: 700;
}

@media (max-width: 720px) {
  .ds-topbar {
    gap: 14px;
    padding: 0 12px;
  }

  .ds-topbar__brand-name,
  .ds-topbar__user-copy {
    display: none;
  }
}
</style>
