<!--
  AI 工作台区块(仅 DashboardView 使用):一体面板 body 内的分区,
  头部 = eyebrow + 标题 + 描述 + 右侧操作,区块之间只用 1px 分隔线,不再各自成卡。
-->
<script setup lang="ts">
defineProps<{
  eyebrow?: string;
  title: string;
  description?: string;
}>();
</script>

<template>
  <section class="ai-wb-section">
    <header class="ai-wb-section__head">
      <div class="ai-wb-section__copy">
        <p v-if="eyebrow" class="ai-wb-section__eyebrow">{{ eyebrow }}</p>
        <h2 class="ai-wb-section__title">{{ title }}</h2>
        <p v-if="description" class="ai-wb-section__desc">{{ description }}</p>
      </div>
      <div v-if="$slots.actions" class="ai-wb-section__actions">
        <slot name="actions" />
      </div>
    </header>
    <slot />
  </section>
</template>

<style scoped>
.ai-wb-section {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 18px;
  padding: 24px;
}

.ai-wb-section + .ai-wb-section {
  border-top: 1px solid var(--ds-line);
}

.ai-wb-section__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 10px 16px;
}

.ai-wb-section__copy {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-width: 640px;
}

.ai-wb-section__eyebrow {
  margin: 0;
  color: var(--ds-accent-hover);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.ai-wb-section__title {
  margin: 0;
  color: var(--ds-ink);
  font-size: 16px;
  font-weight: 650;
}

.ai-wb-section__desc {
  margin: 0;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.6;
}

.ai-wb-section__actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
