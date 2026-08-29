<script setup lang="ts">
withDefaults(
  defineProps<{
    tone?: "neutral" | "accent" | "positive" | "warning" | "danger" | "info";
  }>(),
  {
    tone: "neutral"
  }
);
</script>

<template>
  <span class="ds-tag" :class="`ds-tag--${tone}`">
    <span class="ds-tag__dot" aria-hidden="true" />
    <slot />
  </span>
</template>

<style scoped>
.ds-tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border-radius: var(--ds-radius-pill);
  padding: 1.5px 9px;
  font-size: 12px;
  font-weight: 500;
  line-height: 1.5;
  white-space: nowrap;
}

.ds-tag__dot {
  flex: none;
  width: 5px;
  height: 5px;
  border-radius: var(--ds-radius-circle);
  background: currentColor;
}

.ds-tag--neutral {
  background: var(--ds-panel-muted);
  color: var(--ds-muted);
}

.ds-tag--accent {
  background: color-mix(in srgb, var(--ds-accent) 10%, transparent);
  color: var(--ds-accent);
}

.ds-tag--positive {
  background: color-mix(in srgb, var(--ds-positive) 12%, transparent);
  color: var(--ds-positive);
}

.ds-tag--warning {
  background: color-mix(in srgb, var(--ds-warning) 14%, transparent);
  color: var(--ds-warning);
}

.ds-tag--danger {
  background: color-mix(in srgb, var(--ds-danger) 10%, transparent);
  color: var(--ds-danger);
}

.ds-tag--info {
  background: color-mix(in srgb, var(--ds-info) 12%, transparent);
  color: var(--ds-info);
}
</style>
