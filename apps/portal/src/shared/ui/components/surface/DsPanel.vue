<script setup lang="ts">
defineProps<{
  title: string;
  description?: string;
}>();
</script>

<template>
  <section class="ds-panel">
    <div class="ds-panel__head">
      <div>
        <h2 class="ds-panel__title">{{ title }}</h2>
        <p v-if="description" class="ds-panel__description">{{ description }}</p>
      </div>
      <div class="ds-panel__actions">
        <slot name="actions" />
      </div>
    </div>
    <div class="ds-panel__body">
      <slot />
    </div>
  </section>
</template>

<style scoped>
.ds-panel {
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.ds-panel__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 18px;
  border-bottom: 1px solid var(--ds-line);
  padding: 16px 18px;
}

.ds-panel__title {
  margin: 0;
  font-size: 15px;
  font-weight: 650;
  color: var(--ds-ink);
}

.ds-panel__description {
  margin: 5px 0 0;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.6;
}

.ds-panel__body {
  padding: 18px;
}
</style>
