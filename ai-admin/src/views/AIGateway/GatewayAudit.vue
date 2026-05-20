<script setup>
import { onMounted, reactive, shallowRef } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { formatTimestamp, listGatewayAuditLogs } from '@/api/aiGateway'

const loading = shallowRef(false)
const logs = shallowRef([])
const limit = shallowRef(100)
const filters = reactive({
  actor: '',
  object_type: '',
  object_id: '',
  result: ''
})

const fetchLogs = async () => {
  loading.value = true
  try {
    logs.value = await listGatewayAuditLogs({
      limit: limit.value,
      actor: filters.actor || undefined,
      object_type: filters.object_type || undefined,
      object_id: filters.object_id || undefined,
      result: filters.result || undefined
    })
  } finally {
    loading.value = false
  }
}

const resultTagType = (result) => {
  const map = { success: 'success', failed: 'danger' }
  return map[result] || 'info'
}

const formatSummary = (value) => {
  if (!value) return ''
  if (typeof value === 'string') return value
  return JSON.stringify(value)
}

onMounted(fetchLogs)
</script>

<template>
  <section class="panel">
    <div class="header">
      <h3>网关审计</h3>
      <p>记录 AI Gateway 管理侧写操作</p>
    </div>

    <div class="toolbar">
      <el-input v-model="filters.actor" clearable placeholder="操作者" />
      <el-input v-model="filters.object_type" clearable placeholder="对象类型" />
      <el-input v-model="filters.object_id" clearable placeholder="对象 ID" />
      <el-select v-model="filters.result" clearable placeholder="结果">
        <el-option label="success" value="success" />
        <el-option label="failed" value="failed" />
      </el-select>
      <el-input-number v-model="limit" :min="1" :max="500" :step="50" class="limit-input" controls-position="right" />
      <el-button type="primary" :icon="Refresh" :loading="loading" @click="fetchLogs">刷新</el-button>
    </div>

    <el-table v-loading="loading" :data="logs" border stripe class="w-full">
      <el-table-column prop="created_at" label="时间" width="170">
        <template #default="{ row }">{{ formatTimestamp(row.created_at) }}</template>
      </el-table-column>
      <el-table-column prop="actor" label="操作者" min-width="140" show-overflow-tooltip />
      <el-table-column prop="action" label="动作" min-width="220" show-overflow-tooltip />
      <el-table-column prop="object_type" label="对象类型" width="150" />
      <el-table-column prop="object_id" label="对象 ID" min-width="180" show-overflow-tooltip />
      <el-table-column prop="http_status" label="HTTP" width="90" align="right" />
      <el-table-column prop="result" label="结果" width="100">
        <template #default="{ row }">
          <el-tag :type="resultTagType(row.result)" size="small">{{ row.result }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="摘要" min-width="260" show-overflow-tooltip>
        <template #default="{ row }">{{ formatSummary(row.request_summary) }}</template>
      </el-table-column>
    </el-table>
  </section>
</template>

<style scoped>
.panel {
  min-width: 0;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #ffffff;
  padding: 16px;
}

.header {
  margin-bottom: 12px;
}

.header h3 {
  margin: 0;
  color: #0f172a;
  font-size: 16px;
  font-weight: 900;
}

.header p {
  margin: 4px 0 0;
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}

.toolbar {
  display: grid;
  grid-template-columns: repeat(4, minmax(120px, 1fr)) 100px auto;
  gap: 8px;
  align-items: center;
  margin-bottom: 14px;
}

.limit-input {
  width: 100px;
}

@media (max-width: 1180px) {
  .toolbar {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
