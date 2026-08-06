<!--
  网关审计。
  适配：listGatewayAuditLogs → aiAdminApi.listGatewayAuditLogs；v4 返回 {items,total}，取 .items；formatTimestamp 内联。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       筛选/表格同卡）,el-table → DsTable,el-tag → DsTag。
       ⚠ 该页 API 仅支持 limit 参数、无服务端分页,故不加 DsPagination。
  ⚠ 后端缺口：v4 /api/v1/audit-logs 仅支持 limit 查询参数，不支持 actor/object_type/object_id/result 过滤。
    按"不删列"原则保留 V1 的 4 个筛选输入框（仍可输入），但只有 limit 会传给后端；其余筛选条件后端忽略。
-->
<script setup lang="ts">
import { onMounted, reactive, shallowRef } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { FileText } from 'lucide-vue-next'
import { PortalPagePanel } from '@/platform'
import { DsFilterBar, DsFilterField, DsTable, DsTag, type DsTableColumn } from '@/shared/ui'
import { aiAdminApi } from '@/api/aiAdmin'

const formatTimestamp = (value: any) => {
  if (!value) return ''
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

const loading = shallowRef(false)
const logs = shallowRef<any[]>([])
const limit = shallowRef(100)
const filters = reactive({
  actor: '',
  object_type: '',
  object_id: '',
  result: ''
})

const columns: DsTableColumn[] = [
  { key: 'created_at', title: '时间', width: 170 },
  { key: 'actor', title: '操作者' },
  { key: 'action', title: '动作' },
  { key: 'object_type', title: '对象类型', width: 150 },
  { key: 'object_id', title: '对象 ID' },
  { key: 'http_status', title: 'HTTP', width: 90, align: 'right' },
  { key: 'result', title: '结果', width: 100 },
  { key: 'request_summary', title: '摘要' }
]

const fetchLogs = async () => {
  loading.value = true
  try {
    // v4 仅接受 limit；其余 filters 字段后端忽略（保留 UI 输入以贴合 V1）
    const res: any = await aiAdminApi.listGatewayAuditLogs({
      limit: limit.value,
      actor: filters.actor || undefined,
      object_type: filters.object_type || undefined,
      object_id: filters.object_id || undefined,
      result: filters.result || undefined
    })
    logs.value = res?.items || []
  } finally {
    loading.value = false
  }
}

const resultTone = (result: any): 'positive' | 'danger' | 'info' => {
  const map: Record<string, 'positive' | 'danger'> = { success: 'positive', failed: 'danger' }
  return map[result] || 'info'
}

const formatSummary = (value: any) => {
  if (!value) return ''
  if (typeof value === 'string') return value
  return JSON.stringify(value)
}

onMounted(fetchLogs)
</script>

<template>
  <div class="audit-page">
    <PortalPagePanel
      :icon="FileText"
      :breadcrumbs="[{ label: '智能服务' }, { label: '日志审计' }, { label: '网关审计' }]"
      description="记录 AI Gateway 管理侧写操作。"
      fill
    >
      <template #actions>
        <el-button type="primary" :icon="Refresh" :loading="loading" @click="fetchLogs">刷新</el-button>
      </template>

      <template #filters>
        <DsFilterBar>
          <DsFilterField label="操作者">
            <el-input v-model="filters.actor" clearable placeholder="操作者" class="audit-filter-input" />
          </DsFilterField>
          <DsFilterField label="对象类型">
            <el-input v-model="filters.object_type" clearable placeholder="对象类型" class="audit-filter-input" />
          </DsFilterField>
          <DsFilterField label="对象 ID">
            <el-input v-model="filters.object_id" clearable placeholder="对象 ID" class="audit-filter-input" />
          </DsFilterField>
          <DsFilterField label="结果">
            <el-select v-model="filters.result" clearable placeholder="结果" class="audit-filter-input">
              <el-option label="success" value="success" />
              <el-option label="failed" value="failed" />
            </el-select>
          </DsFilterField>
          <DsFilterField label="条数上限">
            <el-input-number v-model="limit" :min="1" :max="500" :step="50" class="audit-limit-input" :controls="false" />
          </DsFilterField>
        </DsFilterBar>
      </template>

      <DsTable
        :frame="false"
        :columns="columns"
        :rows="logs"
        row-key="id"
        :loading="loading"
        empty-title="暂无审计日志"
        class="audit-table"
      >
        <template #cell-created_at="{ row }">{{ formatTimestamp(row.created_at) }}</template>
        <template #cell-result="{ row }">
          <DsTag :tone="resultTone(row.result)">{{ row.result }}</DsTag>
        </template>
        <template #cell-request_summary="{ row }">{{ formatSummary(row.request_summary) }}</template>
      </DsTable>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.audit-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* fill 模式下表格区伸展撑满面板 body */
.audit-table {
  flex: 1;
  min-height: 0;
}

.audit-filter-input {
  width: min(160px, 100%);
}

.audit-limit-input {
  width: min(120px, 100%);
}
</style>
