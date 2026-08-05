<script setup lang="ts">
defineProps<{
  label: string;
}>();

defineSlots<{ default?(): unknown }>();
</script>

<template>
  <label class="ds-filter-field">
    <span class="ds-filter-field__label">{{ label }}</span>
    <slot />
  </label>
</template>

<style scoped>
.ds-filter-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.ds-filter-field__label {
  font-size: 12px;
  font-weight: 500;
  color: var(--ds-muted);
}
</style>
