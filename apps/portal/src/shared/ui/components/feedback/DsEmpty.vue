<script setup lang="ts">
import { Inbox } from "lucide-vue-next";

withDefaults(
  defineProps<{
    title?: string;
    description?: string;
  }>(),
  {
    title: "暂无数据"
  }
);

defineSlots<{ icon?(): unknown; action?(): unknown }>();
</script>

<template>
  <div class="ds-empty">
    <div class="ds-empty__icon">
      <slot name="icon">
        <Inbox :size="24" :stroke-width="1.5" />
      </slot>
    </div>
    <div class="ds-empty__title">{{ title }}</div>
    <p v-if="description" class="ds-empty__description">{{ description }}</p>
    <div v-if="$slots.action" class="ds-empty__action">
      <slot name="action" />
    </div>
  </div>
</template>

<style scoped>
.ds-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 40px 24px;
  text-align: center;
}

.ds-empty__icon {
  display: grid;
  place-items: center;
  width: 56px;
  height: 56px;
  border-radius: 50%;
  background: var(--ds-accent-soft);
  color: var(--ds-accent);
}

.ds-empty__title {
  margin-top: 12px;
  font-size: 14px;
  font-weight: 600;
  color: var(--ds-ink-soft);
}

.ds-empty__description {
  margin: 6px 0 0;
  font-size: 12.5px;
  color: var(--ds-muted);
  line-height: 1.6;
}

.ds-empty__action {
  margin-top: 16px;
}
</style>
