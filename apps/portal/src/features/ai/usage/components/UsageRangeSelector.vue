<script setup lang="ts">
import type { WorkbenchRangeId, WorkbenchRangeOption } from "@/components/workbench/workbenchRanges";

defineProps<{
  modelValue: WorkbenchRangeId;
  options: WorkbenchRangeOption[];
}>();

const emit = defineEmits<{
  "update:modelValue": [value: WorkbenchRangeId];
  "update:customRange": [value: [number, number]];
}>();

const customRange = defineModel<[number, number] | null>("customRange", { default: null });

function choose(option: WorkbenchRangeOption) {
  emit("update:modelValue", option.id);
}
</script>

<template>
  <div class="usage-range-selector" aria-label="统计时间范围">
    <button
      v-for="option in options"
      :key="option.id"
      type="button"
      class="usage-range-selector__option"
      :class="{ 'is-active': modelValue === option.id }"
      @click="choose(option)"
    >
      {{ option.label }}
    </button>
    <el-date-picker
      v-if="modelValue === 'custom'"
      v-model="customRange"
      type="datetimerange"
      value-format="x"
      range-separator="至"
      start-placeholder="开始"
      end-placeholder="结束"
      size="small"
      @change="(value: [number, number] | null) => value && emit('update:customRange', value)"
    />
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
