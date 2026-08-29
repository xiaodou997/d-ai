<script setup lang="ts">
import { computed } from "vue";

import { formatMs } from "./format";

const props = withDefaults(
  defineProps<{
    totalMs?: number | null;
    firstResponseByteMs?: number | null;
    headerMs?: number | null;
    compact?: boolean;
  }>(),
  {
    totalMs: null,
    firstResponseByteMs: null,
    headerMs: null,
    compact: false
  }
);

const diagnosticTooltip = computed(() => {
  const parts: string[] = [];
  if (props.headerMs) parts.push(`连接 ${formatMs(props.headerMs)}`);
  return parts.join(" · ");
});
</script>

<template>
  <span class="usage-timing-cell mono">
    <span class="usage-timing-cell__main">{{ formatMs(totalMs) }}</span>
    <span v-if="firstResponseByteMs || headerMs" class="usage-timing-cell__meta">
      <span v-if="firstResponseByteMs" class="usage-timing-cell__first">首响 {{ formatMs(firstResponseByteMs) }}</span>
      <el-tooltip v-if="compact && diagnosticTooltip" :content="diagnosticTooltip" placement="top">
        <span class="usage-timing-cell__hint" @click.stop>?</span>
      </el-tooltip>
      <span v-else-if="headerMs" class="usage-timing-cell__diag">连接 {{ formatMs(headerMs) }}</span>
    </span>
  </span>
</template>

<style scoped>
.usage-timing-cell {
  display: inline-flex;
  flex-direction: column;
  gap: 2px;
  line-height: 1.3;
}

.usage-timing-cell__main {
  color: var(--ds-ink);
  font-size: 12px;
  font-weight: 700;
}

.usage-timing-cell__meta {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.usage-timing-cell__first,
.usage-timing-cell__diag {
  color: var(--ds-faint);
  font-size: 11px;
  white-space: nowrap;
}

.usage-timing-cell__first {
  color: var(--ds-info);
}

.usage-timing-cell__hint {
  display: inline-grid;
  place-items: center;
  width: 14px;
  height: 14px;
  border: 1px solid color-mix(in srgb, var(--ds-info) 22%, var(--ds-info-soft));
  border-radius: var(--ds-radius-pill);
  background: color-mix(in srgb, var(--ds-info-soft) 78%, var(--ds-white) 22%);
  color: var(--ds-info);
  font-size: 10px;
  font-weight: 700;
  cursor: help;
  flex: 0 0 auto;
}

.mono {
  font-family: "SF Mono", "Fira Code", monospace;
}
</style>
