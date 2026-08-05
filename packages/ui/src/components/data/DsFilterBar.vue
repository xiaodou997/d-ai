<script setup lang="ts">
defineSlots<{ default?(): unknown; actions?(): unknown }>();
</script>

<template>
  <div class="ds-filter-bar">
    <slot />
    <div v-if="$slots.actions" class="ds-filter-bar__actions">
      <slot name="actions" />
    </div>
  </div>
</template>

<style scoped>
.ds-filter-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 12px;
}

.ds-filter-bar__actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
}
</style>
