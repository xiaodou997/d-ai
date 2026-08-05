<!--
  风险事件列表 — 风控中心「风险事件」Tab 内容。
  重构：el-table 迁移至 DsTable(:frame="false"),筛选改为 DsFilterBar/DsFilterField,
       级别 el-tag 换成 DsTag(tone 映射不变);接口仅支持 limit 不支持分页,
       故不接 DsPagination,保留「共 N 条」计数;处置弹窗仍为 element-plus,业务逻辑与请求参数不变。
-->
<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { DsFilterBar, DsFilterField, DsTable, DsTag, type DsTableColumn } from '@/shared/ui'

import { useRiskControlEvents } from '../composables/useRiskControlEvents'

const emit = defineEmits<{
  'open-event-count': [count: number]
}>()

const eventStatusOptions = [
  { label: '待处理', value: 'open' },
  { label: '已确认', value: 'acknowledged' },
  { label: '已处理', value: 'resolved' },
  { label: '已忽略', value: 'dismissed' }
]

const {
  eventStatusFilter,
  events,
  eventsLoading,
  eventsTotal,
  fetchEvents,
  openEventCount,
  openResolveDialog,
  resolveDialogVisible,
  resolveForm,
  resolveTarget,
  resolving,
  submitResolve
} = useRiskControlEvents()

const columns: DsTableColumn[] = [
  { key: 'created_at', title: '时间', width: 170 },
  { key: 'severity', title: '级别', width: 90 },
  { key: 'tenant_id', title: '租户', mono: true },
  { key: 'user_id', title: '用户', mono: true },
  { key: 'summary', title: '摘要' },
  { key: 'status', title: '状态', width: 100 },
  { key: 'actions', title: '操作', width: 100 }
]

function formatTimestamp(value: unknown) {
  if (!value) return ''
  return new Date(value as string | number).toLocaleString('zh-CN', { hour12: false })
}

function severityTone(severity: string): 'info' | 'warning' | 'danger' {
  const map: Record<string, 'info' | 'warning' | 'danger'> = { low: 'info', medium: 'warning', high: 'danger' }
  return map[severity] || 'info'
}

function eventStatusLabel(status: string) {
  return eventStatusOptions.find((item) => item.value === status)?.label || status
}

watch(openEventCount, (count) => emit('open-event-count', count), { immediate: true })
onMounted(fetchEvents)
</script>

<template>
  <!-- 单根节点:父组件对本面板使用 v-show,多根会导致指令失效 -->
  <div class="risk-events-panel">
    <DsFilterBar class="risk-events-filters">
      <DsFilterField label="状态">
        <el-select v-model="eventStatusFilter" clearable placeholder="状态" class="filter-input" @change="fetchEvents">
          <el-option v-for="item in eventStatusOptions" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
      </DsFilterField>
      <template #actions>
        <span class="result-count">共 {{ eventsTotal }} 条</span>
        <el-button type="primary" :icon="Refresh" :loading="eventsLoading" @click="fetchEvents">刷新</el-button>
      </template>
    </DsFilterBar>

    <DsTable
      :frame="false"
      :columns="columns"
      :rows="events"
      row-key="id"
      :loading="eventsLoading"
      empty-title="暂无风险事件"
    >
      <template #cell-created_at="{ row }">{{ formatTimestamp(row.created_at) }}</template>
      <template #cell-severity="{ row }">
        <DsTag :tone="severityTone(row.severity)">{{ row.severity }}</DsTag>
      </template>
      <template #cell-status="{ row }">{{ eventStatusLabel(row.status) }}</template>
      <template #cell-actions="{ row }">
        <el-button
          size="small"
          link
          type="primary"
          :disabled="row.status !== 'open' && row.status !== 'acknowledged'"
          @click="openResolveDialog(row)"
        >
          处置
        </el-button>
      </template>
    </DsTable>

    <el-dialog v-model="resolveDialogVisible" title="处置风险事件" width="480px" append-to-body>
      <p v-if="resolveTarget" class="resolve-summary">{{ resolveTarget.summary }}</p>
      <el-form :model="resolveForm" label-position="top">
        <el-form-item label="处置结果">
          <el-select v-model="resolveForm.status" class="full-field">
            <el-option label="已确认（继续观察）" value="acknowledged" />
            <el-option label="已处理" value="resolved" />
            <el-option label="忽略（误报）" value="dismissed" />
          </el-select>
        </el-form-item>
        <el-form-item label="处置备注">
          <el-input
            v-model="resolveForm.note"
            type="textarea"
            :rows="3"
            placeholder="可选，记录处置说明；如需封禁账号请前往用户管理单独操作"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="resolveDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="resolving" @click="submitResolve">提交</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.risk-events-filters {
  margin-bottom: 16px;
}

.filter-input {
  width: min(160px, 100%);
}

/* el-select 默认 width:100%(--el-select-width),而 DsFilterField 是 shrink-to-fit 的
   纵向 flex 项,空占位时内容固有宽度极小,select 会被压成几十像素;
   这里固定与输入框一致的宽度(scoped 样式可作用于子组件根节点) */
.filter-input.el-select {
  width: 160px;
}

.result-count {
  font-size: 12.5px;
  color: var(--ds-muted);
  white-space: nowrap;
}

.full-field {
  width: 100%;
}

.resolve-summary {
  margin: 0 0 12px;
  padding: 10px 12px;
  background: var(--ds-panel-muted);
  border-radius: 8px;
  font-size: 13px;
  color: var(--ds-ink);
}
</style>
