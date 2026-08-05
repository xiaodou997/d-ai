<script setup lang="ts">
import { computed, useSlots } from "vue";

const slots = useSlots();
const hasActions = computed(() => Boolean(slots.actions));
</script>

<template>
  <div class="portal-filter-bar">
    <div class="portal-filter-bar__fields">
      <slot />
    </div>

    <div v-if="hasActions" class="portal-filter-bar__actions">
      <slot name="actions" />
    </div>
  </div>
</template>

<style scoped>
.portal-filter-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.portal-filter-bar__fields {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  min-width: 0;
  flex: 1;
}

.portal-filter-bar__actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
}

@media (max-width: 768px) {
  .portal-filter-bar__actions {
    margin-left: 0;
  }
}

/* 统一栏内所有控件（输入框 / 下拉框 / 日期选择器）的高度与描边，
   避免各控件高度不一致；focus 态统一走主题色。 */
.portal-filter-bar :deep(.el-input__wrapper),
.portal-filter-bar :deep(.el-select__wrapper),
.portal-filter-bar :deep(.el-range-editor.el-input__wrapper) {
  min-height: 40px;
  border-radius: var(--ds-radius-control);
  box-shadow: 0 0 0 1px var(--ds-line-strong) inset;
  background: var(--ds-panel);
}

.portal-filter-bar :deep(.el-input__wrapper:hover),
.portal-filter-bar :deep(.el-select__wrapper:hover),
.portal-filter-bar :deep(.el-range-editor.el-input__wrapper:hover) {
  box-shadow: 0 0 0 1px var(--ds-accent) inset;
}

.portal-filter-bar :deep(.el-input__wrapper.is-focus),
.portal-filter-bar :deep(.el-select__wrapper.is-focused),
.portal-filter-bar :deep(.el-range-editor.el-input__wrapper.is-focus) {
  box-shadow:
    0 0 0 1px var(--ds-accent) inset,
    0 0 0 3px var(--ds-accent-soft);
}

.portal-filter-bar :deep(.el-date-editor) {
  flex: 0 1 auto;
  min-width: 0;
  max-width: 100%;
}

.portal-filter-bar :deep(.el-date-editor--daterange),
.portal-filter-bar :deep(.el-date-editor--monthrange),
.portal-filter-bar :deep(.el-date-editor--yearrange) {
  width: min(var(--portal-filter-date-range-width, 280px), 100%);
}

.portal-filter-bar :deep(.el-date-editor--datetimerange),
.portal-filter-bar :deep(.el-date-editor--timerange) {
  width: min(var(--portal-filter-datetime-range-width, 340px), 100%);
}
</style>
