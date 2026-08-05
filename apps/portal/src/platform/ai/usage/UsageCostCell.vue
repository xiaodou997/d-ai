<script setup lang="ts">
import { computed } from "vue";

import { formatCredits } from "../utils";
import type { UsageCostSecondaryItem } from "./types";

const props = withDefaults(
  defineProps<{
    credits?: number | null;
    /** 次要费用明细（上游参考成本/租户应收/Key 额度等），空数组不渲染次行。 */
    secondary?: UsageCostSecondaryItem[];
  }>(),
  { credits: 0, secondary: () => [] }
);

const secondaryText = computed(() => props.secondary.map((item) => `${item.label} ${item.value}`).join(" · "));
</script>

<template>
  <span class="usage-cost-cell mono">
    <strong class="usage-cost-cell__main">{{ formatCredits(credits || 0) }}</strong>
    <el-tooltip v-if="secondary.length" :content="secondaryText" placement="top">
      <span class="usage-cost-cell__secondary">{{ secondaryText }}</span>
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
  color: var(--ds-positive);
  font-size: 13px;
  font-weight: 700;
}

.usage-cost-cell__secondary {
  color: var(--ds-faint);
  font-size: 11px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 160px;
}

.mono {
  font-family: "SF Mono", "Fira Code", monospace;
}
</style>
