<script setup lang="ts">
import { computed } from "vue";

import { formatCompactToken, formatTokenCount } from "./format";

const props = withDefaults(
  defineProps<{
    prompt?: number | null;
    completion?: number | null;
    cacheRead?: number | null;
    cacheWrite?: number | null;
    reasoning?: number | null;
    /** dense 模式下缓存/推理明细收进 tooltip（customer 端等窄表用）。 */
    dense?: boolean;
  }>(),
  { prompt: 0, completion: 0, cacheRead: 0, cacheWrite: 0, reasoning: 0, dense: false }
);

const extraParts = computed(() => {
  const parts: string[] = [];
  if (props.cacheRead) parts.push(`缓存读 ${formatTokenCount(props.cacheRead)}`);
  if (props.cacheWrite) parts.push(`缓存写 ${formatTokenCount(props.cacheWrite)}`);
  if (props.reasoning) parts.push(`推理 ${formatTokenCount(props.reasoning)}`);
  return parts;
});

const tooltipContent = computed(() =>
  [`输入 ${formatTokenCount(props.prompt)}`, `输出 ${formatTokenCount(props.completion)}`, ...extraParts.value].join("　")
);
</script>

<template>
  <el-tooltip v-if="dense" :content="tooltipContent" placement="top">
    <span class="usage-token-cell usage-token-cell--dense mono">
      <span class="usage-token-cell__in">↓{{ formatCompactToken(prompt) }}</span>
      <span class="usage-token-cell__out">↑{{ formatCompactToken(completion) }}</span>
      <span v-if="extraParts.length" class="usage-token-cell__more">+</span>
    </span>
  </el-tooltip>
  <span v-else class="usage-token-cell mono">
    <span class="usage-token-cell__main">
      <span class="usage-token-cell__in">↓{{ formatCompactToken(prompt) }}</span>
      <span class="usage-token-cell__out">↑{{ formatCompactToken(completion) }}</span>
    </span>
    <span v-if="extraParts.length" class="usage-token-cell__extra">{{ extraParts.join(" · ") }}</span>
  </span>
</template>

<style scoped>
.usage-token-cell {
  display: inline-flex;
  flex-direction: column;
  gap: 1px;
  line-height: 1.4;
}

.usage-token-cell--dense {
  flex-direction: row;
  align-items: center;
  gap: 6px;
}

.usage-token-cell__main {
  display: inline-flex;
  gap: 8px;
  align-items: baseline;
}

.usage-token-cell__in {
  color: var(--ds-positive);
  font-size: 12px;
  font-weight: 600;
}

.usage-token-cell__out {
  color: var(--ds-warning);
  font-size: 12px;
  font-weight: 600;
}

.usage-token-cell__extra {
  color: var(--ds-info);
  font-size: 11px;
  white-space: nowrap;
}

.usage-token-cell__more {
  color: var(--ds-info);
  font-size: 11px;
  font-weight: 700;
}

.mono {
  font-family: "SF Mono", "Fira Code", monospace;
}
</style>
