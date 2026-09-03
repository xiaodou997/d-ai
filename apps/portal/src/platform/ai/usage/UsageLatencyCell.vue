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
    <span class="usage-latency-cell__main">{{ formatMs(latencyMs) }}</span>
    <span v-if="firstTokenMs" class="usage-latency-cell__first">首字节 {{ formatMs(firstTokenMs) }}</span>
  </span>
</template>

<style scoped>
.usage-latency-cell {
  display: inline-flex;
  flex-direction: column;
  gap: 1px;
  line-height: 1.4;
}

.usage-latency-cell__main {
  color: var(--ds-ink);
  font-size: 12px;
}

.usage-latency-cell__first {
  color: var(--ds-faint);
  font-size: 11px;
  white-space: nowrap;
}

.mono {
  font-family: "SF Mono", "Fira Code", monospace;
}
</style>
