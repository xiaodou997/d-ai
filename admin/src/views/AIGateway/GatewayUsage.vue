<script setup>
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import {
  formatCredits,
  formatTimestamp,
  listUsageLogs,
  listUsageSummary,
  listUsageUnitSummary
} from '@/api/aiGateway'

const authStore = useAuthStore()
const loading = shallowRef(false)
const summaryLoading = shallowRef(false)
const logs = shallowRef([])
const summaryRows = shallowRef([])
const unitRows = shallowRef([])
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

const unitLabel = (unit) => {
  const map = {
    token: 'Token',
    input_token: '输入 Token',
    output_token: '输出 Token',
    image: '图片',
    second: '秒',
    request: '请求'
  }
  return map[unit] || unit || '-'
}

const billableFieldLabel = (unit) => {
  const map = {
    token: 'token_count',
    input_token: 'input_token_count',
    output_token: 'output_token_count',
    image: 'image_count',
    second: 'second_count',
    request: 'request_count'
  }
  return map[unit] || 'billable_units'
}

const capabilityLabel = (capability) => {
  const map = {
    chat: '文本',
    embedding: 'Embedding',
    image: '图片',
    video: '视频',
    audio: '音频',
    rerank: '重排'
  }
  return map[capability] || capability || '-'
}

const usageParams = computed(() => ({
  tenant_id: authStore.isPlatformAdmin ? filters.tenant_id || undefined : authStore.tenantId || undefined,
  user_id: authStore.isEndUser ? authStore.userId || undefined : filters.user_id || undefined,
  model_code: filters.model_code || undefined,
  request_status: filters.request_status || undefined
}))

const summaryTotals = computed(() => summaryRows.value.reduce((acc, row) => {
  acc.requestCount += Number(row.request_count) || 0
  acc.promptTokens += Number(row.total_prompt_tokens) || 0
  acc.completionTokens += Number(row.total_completion_tokens) || 0
  acc.totalTokens += Number(row.total_tokens) || 0
  acc.providerCost += Number(row.total_provider_cost) || 0
  acc.platformCost += Number(row.total_platform_cost) || 0
  acc.userCost += Number(row.total_user_cost) || 0
  acc.quotaCost += Number(row.total_quota_cost) || 0
  return acc
}, {
  requestCount: 0,
  promptTokens: 0,
  completionTokens: 0,
  totalTokens: 0,
  providerCost: 0,
  platformCost: 0,
  userCost: 0,
  quotaCost: 0
}))

const unitCostShare = (row) => {
  const total = summaryTotals.value.userCost
  if (!total) return '0%'
  return `${((Number(row.total_user_cost) || 0) * 100 / total).toFixed(1)}%`
}

const fetchUsage = async () => {
  loading.value = true
  summaryLoading.value = true
  try {
    const params = usageParams.value
    const [nextUnitSummary, nextSummary, nextLogs] = await Promise.all([
      listUsageUnitSummary(params),
      listUsageSummary(params),
      listUsageLogs({ ...params, limit: limit.value })
    ])
    unitRows.value = nextUnitSummary
    summaryRows.value = nextSummary
    logs.value = nextLogs
  } finally {
    loading.value = false
    summaryLoading.value = false
  }
}

onMounted(fetchUsage)
</script>

