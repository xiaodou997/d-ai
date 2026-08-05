<script setup lang="ts">
import { TrendingDown, TrendingUp } from "lucide-vue-next";

withDefaults(
  defineProps<{
    label: string;
    value: string;
    hint?: string;
    trend?: string;
    trendDirection?: "up" | "down";
  }>(),
  {
    trendDirection: "up"
  }
);
</script>

<template>
  <article class="ds-metric-card">
    <div class="ds-metric-card__header">
      <div class="ds-metric-card__label">{{ label }}</div>
      <span
        v-if="trend"
        class="ds-metric-card__trend"
        :class="`ds-metric-card__trend--${trendDirection}`"
      >
        <TrendingUp v-if="trendDirection === 'up'" :size="12" :stroke-width="2" />
        <TrendingDown v-else :size="12" :stroke-width="2" />
        {{ trend }}
      </span>
    </div>
    <div class="ds-metric-card__value">{{ value }}</div>
    <div v-if="hint" class="ds-metric-card__hint">{{ hint }}</div>
  </article>
</template>

<style scoped>
.ds-metric-card {
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  padding: 18px 20px;
  box-shadow: var(--ds-shadow-sm);
}

.ds-metric-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.ds-metric-card__label {
  color: var(--ds-muted);
  font-size: 12.5px;
  font-weight: 600;
}

.ds-metric-card__trend {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border-radius: var(--ds-radius-pill);
  padding: 2px 8px;
  font-size: 11.5px;
  font-weight: 600;
  line-height: 1.4;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

.ds-metric-card__trend--up {
  background: var(--ds-positive-soft);
  color: var(--ds-positive);
}

.ds-metric-card__trend--down {
  background: var(--ds-danger-soft);
  color: var(--ds-danger);
}

.ds-metric-card__value {
  margin-top: 8px;
  font-size: 30px;
  font-weight: 600;
  letter-spacing: -0.02em;
  line-height: 1.2;
  color: var(--ds-ink);
  font-variant-numeric: tabular-nums;
}

.ds-metric-card__hint {
  margin-top: 6px;
  color: var(--ds-muted);
  font-size: 12px;
  font-weight: 500;
}
</style>
