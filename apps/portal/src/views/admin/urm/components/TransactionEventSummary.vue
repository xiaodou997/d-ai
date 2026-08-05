<script setup lang="ts">
withDefaults(
  defineProps<{
    row?: any
    label?: string
  }>(),
  {
    row: null,
    label: '积分'
  }
)

const STATUS_MAP: Record<string, string> = {
  pending: '进行中',
  succeeded: '成功',
  released: '已释放',
  cancelled: '取消',
  refunded: '已退款'
}
</script>

<template>
  <div v-if="row" class="event-summary">
    <div class="event-summary__row">
      <span class="event-summary__key">交易流水</span>
      <span class="event-summary__mono">{{ row.eventId }}</span>
    </div>
    <div class="event-summary__row">
      <span class="event-summary__key">租户{{ label }}</span>
      <span class="event-summary__val event-summary__val--tenant">{{ (row.tenantCredits || 0).toLocaleString() }}</span>
    </div>
    <div class="event-summary__row">
      <span class="event-summary__key">用户{{ label }}</span>
      <span class="event-summary__val event-summary__val--user">{{ (row.userCredits || 0).toLocaleString() }}</span>
    </div>
    <div class="event-summary__row">
      <span class="event-summary__key">当前状态</span>
      <span>{{ STATUS_MAP[row.status] ?? row.status }}</span>
    </div>
  </div>
</template>

<style scoped>
.event-summary {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 16px;
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel-muted);
  font-size: 13px;
}

.event-summary__row {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}

.event-summary__key {
  color: var(--ds-muted);
}

.event-summary__mono {
  font-family: var(--ds-font-mono);
  font-size: 12px;
  color: var(--ds-ink-soft);
}

.event-summary__val {
  font-weight: 700;
}

.event-summary__val--tenant {
  color: var(--ds-accent);
}

.event-summary__val--user {
  color: var(--ds-positive);
}
</style>
