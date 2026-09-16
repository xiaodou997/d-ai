<script setup lang="ts">
import { WORKBENCH_RANGE_OPTIONS, type WorkbenchRangeId } from "@/components/workbench/workbenchRanges";
defineProps<{ modelValue: WorkbenchRangeId; customRange: [Date, Date] | null }>();
const emit = defineEmits<{ "update:modelValue": [value: WorkbenchRangeId]; "update:customRange": [value: [Date, Date] | null] }>();
</script>
<template>
  <div class="record-range">
    <div class="record-range__options" role="group" aria-label="时间范围">
      <button v-for="option in WORKBENCH_RANGE_OPTIONS" :key="option.id" type="button" :aria-pressed="modelValue === option.id" @click="emit('update:modelValue', option.id)">{{ option.label }}</button>
    </div>
    <el-date-picker v-if="modelValue === 'custom'" :model-value="customRange" type="datetimerange" start-placeholder="开始时间" end-placeholder="结束时间" range-separator="至" @update:model-value="emit('update:customRange', $event)" />
  </div>
</template>
<style scoped>
.record-range { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-width: 0; }
.record-range__options { display: flex; flex-wrap: wrap; gap: 3px; padding: 3px; border-radius: var(--ds-radius-control); background: var(--ds-panel-muted); }
.record-range button { padding: 6px 10px; background: transparent; border: 0; border-radius: var(--ds-radius-control); color: var(--ds-muted); cursor: pointer; font-size: 12px; }
.record-range button[aria-pressed="true"] { background: var(--ds-panel); color: var(--ds-accent); box-shadow: var(--ds-shadow-sm); }
.record-range :deep(.el-date-editor) { max-width: 100%; }
</style>