<template>
  <section class="panel">
    <div class="header">
      <h3>调用日志</h3>
      <p>按创建时间倒序，最多读取 500 条</p>
    </div>

    <div class="toolbar">
      <el-input v-if="authStore.isPlatformAdmin" v-model="filters.tenant_id" clearable placeholder="租户" />
      <el-input v-if="!authStore.isEndUser" v-model="filters.user_id" clearable placeholder="用户" />
      <el-input v-model="filters.model_code" clearable placeholder="模型" />
      <el-select v-model="filters.request_status" clearable placeholder="状态">
        <el-option label="success" value="success" />
        <el-option label="failed" value="failed" />
        <el-option label="rejected" value="rejected" />
      </el-select>
      <el-input-number v-model="limit" :min="1" :max="500" :step="50" class="limit-input" controls-position="right" />
      <el-button type="primary" :icon="Refresh" :loading="loading || summaryLoading" @click="fetchUsage">刷新</el-button>
    </div>

    <div v-loading="summaryLoading" class="summary-grid">
      <div class="metric-item">
        <span>请求数</span>
        <strong>{{ formatCredits(summaryTotals.requestCount) }}</strong>
      </div>
      <div class="metric-item">
        <span>Prompt Token</span>
        <strong>{{ formatCredits(summaryTotals.promptTokens) }}</strong>
      </div>
      <div class="metric-item">
        <span>Completion Token</span>
        <strong>{{ formatCredits(summaryTotals.completionTokens) }}</strong>
      </div>
      <div class="metric-item">
        <span>Total Token</span>
        <strong>{{ formatCredits(summaryTotals.totalTokens) }}</strong>
      </div>
      <div class="metric-item">
        <span>供应商成本</span>
        <strong>{{ formatCredits(summaryTotals.providerCost) }}</strong>
      </div>
      <div class="metric-item">
        <span>平台成本</span>
        <strong>{{ formatCredits(summaryTotals.platformCost) }}</strong>
      </div>
      <div class="metric-item">
        <span>Key 额度消耗</span>
        <strong>{{ formatCredits(summaryTotals.quotaCost) }}</strong>
      </div>
    </div>

    <el-table v-loading="summaryLoading" :data="unitRows" border stripe class="w-full unit-table">
      <el-table-column label="计费单位" min-width="130">
        <template #default="{ row }">
          <div class="unit-cell">
            <span>{{ unitLabel(row.billable_unit_type) }}</span>
            <small>{{ billableFieldLabel(row.billable_unit_type) }}</small>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="请求数" width="100" align="right">
        <template #default="{ row }">{{ formatCredits(row.request_count) }}</template>
      </el-table-column>
      <el-table-column label="计费量" width="120" align="right">
        <template #default="{ row }">{{ formatCredits(row.total_billable_units) }}</template>
      </el-table-column>
      <el-table-column label="用户计费" width="120" align="right">
        <template #default="{ row }">{{ formatCredits(row.total_user_cost) }}</template>
      </el-table-column>
      <el-table-column label="用户计费占比" width="120" align="right">
        <template #default="{ row }">{{ unitCostShare(row) }}</template>
      </el-table-column>
      <el-table-column label="平台成本" width="110" align="right">
        <template #default="{ row }">{{ formatCredits(row.total_platform_cost) }}</template>
      </el-table-column>
      <el-table-column label="供应商成本" width="120" align="right">
        <template #default="{ row }">{{ formatCredits(row.total_provider_cost) }}</template>
      </el-table-column>
    </el-table>

    <el-table v-loading="summaryLoading" :data="summaryRows" border stripe class="w-full summary-table">
      <el-table-column prop="model_code" label="模型" min-width="150" show-overflow-tooltip />
      <el-table-column label="请求数" width="100" align="right">
        <template #default="{ row }">{{ formatCredits(row.request_count) }}</template>
      </el-table-column>
      <el-table-column label="Prompt" width="110" align="right">
        <template #default="{ row }">{{ formatCredits(row.total_prompt_tokens) }}</template>
      </el-table-column>
      <el-table-column label="Completion" width="120" align="right">
        <template #default="{ row }">{{ formatCredits(row.total_completion_tokens) }}</template>
      </el-table-column>
      <el-table-column label="Token" width="110" align="right">
        <template #default="{ row }">{{ formatCredits(row.total_tokens) }}</template>
      </el-table-column>
      <el-table-column label="Key 额度消耗" width="130" align="right">
        <template #default="{ row }">{{ formatCredits(row.total_quota_cost) }}</template>
      </el-table-column>
      <el-table-column label="平台成本(积分)" width="140" align="right">
        <template #default="{ row }">{{ formatCredits(row.total_platform_cost) }}</template>
      </el-table-column>
      <el-table-column label="用户计费(积分)" width="140" align="right">
        <template #default="{ row }">{{ formatCredits(row.total_user_cost) }}</template>
      </el-table-column>
      <el-table-column label="供应商成本(积分)" width="150" align="right">
        <template #default="{ row }">{{ formatCredits(row.total_provider_cost) }}</template>
      </el-table-column>
    </el-table>

    <el-table v-loading="loading" :data="logs" border stripe class="w-full">
      <el-table-column prop="created_at" label="时间" width="170">
        <template #default="{ row }">{{ formatTimestamp(row.created_at) }}</template>
      </el-table-column>
      <el-table-column prop="tenant_id" label="租户" min-width="120" show-overflow-tooltip />
      <el-table-column prop="user_id" label="用户" min-width="120" show-overflow-tooltip />
      <el-table-column prop="model_code" label="模型" min-width="150" />
      <el-table-column prop="provider_code" label="厂商" width="120" />
      <el-table-column prop="upstream_model" label="上游模型" min-width="150" show-overflow-tooltip />
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
      <el-table-column label="Key 额度消耗" width="130" align="right">
        <template #default="{ row }">{{ formatCredits(row.api_key_quota_cost) }}</template>
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

.summary-grid {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 10px;
  margin-bottom: 12px;
}

.metric-item {
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 12px;
  min-width: 0;
  background: #f8fafc;
}

.metric-item span {
  display: block;
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}

.metric-item strong {
  display: block;
  margin-top: 6px;
  color: #0f172a;
  font-size: 20px;
  line-height: 1.1;
}

.unit-table,
.summary-table {
  margin-bottom: 14px;
}

.unit-cell {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
}

.unit-cell span {
  color: #0f172a;
  font-weight: 800;
}

.unit-cell small {
  color: #94a3b8;
  font-size: 11px;
}

@media (max-width: 1180px) {
  .toolbar {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
