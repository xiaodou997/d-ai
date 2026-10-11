<script setup lang="ts">
import { DsButton } from "@/shared/ui";

defineProps<{ title: string; errors: string[]; loading: boolean }>();
defineEmits<{ retry: [] }>();
</script>

<template>
  <article v-if="errors.length" class="finance-card finance-unavailable" role="alert" :aria-label="`${title}加载失败`">
    <h2>{{ title }}暂不可用</h2>
    <p>{{ errors.join("、") }}加载失败，暂时无法展示准确数据。</p>
    <DsButton variant="secondary" size="sm" :loading="loading" @click="$emit('retry')">重新加载</DsButton>
  </article>
  <slot v-else />
</template>

<style scoped>
.finance-unavailable {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  gap: 12px;
  padding: 24px;
  color: var(--ds-muted);
}
.finance-unavailable h2 {
  margin: 0;
  color: var(--ds-ink);
  font-size: 16px;
}
.finance-unavailable p {
  margin: 0;
  font-size: 13px;
}
</style>
