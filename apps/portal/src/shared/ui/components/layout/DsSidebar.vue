<script setup lang="ts">
import { computed } from "vue";
import { RouterLink } from "vue-router";

import { resolveSidebarIcon } from "./sidebar-icons";

interface SidebarItem {
  id: string;
  label: string;
  to?: string;
  icon?: string;
  active?: boolean;
  disabled?: boolean;
  children?: SidebarItem[];
}

const props = withDefaults(
  defineProps<{
    groups: SidebarItem[];
    version?: string;
    /** 折叠态由壳层持有(DsAppShell),折叠按钮在顶栏品牌区 */
    collapsed?: boolean;
  }>(),
  {
    collapsed: false
  }
);

const displayVersion = computed(() => {
  const raw = props.version?.trim();
  if (!raw) return "";
  return raw.startsWith("v") ? raw : `v${raw}`;
});

function isGroup(item: SidebarItem): boolean {
  return Boolean(item.children && item.children.length);
}
</script>

<template>
  <nav class="ds-sidebar" :class="{ 'is-collapsed': collapsed }">
    <template v-for="group in groups" :key="group.id">
      <!-- 直达菜单（一级模块下没有目录分组的叶子） -->
      <RouterLink
        v-if="!isGroup(group) && group.to && !group.disabled"
        class="ds-sidebar__link ds-sidebar__link--top"
        :class="{ 'is-active': group.active }"
        :to="group.to"
      >
        <component :is="resolveSidebarIcon(group.icon)" v-if="resolveSidebarIcon(group.icon)" :size="16" class="ds-sidebar__icon" aria-hidden="true" />
        <span v-else class="ds-sidebar__dot" aria-hidden="true" />
        <span class="ds-sidebar__label">{{ group.label }}</span>
      </RouterLink>
      <span
        v-else-if="!isGroup(group)"
        class="ds-sidebar__link ds-sidebar__link--top is-disabled"
      >
        <component :is="resolveSidebarIcon(group.icon)" v-if="resolveSidebarIcon(group.icon)" :size="16" class="ds-sidebar__icon" aria-hidden="true" />
        <span v-else class="ds-sidebar__dot" aria-hidden="true" />
        <span class="ds-sidebar__label">{{ group.label }}</span>
      </span>

      <!-- 分组（目录标题 → 菜单，恒展开不折叠，对齐 V1） -->
      <div v-else class="ds-sidebar__group">
        <div class="ds-sidebar__group-head">{{ group.label }}</div>
        <div class="ds-sidebar__group-body">
          <template v-for="child in group.children" :key="child.id">
            <RouterLink
              v-if="child.to && !child.disabled"
              class="ds-sidebar__link"
              :class="{ 'is-active': child.active }"
              :to="child.to"
            >
              <component :is="resolveSidebarIcon(child.icon)" v-if="resolveSidebarIcon(child.icon)" :size="16" class="ds-sidebar__icon" aria-hidden="true" />
              <span v-else class="ds-sidebar__dot" aria-hidden="true" />
              <span class="ds-sidebar__label">{{ child.label }}</span>
            </RouterLink>
            <span v-else class="ds-sidebar__link is-disabled">
              <component :is="resolveSidebarIcon(child.icon)" v-if="resolveSidebarIcon(child.icon)" :size="16" class="ds-sidebar__icon" aria-hidden="true" />
              <span v-else class="ds-sidebar__dot" aria-hidden="true" />
              <span class="ds-sidebar__label">{{ child.label }}</span>
            </span>
          </template>
        </div>
      </div>
    </template>

    <div v-if="displayVersion" class="ds-sidebar__footer">
      <span class="ds-sidebar__footer-label">系统版本</span>
      <span class="ds-sidebar__version">{{ displayVersion }}</span>
    </div>
  </nav>
</template>

<style scoped>
.ds-sidebar {
  display: flex;
  height: 100%;
  flex-direction: column;
  gap: 2px;
  overflow-y: auto;
  overflow-x: hidden;
  border-right: 1px solid var(--ds-line);
  background: var(--ds-panel);
  padding: 14px 12px 16px;
}

.ds-sidebar__footer {
  margin-top: auto;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 18px 10px 0;
}

.ds-sidebar__footer-label {
  color: var(--ds-faint);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.ds-sidebar__version {
  display: inline-flex;
  align-items: center;
  width: fit-content;
  min-height: 28px;
  padding: 0 10px;
  border: 1px solid color-mix(in srgb, var(--ds-accent) 18%, transparent);
  border-radius: 999px;
  background: color-mix(in srgb, var(--ds-accent-soft) 78%, var(--ds-panel));
  color: var(--ds-accent);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.01em;
  white-space: nowrap;
}

.ds-sidebar__group {
  margin-top: 6px;
}

.ds-sidebar__group-head {
  padding: 12px 10px 6px;
  color: var(--ds-faint);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  white-space: nowrap;
}

.ds-sidebar__group-body {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.ds-sidebar__link {
  position: relative;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border-radius: var(--ds-radius-control);
  color: var(--ds-ink-soft);
  font-size: 13.5px;
  font-weight: 500;
  text-decoration: none;
  white-space: nowrap;
  transition:
    background-color 130ms ease,
    color 130ms ease;
}

/* 菜单图标:默认浅灰,hover 加深,选中跟随主题色 */
.ds-sidebar__icon {
  flex: 0 0 auto;
  color: var(--ds-faint);
  transition: color 130ms ease;
}

.ds-sidebar__link:hover .ds-sidebar__icon {
  color: var(--ds-ink-soft);
}

.ds-sidebar__link.is-active .ds-sidebar__icon {
  color: var(--ds-accent);
}

.ds-sidebar__link--top {
  font-weight: 600;
}

.ds-sidebar__link:hover {
  background: var(--ds-panel-muted);
  color: var(--ds-ink);
}

.ds-sidebar__link.is-active {
  background: var(--ds-accent-soft);
  color: var(--ds-accent);
  font-weight: 600;
}

.ds-sidebar__link.is-active::before {
  content: "";
  position: absolute;
  left: 0;
  top: 7px;
  bottom: 7px;
  width: 3px;
  border-radius: var(--ds-radius-pill);
  background: var(--ds-accent);
}

.ds-sidebar__link.is-disabled {
  color: var(--ds-faint);
  cursor: not-allowed;
}

.ds-sidebar__link.is-disabled:hover {
  background: transparent;
  color: var(--ds-faint);
}

/* 折叠态下的居中圆点（当前菜单项没有图标，用圆点占位） */
.ds-sidebar__dot {
  display: none;
  width: 6px;
  height: 6px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: var(--ds-faint);
}

.ds-sidebar__link.is-active .ds-sidebar__dot {
  background: var(--ds-accent);
}

/* 折叠态（64px 窄轨）：隐藏文字，圆点 / 版本胶囊居中 */
.ds-sidebar.is-collapsed {
  padding: 14px 8px 16px;
}

.ds-sidebar.is-collapsed .ds-sidebar__label,
.ds-sidebar.is-collapsed .ds-sidebar__group-head,
.ds-sidebar.is-collapsed .ds-sidebar__footer-label {
  display: none;
}

.ds-sidebar.is-collapsed .ds-sidebar__dot {
  display: block;
}

.ds-sidebar.is-collapsed .ds-sidebar__link {
  justify-content: center;
  gap: 0;
  padding: 8px 0;
}

.ds-sidebar.is-collapsed .ds-sidebar__footer {
  align-items: center;
  padding: 18px 0 0;
}

.ds-sidebar.is-collapsed .ds-sidebar__version {
  padding: 0 6px;
  font-size: 11px;
}
</style>
