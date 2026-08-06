<!-- 交易流水弹窗内的语义提示条：danger / info / warning 三种语气，颜色全部走 ds token -->
<script setup lang="ts">
defineProps<{
  tone: 'danger' | 'info' | 'warning'
}>()
</script>

<template>
  <div class="tx-alert" :class="`tx-alert--${tone}`">
    <slot />
  </div>
</template>

<style scoped>
.tx-alert {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px 14px;
  border-radius: var(--ds-radius-control);
  font-size: 13px;
  font-weight: 600;
  line-height: 1.6;
}

.tx-alert :deep(p) {
  margin: 0;
}

/* 汇总行（如租户/用户积分合计） */
.tx-alert :deep(.tx-alert__row) {
  display: flex;
  gap: 24px;
  flex-wrap: wrap;
  font-weight: 500;
}

.tx-alert--danger {
  background: var(--ds-danger-soft);
  border: 1px solid color-mix(in srgb, var(--ds-danger) 22%, transparent);
  color: var(--ds-danger);
}

.tx-alert--info {
  background: var(--ds-info-soft);
  border: 1px solid color-mix(in srgb, var(--ds-info) 22%, transparent);
  color: var(--ds-info);
}

.tx-alert--warning {
  background: var(--ds-warning-soft);
  border: 1px solid color-mix(in srgb, var(--ds-warning) 22%, transparent);
  color: var(--ds-warning);
}
</style>
