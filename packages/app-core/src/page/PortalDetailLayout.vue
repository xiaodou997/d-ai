<script setup lang="ts">
import { computed, useSlots } from "vue";

const props = withDefaults(
  defineProps<{
    /** 左侧摘要栏宽度，仅在提供 #summary 时生效 */
    summaryWidth?: string;
  }>(),
  {
    summaryWidth: "320px"
  }
);

const slots = useSlots();
const hasSummary = computed(() => Boolean(slots.summary));
</script>

<template>
  <div
    class="portal-detail-layout"
    :class="{ 'portal-detail-layout--split': hasSummary }"
    :style="{ '--portal-summary-width': props.summaryWidth }"
  >
    <aside v-if="hasSummary" class="portal-detail-layout__summary">
      <slot name="summary" />
    </aside>

    <div class="portal-detail-layout__main">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.portal-detail-layout {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}

.portal-detail-layout--split {
  display: grid;
  grid-template-columns: var(--portal-summary-width) minmax(0, 1fr);
  align-items: start;
  gap: 16px;
}

.portal-detail-layout__summary {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
}

.portal-detail-layout__main {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}

@media (max-width: 960px) {
  .portal-detail-layout--split {
    grid-template-columns: 1fr;
  }
}
</style>
