<script setup lang="ts">
import { computed } from "vue";

const props = withDefaults(
  defineProps<{
    rows?: number;
    animated?: boolean;
  }>(),
  {
    rows: 4,
    animated: true
  }
);

const WIDTHS = ["100%", "88%", "94%", "80%"];

const widths = computed(() =>
  Array.from({ length: props.rows }, (_, index) => WIDTHS[index % WIDTHS.length])
);
</script>

<template>
  <div class="ds-skeleton" :class="{ 'ds-skeleton--animated': animated }" aria-hidden="true">
    <div
      v-for="(width, index) in widths"
      :key="index"
      class="ds-skeleton__row"
      :style="{ width }"
    />
  </div>
</template>

<style scoped>
.ds-skeleton {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.ds-skeleton__row {
  position: relative;
  height: 14px;
  overflow: hidden;
  border-radius: 6px;
  background: var(--ds-panel-muted);
}

.ds-skeleton--animated .ds-skeleton__row::after {
  content: "";
  position: absolute;
  inset: 0;
  background: linear-gradient(
    90deg,
    transparent 0%,
    rgba(255, 255, 255, 0.65) 50%,
    transparent 100%
  );
  transform: translateX(-100%);
  animation: ds-skeleton-shimmer 1.4s ease-in-out infinite;
}

@keyframes ds-skeleton-shimmer {
  to {
    transform: translateX(100%);
  }
}
</style>
