<script setup lang="ts">
import type { WorkbenchRangeId, WorkbenchRangeOption } from "@/components/workbench/workbenchRanges";

defineProps<{
  modelValue: WorkbenchRangeId;
  options: WorkbenchRangeOption[];
}>();

const emit = defineEmits<{
  "update:modelValue": [value: WorkbenchRangeId];
}>();
</script>

<template>
  <div class="usage-range-selector" aria-label="统计时间范围">
    <button
      v-for="option in options"
      :key="option.id"
      type="button"
      class="usage-range-selector__option"
      :class="{ 'is-active': modelValue === option.id }"
      @click="emit('update:modelValue', option.id)"
    >
      {{ option.label }}
    </button>
  </div>
</template>

<style scoped>
.usage-range-selector {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 2px;
  padding: 3px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-pill);
  background: var(--ds-panel-muted);
}

.usage-range-selector__option {
  border: 0;
  border-radius: var(--ds-radius-pill);
  background: transparent;
  color: var(--ds-muted);
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
  transition: color 120ms ease, background-color 120ms ease, box-shadow 120ms ease;
}

.usage-range-selector__option:hover {
  color: var(--ds-ink);
}

.usage-range-selector__option.is-active {
  background: var(--ds-panel);
  color: var(--ds-accent-hover);
  box-shadow: var(--ds-shadow-sm);
}
</style>
