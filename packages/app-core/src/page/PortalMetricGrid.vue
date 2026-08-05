<script setup lang="ts">
import { computed } from "vue";

import { DsMetricCard } from "@dai/ui";

type Metric = {
  label: string;
  value: string;
  hint?: string;
};

const props = withDefaults(
  defineProps<{
    /** 指标数据；不传则用默认插槽自定义内容 */
    metrics?: Metric[];
    /** 单卡最小宽度，用于响应式自动换行（列数随容器宽度自适应） */
    minColWidth?: string;
  }>(),
  {
    minColWidth: "200px"
  }
);

const useMetrics = computed(() => Array.isArray(props.metrics) && props.metrics.length > 0);
</script>

<template>
  <div
    class="portal-metric-grid"
    :style="{ '--portal-metric-min': minColWidth }"
  >
    <slot>
      <template v-if="useMetrics">
        <DsMetricCard
          v-for="(m, i) in metrics"
          :key="i"
          :label="m.label"
          :value="m.value"
          :hint="m.hint"
        />
      </template>
    </slot>
  </div>
</template>

<style scoped>
.portal-metric-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(var(--portal-metric-min), 100%), 1fr));
  gap: 14px;
}
</style>
