<script setup>
import { onMounted, reactive, shallowRef } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { formatCredits, formatTimestamp, listUsageLogs } from '@/api/aiGateway'

const loading = shallowRef(false)
const logs = shallowRef([])
const limit = shallowRef(100)
const filters = reactive({
  tenant_id: '',
  user_id: '',
  model_code: '',
  request_status: ''
})

const statusTagType = (status) => {
  const map = { success: 'success', failed: 'danger', rejected: 'warning' }
  return map[status] || 'info'
}

const fetchLogs = async () => {
  loading.value = true
  try {
    logs.value = await listUsageLogs({
      limit: limit.value,
      tenant_id: filters.tenant_id || undefined,
      user_id: filters.user_id || undefined,
      model_code: filters.model_code || undefined,
      request_status: filters.request_status || undefined
    })
  } finally {
    loading.value = false
  }
}

onMounted(fetchLogs)
</script>

<template>
  <section class="panel">
    <div class="section-head">
      <div>
        <h3>调用日志</h3>
        <p>按创建时间倒序，最多读取 500 条</p>
      </div>
      <div class="toolbar">
        <el-input v-model="filters.tenant_id" clearable placeholder="租户" />
        <el-input v-model="filters.user_id" clearable placeholder="用户" />
        <el-input v-model="filters.model_code" clearable placeholder="模型" />
        <el-select v-model="filters.request_status" clearable placeholder="状态">
          <el-option label="success" value="success" />
          <el-option label="failed" value="failed" />
          <el-option label="rejected" value="rejected" />
        </el-select>
        <el-input-number v-model="limit" :min="1" :max="500" :step="50" />
        <el-button type="primary" :icon="Refresh" :loading="loading" @click="fetchLogs">刷新</el-button>
      </div>
    </div>

    <el-table v-loading="loading" :data="logs" border stripe class="w-full">
      <el-table-column prop="created_at" label="时间" width="170">
        <template #default="{ row }">{{ formatTimestamp(row.created_at) }}</template>
      </el-table-column>
      <el-table-column prop="tenant_id" label="租户" min-width="120" show-overflow-tooltip />
      <el-table-column prop="user_id" label="用户" min-width="120" show-overflow-tooltip />
      <el-table-column prop="model_code" label="模型" min-width="150" />
      <el-table-column prop="provider_code" label="厂商" width="120" />
      <el-table-column prop="upstream_model" label="上游模型" min-width="150" show-overflow-tooltip />
      <el-table-column prop="capability_type" label="能力" width="90" />
      <el-table-column prop="total_tokens" label="Tokens" width="100" align="right" />
      <el-table-column label="计费量" width="130" align="right">
        <template #default="{ row }">
          {{ row.billable_units }} {{ row.billable_unit_type }}
        </template>
      </el-table-column>
      <el-table-column label="Usage" width="110">
        <template #default="{ row }">
          <el-tag :type="row.usage_estimated ? 'warning' : 'success'" size="small">
            {{ row.usage_estimated ? '估算' : row.usage_source }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="平台成本(积分)" width="130" align="right">
        <template #default="{ row }">{{ formatCredits(row.platform_cost) }}</template>
      </el-table-column>
      <el-table-column label="用户计费(积分)" width="130" align="right">
        <template #default="{ row }">{{ formatCredits(row.user_cost) }}</template>
      </el-table-column>
      <el-table-column prop="latency_ms" label="耗时(ms)" width="100" align="right" />
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <el-tag :type="statusTagType(row.request_status)" size="small">{{ row.request_status }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="error_message" label="错误" min-width="180" show-overflow-tooltip />
    </el-table>
  </section>
</template>

<style scoped>
.panel {
  border: 1px solid #f1f5f9;
  border-radius: 14px;
  padding: 16px;
  min-width: 0;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.section-head h3 {
  margin: 0;
  color: #0f172a;
  font-size: 16px;
  font-weight: 800;
}

.section-head p {
  margin: 4px 0 0;
  color: #64748b;
  font-size: 12px;
}

.toolbar {
  display: grid;
  grid-template-columns: repeat(4, minmax(120px, 1fr)) 120px auto;
  gap: 8px;
  align-items: center;
}

@media (max-width: 1180px) {
  .section-head {
    align-items: stretch;
    flex-direction: column;
  }

  .toolbar {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
