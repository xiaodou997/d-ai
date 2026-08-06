<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    ops?: any[]
    terminalNote?: string
  }>(),
  {
    ops: () => [],
    terminalNote: ''
  }
)

type OpMeta = { label: string; icon: string; variant: string }

const OPS_META: Record<string, OpMeta> = {
  auto_released: { label: '自动释放', icon: '⏱', variant: 'released' },
  manual_confirm: { label: '手动确认扣款', icon: '✓', variant: 'confirm' },
  admin_cancelled: { label: '管理员取消', icon: '✕', variant: 'cancelled' },
  admin_dismissed: { label: '管理员免除', icon: '○', variant: 'dismissed' },
  refunded: { label: '退款', icon: '↩', variant: 'refunded' }
}

const metaOf = (action: string): OpMeta =>
  OPS_META[action] || { label: action, icon: '·', variant: 'default' }

const formatTime = (at?: string | number) =>
  at ? new Date(at).toLocaleString('zh-CN', { hour12: false }) : ''

const linesOf = (op: any): string[] => {
  const lines: string[] = []
  if (op.operator_id) lines.push(`操作人: ${op.operator_id}`)
  if (op.note) lines.push(op.note)
  if (op.reason) lines.push(op.reason)
  if (op.actual_tenant !== undefined) lines.push(`实扣租户: ${op.actual_tenant}  用户: ${op.actual_user}`)
  return lines
}
</script>

<template>
  <div v-if="!props.ops.length && !props.terminalNote" class="ops-timeline__empty">
    暂无操作记录
  </div>
  <div v-else class="ops-timeline">
    <p class="ops-timeline__title">操作时间线</p>
    <div class="ops-timeline__list">
      <div v-for="(op, i) in props.ops" :key="i" class="ops-timeline__item">
        <div class="ops-timeline__dot" :class="`ops-timeline__dot--${metaOf(op.action).variant}`">
          {{ metaOf(op.action).icon }}
        </div>
        <div class="ops-timeline__body">
          <div class="ops-timeline__head">
            <span class="ops-timeline__label">{{ metaOf(op.action).label }}</span>
            <span class="ops-timeline__time">{{ formatTime(op.at) }}</span>
          </div>
          <p v-for="(line, j) in linesOf(op)" :key="j" class="ops-timeline__line">{{ line }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ops-timeline__empty {
  padding: 16px 32px;
  color: var(--ds-faint);
  font-size: 12px;
}

.ops-timeline {
  padding: 16px 32px;
}

.ops-timeline__title {
  margin: 0 0 12px;
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--ds-faint);
}

.ops-timeline__list {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding-left: 24px;
}

/* 时间线连接线 */
.ops-timeline__list::before {
  content: "";
  position: absolute;
  left: 8px;
  top: 4px;
  bottom: 4px;
  width: 1px;
  background: var(--ds-line);
}

.ops-timeline__item {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}

.ops-timeline__dot {
  flex-shrink: 0;
  width: 16px;
  height: 16px;
  margin-top: 2px;
  margin-left: -24px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: var(--ds-accent-contrast);
  font-size: 9px;
  font-weight: 800;
}

.ops-timeline__dot--released { background: var(--ds-danger); }
.ops-timeline__dot--confirm { background: var(--ds-info); }
.ops-timeline__dot--cancelled { background: var(--ds-faint); }
.ops-timeline__dot--dismissed { background: var(--ds-warning); }
.ops-timeline__dot--refunded { background: var(--ds-positive); }
.ops-timeline__dot--default { background: var(--ds-line-strong); }

.ops-timeline__body {
  flex: 1;
  min-width: 0;
}

.ops-timeline__head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.ops-timeline__label {
  font-size: 12px;
  font-weight: 700;
  color: var(--ds-ink-soft);
}

.ops-timeline__time {
  font-size: 10px;
  color: var(--ds-faint);
}

.ops-timeline__line {
  margin: 2px 0 0;
  font-size: 12px;
  color: var(--ds-muted);
}
</style>
