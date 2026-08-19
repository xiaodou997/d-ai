<script setup lang="ts">
import { RefreshCw } from "lucide-vue-next";

import { DsButton } from "@/shared/ui";
import { WORKBENCH_RANGE_OPTIONS, type WorkbenchRangeId } from "@/components/workbench/workbenchRanges";

withDefaults(defineProps<{
  modelValue: WorkbenchRangeId;
  loading?: boolean;
  updatedAt?: Date | null;
  showRange?: boolean;
}>(), {
  showRange: true
});

const emit = defineEmits<{
  "update:modelValue": [value: WorkbenchRangeId];
  refresh: [];
}>();

function formatUpdatedAt(value?: Date | null) {
  return value ? value.toLocaleTimeString("zh-CN", { hour12: false }) : "—";
}
</script>

<template>
  <div class="overview-range-controls">
    <div v-if="showRange" class="overview-range-controls__segments" role="group" aria-label="时间范围">
      <button
        v-for="option in WORKBENCH_RANGE_OPTIONS"
        :key="option.id"
        type="button"
        class="overview-range-controls__segment"
        :class="{ 'is-active': modelValue === option.id }"
        @click="emit('update:modelValue', option.id)"
      >
        {{ option.label.replace('最近', '').replace('近', '') }}
      </button>
    </div>
    <span class="overview-range-controls__updated">更新 {{ formatUpdatedAt(updatedAt) }}</span>
    <DsButton :disabled="loading" @click="emit('refresh')">
      <template #icon><RefreshCw :size="14" :class="{ 'is-spinning': loading }" /></template>
      刷新
    </DsButton>
  </div>
</template>

<style scoped>
.overview-range-controls { display: flex; align-items: center; justify-content: flex-end; gap: 10px; flex-wrap: wrap; }
.overview-range-controls__segments { display: inline-flex; padding: 3px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-control); background: var(--ds-panel-muted); }
.overview-range-controls__segment { border: 0; border-radius: var(--ds-radius-control); background: transparent; color: var(--ds-muted); cursor: pointer; padding: 6px 9px; font-size: 11px; white-space: nowrap; }
.overview-range-controls__segment:hover { color: var(--ds-ink); }
.overview-range-controls__segment.is-active { background: var(--ds-panel); color: var(--ds-accent); box-shadow: var(--ds-shadow-sm); font-weight: 650; }
.overview-range-controls__updated { color: var(--ds-muted); font-size: 11px; white-space: nowrap; }
.is-spinning { animation: overview-spin 900ms linear infinite; }
@keyframes overview-spin { to { transform: rotate(360deg); } }
@media (max-width: 760px) { .overview-range-controls { justify-content: flex-start; } .overview-range-controls__updated { order: 3; width: 100%; } }
</style>
