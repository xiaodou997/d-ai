<script setup lang="ts">
import { computed } from "vue";

import { formatUSD2 } from "./format";
import type { UsageCostSecondaryItem } from "./types";

const props = withDefaults(
  defineProps<{
    amountUSD?: number | null;
    /** 主费用的展示标签；传入时会与金额显示在同一行。 */
    primaryLabel?: string;
    /** 次要费用明细；第一项展示为第二行，其余项目收进 tooltip。 */
    secondary?: UsageCostSecondaryItem[];
  }>(),
  { amountUSD: 0, secondary: () => [] }
);

const secondaryText = computed(() => props.secondary.map((item) => `${item.label} ${item.value}`).join(" · "));
</script>

<template>
  <span class="usage-cost-cell mono">
    <strong class="usage-cost-cell__main">
      <span v-if="primaryLabel" class="usage-cost-cell__label">{{ primaryLabel }}</span>
      <span>{{ formatUSD2(amountUSD || 0) }}</span>
    </strong>
    <span v-if="secondary[0]" class="usage-cost-cell__secondary">
      <span class="usage-cost-cell__label">{{ secondary[0].label }}</span>
      <span>{{ secondary[0].value }}</span>
    </span>
    <el-tooltip v-if="secondary.length > 1" :content="secondaryText" placement="top">
      <span class="usage-cost-cell__more">+{{ secondary.length - 1 }}</span>
    </el-tooltip>
  </span>
</template>

<style scoped>
.usage-cost-cell {
  display: inline-flex;
  flex-direction: column;
  gap: 1px;
  line-height: 1.4;
  min-width: 0;
}

.usage-cost-cell__main {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  color: var(--ds-accent);
  font-size: 13px;
  font-weight: 700;
}

.usage-cost-cell__secondary {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  color: var(--ds-info);
  font-size: 11px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 160px;
}

.usage-cost-cell__label {
  font-family: var(--ds-font-sans);
  font-size: 10px;
  font-weight: 600;
  white-space: nowrap;
}

.usage-cost-cell__more {
  color: var(--ds-faint);
  font-size: 10px;
}

.mono {
  font-family: "SF Mono", "Fira Code", monospace;
}
</style>
