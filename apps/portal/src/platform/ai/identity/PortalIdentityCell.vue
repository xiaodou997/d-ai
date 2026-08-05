<script setup lang="ts">
withDefaults(
  defineProps<{
    label: string;
    meta?: string;
  }>(),
  {
    meta: ""
  }
);
</script>

<template>
  <span class="portal-identity-cell">
    <span class="portal-identity-cell__label">{{ label }}</span>
    <span v-if="meta" class="portal-identity-cell__meta">{{ meta }}</span>
  </span>
</template>

<style scoped>
.portal-identity-cell {
  display: inline-flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
}

.portal-identity-cell__label {
  overflow: hidden;
  color: var(--ds-ink);
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.portal-identity-cell__meta {
  overflow: hidden;
  color: var(--ds-faint);
  font-family: "SF Mono", "Fira Code", monospace;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
