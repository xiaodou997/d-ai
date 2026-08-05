<script setup lang="ts">
import { RouterLink } from "vue-router";

export interface DsPageHeaderBreadcrumb {
  label: string;
  to?: string;
}

defineProps<{
  title: string;
  description?: string;
  breadcrumbs?: DsPageHeaderBreadcrumb[];
}>();

defineSlots<{ actions?(): unknown }>();
</script>

<template>
  <header class="ds-page-header">
    <nav v-if="breadcrumbs?.length" class="ds-page-header__breadcrumbs" aria-label="面包屑">
      <template v-for="(crumb, index) in breadcrumbs" :key="index">
        <span v-if="index > 0" class="ds-page-header__separator">/</span>
        <RouterLink
          v-if="crumb.to && index < breadcrumbs.length - 1"
          :to="crumb.to"
          class="ds-page-header__crumb ds-page-header__crumb--link"
        >
          {{ crumb.label }}
        </RouterLink>
        <span
          v-else
          class="ds-page-header__crumb"
          :class="{ 'ds-page-header__crumb--current': index === breadcrumbs.length - 1 }"
        >
          {{ crumb.label }}
        </span>
      </template>
    </nav>
    <div class="ds-page-header__row">
      <div class="ds-page-header__text">
        <h1 class="ds-page-header__title">{{ title }}</h1>
        <p v-if="description" class="ds-page-header__description">{{ description }}</p>
      </div>
      <div v-if="$slots.actions" class="ds-page-header__actions">
        <slot name="actions" />
      </div>
    </div>
  </header>
</template>

<style scoped>
.ds-page-header__breadcrumbs {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 500;
  letter-spacing: 0.01em;
  color: var(--ds-muted);
}

.ds-page-header__crumb--link {
  color: var(--ds-muted);
  text-decoration: none;
}

.ds-page-header__crumb--link:hover {
  color: var(--ds-accent);
}

.ds-page-header__crumb--current {
  color: var(--ds-ink);
}

.ds-page-header__separator {
  color: var(--ds-faint);
}

.ds-page-header__row {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  margin-top: 10px;
}

.ds-page-header__row:first-child {
  margin-top: 0;
}

.ds-page-header__text {
  min-width: 0;
}

.ds-page-header__title {
  margin: 0;
  font-size: 26px;
  font-weight: 650;
  letter-spacing: -0.02em;
  color: var(--ds-ink);
  line-height: 1.25;
}

.ds-page-header__description {
  margin: 8px 0 0;
  max-width: 760px;
  font-size: 13px;
  color: var(--ds-muted);
  line-height: 1.7;
}

.ds-page-header__actions {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 10px;
}
</style>
