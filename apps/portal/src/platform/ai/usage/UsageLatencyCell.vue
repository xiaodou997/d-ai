<script setup lang="ts">
import { formatMs } from "./format";

withDefaults(
  defineProps<{
    latencyMs?: number | null;
    firstTokenMs?: number | null;
  }>(),
  { latencyMs: null, firstTokenMs: null }
);
</script>

<template>
  <span class="usage-latency-cell mono">
    <span class="usage-latency-cell__first">首字 {{ formatMs(firstTokenMs) }}</span>
    <span class="usage-latency-cell__total">总延迟 {{ formatMs(latencyMs) }}</span>
  </span>
</template>

<style scoped>
.usage-latency-cell {
  display: inline-flex;
  flex-direction: column;
  gap: 1px;
  line-height: 1.4;
}

.usage-latency-cell__first {
  color: var(--ds-info);
  font-size: 11px;
  font-weight: 700;
  white-space: nowrap;
}

.usage-latency-cell__total {
  color: var(--ds-ink);
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
}

.mono {
  font-family: "SF Mono", "Fira Code", monospace;
}
</style>
