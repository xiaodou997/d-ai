<script setup lang="ts">
import { computed, useSlots, type Component } from "vue";
import { RouterLink } from "vue-router";

export interface PortalPagePanelBreadcrumb {
  label: string;
  to?: string;
}

const props = defineProps<{
  /** 页头身份图标(lucide 组件),渲染在 accent-soft 徽章里 */
  icon?: Component;
  /** 面包屑路径,末级即页面标题(主题色显示,不再单独渲染大标题) */
  breadcrumbs: PortalPagePanelBreadcrumb[];
  /** 页面描述,以 "·" 尾随在面包屑同行,过长省略 */
  description?: string;
  /** 撑满父级 flex 容器剩余高度(页面根需 flex:1 + flex column);body 随之伸展,分页脚固定在底部 */
  fill?: boolean;
}>();

const slots = useSlots();

const hasFilters = computed(() => Boolean(slots.filters));
const hasPagination = computed(() => Boolean(slots.pagination));
const parents = computed(() => props.breadcrumbs.slice(0, -1));
const current = computed(() => props.breadcrumbs[props.breadcrumbs.length - 1]);
</script>

<template>
  <!-- 一体面板:页头(图标+面包屑标题+描述+操作) / 筛选 / 内容 / 分页 同卡 -->
  <section class="portal-page-panel" :class="{ 'portal-page-panel--fill': fill }">
    <header class="portal-page-panel__head">
      <div class="portal-page-panel__identity">
        <span v-if="icon" class="portal-page-panel__icon">
          <component :is="icon" :size="16" />
        </span>
        <nav class="portal-page-panel__crumbs" aria-label="面包屑">
          <template v-for="(crumb, index) in parents" :key="index">
            <RouterLink v-if="crumb.to" :to="crumb.to" class="portal-page-panel__crumb portal-page-panel__crumb--link">
              {{ crumb.label }}
            </RouterLink>
            <span v-else class="portal-page-panel__crumb">{{ crumb.label }}</span>
            <span class="portal-page-panel__sep">/</span>
          </template>
          <span v-if="current" class="portal-page-panel__current">{{ current.label }}</span>
        </nav>
        <span v-if="description" class="portal-page-panel__desc">{{ description }}</span>
      </div>
      <div v-if="$slots.actions" class="portal-page-panel__actions">
        <slot name="actions" />
      </div>
    </header>

    <div v-if="hasFilters" class="portal-page-panel__filters">
      <slot name="filters" />
    </div>

    <div class="portal-page-panel__body">
      <slot />
    </div>

    <footer v-if="hasPagination" class="portal-page-panel__pagination">
      <slot name="pagination" />
    </footer>
  </section>
</template>

<style scoped>
.portal-page-panel {
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.portal-page-panel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px 16px;
  padding: 15px 24px;
  border-bottom: 1px solid var(--ds-line);
}

.portal-page-panel__identity {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  min-width: 0;
}

.portal-page-panel__icon {
  flex: none;
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border-radius: 9px;
  background: var(--ds-accent-soft);
  color: var(--ds-accent);
}

.portal-page-panel__crumbs {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 13.5px;
  font-weight: 500;
  color: var(--ds-muted);
}

.portal-page-panel__crumb--link {
  color: var(--ds-muted);
  text-decoration: none;
}

.portal-page-panel__crumb--link:hover {
  color: var(--ds-accent);
}

.portal-page-panel__sep {
  font-size: 11px;
  color: var(--ds-faint);
}

/* 面包屑末级即页面标题:与父级同字号,只用主题色区分 */
.portal-page-panel__current {
  color: var(--ds-accent);
  font-weight: 500;
}

.portal-page-panel__desc {
  font-size: 12.5px;
  color: var(--ds-faint);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.portal-page-panel__desc::before {
  content: "·";
  margin-right: 8px;
}

.portal-page-panel__actions {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 10px;
}

.portal-page-panel__filters {
  padding: 14px 24px;
  border-bottom: 1px solid var(--ds-line);
  background: var(--ds-panel);
}

.portal-page-panel__body {
  min-width: 0;
}

/* fill 模式:面板撑满父级剩余高度,body 伸展、分页脚自然沉底 */
.portal-page-panel--fill {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.portal-page-panel--fill .portal-page-panel__body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.portal-page-panel__pagination {
  display: flex;
  align-items: center;
  padding: 12px 24px;
  border-top: 1px solid var(--ds-line);
  background: var(--ds-panel);
}

@media (max-width: 768px) {
  .portal-page-panel__head,
  .portal-page-panel__filters,
  .portal-page-panel__pagination {
    padding-inline: 16px;
  }
}
</style>
