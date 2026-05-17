<script setup>
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import {
  formatCredits,
  formatTimestamp,
  getDashboardSummary,
  getOAuthPoolHealth,
  listDashboardRecentErrors,
  listDashboardTopModels,
  listDashboardTopTenants
} from '@/api/aiGateway'

const DAY_OPTIONS = [
  { label: '近24小时', value: 1 },
  { label: '近7天', value: 7 },
  { label: '近30天', value: 30 },
  { label: '近90天', value: 90 },
  { label: '全部', value: 0 }
]

const selectedDays = shallowRef(1)
const loading = shallowRef(false)
const topModels = shallowRef([])
const topTenants = shallowRef([])
const recentErrors = shallowRef([])
const oauthPoolHealth = shallowRef([])

const summary = reactive({
  total_requests: 0,
  successful_requests: 0,
  failed_requests: 0,
  total_tokens: 0,
  total_prompt_tokens: 0,
  total_completion_tokens: 0,
  total_provider_cost: 0,
  total_platform_cost: 0,
  total_user_cost: 0,
  avg_latency_ms: 0
})

const periodLabel = computed(() => DAY_OPTIONS.find((item) => item.value === selectedDays.value)?.label || '近24小时')

const successRate = computed(() => {
  const requests = Number(summary.total_requests) || 0
  if (requests === 0) return '0%'
  return `${((Number(summary.successful_requests) || 0) * 100 / requests).toFixed(1)}%`
})

const statusType = (status) => {
  const map = { success: 'success', failed: 'danger', rejected: 'warning', partial: 'warning' }
  return map[status] || 'info'
}

const fetchAll = async () => {
  loading.value = true
  try {
    const params = { days: selectedDays.value }
    const [nextSummary, nextModels, nextTenants, nextErrors, nextPoolHealth] = await Promise.all([
      getDashboardSummary(params),
      listDashboardTopModels({ ...params, limit: 8 }),
      listDashboardTopTenants({ ...params, limit: 8 }),
      listDashboardRecentErrors({ ...params, limit: 8 }),
      getOAuthPoolHealth()
    ])
    Object.assign(summary, nextSummary || {})
    topModels.value = nextModels || []
    topTenants.value = nextTenants || []
    recentErrors.value = nextErrors || []
    oauthPoolHealth.value = nextPoolHealth || []
  } finally {
    loading.value = false
  }
}

const handleDaysChange = (days) => {
  selectedDays.value = days
  fetchAll()
}

onMounted(fetchAll)
</script>

<template>
  <div class="page-container space-y-5 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <section class="page-head">
      <div>
        <p class="eyebrow">Uni AI API</p>
        <h1>AI Gateway 业务概览</h1>
        <p class="subtitle">基于调用日志统计请求、积分消耗、模型热度和上游异常。</p>
      </div>
      <div class="head-actions">
        <div class="segmented">
          <button
            v-for="option in DAY_OPTIONS"
            :key="option.value"
            class="segment-button"
            :class="{ active: selectedDays === option.value }"
            @click="handleDaysChange(option.value)"
          >
            {{ option.label }}
          </button>
        </div>
        <el-button type="primary" :icon="Refresh" :loading="loading" @click="fetchAll">刷新</el-button>
      </div>
    </section>

    <section v-loading="loading" class="metric-grid">
      <div class="metric">
        <span>请求数</span>
        <strong>{{ formatCredits(summary.total_requests) }}</strong>
        <p>{{ periodLabel }}总调用</p>
      </div>
      <div class="metric">
        <span>成功率</span>
        <strong>{{ successRate }}</strong>
        <p>{{ formatCredits(summary.successful_requests) }} 次成功</p>
      </div>
      <div class="metric">
        <span>用户计费</span>
        <strong>{{ formatCredits(summary.total_user_cost) }}</strong>
        <p>按业务扣费口径统计</p>
      </div>
      <div class="metric">
        <span>Token</span>
        <strong>{{ formatCredits(summary.total_tokens) }}</strong>
        <p>Chat / Responses / Embedding</p>
      </div>
      <div class="metric">
        <span>Prompt Token</span>
        <strong>{{ formatCredits(summary.total_prompt_tokens) }}</strong>
        <p>输入侧 token 总量</p>
      </div>
      <div class="metric">
        <span>Completion Token</span>
        <strong>{{ formatCredits(summary.total_completion_tokens) }}</strong>
        <p>输出侧 token 总量</p>
      </div>
      <div class="metric">
        <span>平台成本</span>
        <strong>{{ formatCredits(summary.total_platform_cost) }}</strong>
        <p>平台承担成本</p>
      </div>
      <div class="metric">
        <span>异常数</span>
        <strong>{{ formatCredits(summary.failed_requests) }}</strong>
        <p>平均 {{ formatCredits(summary.avg_latency_ms) }} ms</p>
      </div>
    </section>

    <section class="grid grid-cols-1 xl:grid-cols-2 gap-5">
      <div class="panel">
        <div class="section-head">
          <div>
            <h2>Top 模型</h2>
            <p>按积分消耗和请求量排序</p>
          </div>
        </div>
        <el-table v-loading="loading" :data="topModels" border stripe class="w-full">
          <el-table-column prop="model_code" label="模型" min-width="150" show-overflow-tooltip />
          <el-table-column label="请求数" width="100" align="right">
            <template #default="{ row }">{{ formatCredits(row.request_count) }}</template>
          </el-table-column>
          <el-table-column label="业务计费" width="110" align="right">
            <template #default="{ row }">{{ formatCredits(row.total_cost) }}</template>
          </el-table-column>
          <el-table-column label="Token" width="110" align="right">
            <template #default="{ row }">{{ formatCredits(row.total_tokens) }}</template>
          </el-table-column>
        </el-table>
      </div>

      <div class="panel">
        <div class="section-head">
          <div>
            <h2>Top 租户</h2>
            <p>按积分消耗和请求量排序</p>
          </div>
        </div>
        <el-table v-loading="loading" :data="topTenants" border stripe class="w-full">
          <el-table-column prop="tenant_id" label="租户" min-width="170" show-overflow-tooltip />
          <el-table-column label="请求数" width="100" align="right">
            <template #default="{ row }">{{ formatCredits(row.request_count) }}</template>
          </el-table-column>
          <el-table-column label="Token" width="110" align="right">
            <template #default="{ row }">{{ formatCredits(row.total_tokens) }}</template>
          </el-table-column>
          <el-table-column label="业务计费" width="120" align="right">
            <template #default="{ row }">{{ formatCredits(row.total_cost) }}</template>
          </el-table-column>
        </el-table>
      </div>
    </section>

    <section class="panel">
      <div class="section-head">
        <div>
          <h2>最近错误</h2>
          <p>用于快速定位上游失败、鉴权拒绝和计费异常。</p>
        </div>
      </div>
      <el-table v-loading="loading" :data="recentErrors" border stripe class="w-full">
        <el-table-column prop="created_at" label="时间" width="170">
          <template #default="{ row }">{{ formatTimestamp(row.created_at) }}</template>
        </el-table-column>
        <el-table-column prop="model_code" label="模型" min-width="150" show-overflow-tooltip />
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag :type="statusType(row.request_status)" size="small">{{ row.request_status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="http_status" label="HTTP" width="90" align="right" />
        <el-table-column prop="request_id" label="请求 ID" min-width="180" show-overflow-tooltip />
        <el-table-column prop="error_code" label="错误码" min-width="150" show-overflow-tooltip />
        <el-table-column prop="error_message" label="错误信息" min-width="260" show-overflow-tooltip />
      </el-table>
    </section>

    <section v-if="oauthPoolHealth.length > 0" class="panel">
      <div class="section-head">
        <div>
          <h2>OAuth 凭据池</h2>
          <p>各账号池 OAuth 凭据状态汇总，快速识别失效或即将过期的凭据。</p>
        </div>
      </div>
      <el-table v-loading="loading" :data="oauthPoolHealth" border stripe class="w-full">
        <el-table-column prop="pool_name" label="账号池" min-width="160" show-overflow-tooltip />
        <el-table-column prop="fixed_provider_type" label="类型" width="140">
          <template #default="{ row }">
            <el-tag size="small" effect="plain">{{ row.fixed_provider_type || '自定义' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="oauth_strategy" label="调度策略" width="110" align="center">
          <template #default="{ row }">
            <span class="strategy-badge" :class="row.oauth_strategy">{{ row.oauth_strategy }}</span>
          </template>
        </el-table-column>
        <el-table-column label="总数" prop="total" width="70" align="right" />
        <el-table-column label="正常" width="80" align="right">
          <template #default="{ row }">
            <span class="count-active">{{ row.active }}</span>
          </template>
        </el-table-column>
        <el-table-column label="失效" width="80" align="right">
          <template #default="{ row }">
            <span :class="row.invalid > 0 ? 'count-invalid' : 'count-zero'">{{ row.invalid }}</span>
          </template>
        </el-table-column>
        <el-table-column label="禁用" width="80" align="right">
          <template #default="{ row }">
            <span :class="row.disabled > 0 ? 'count-warn' : 'count-zero'">{{ row.disabled }}</span>
          </template>
        </el-table-column>
        <el-table-column label="即将过期" width="100" align="right">
          <template #default="{ row }">
            <span :class="row.expiring_soon > 0 ? 'count-warn' : 'count-zero'">{{ row.expiring_soon }}</span>
          </template>
        </el-table-column>
        <el-table-column label="健康度" width="120" align="center">
          <template #default="{ row }">
            <el-progress
              :percentage="row.total > 0 ? Math.round(row.active * 100 / row.total) : 0"
              :status="row.active === 0 ? 'exception' : row.invalid > 0 ? 'warning' : 'success'"
              :stroke-width="8"
              style="width: 90px; display: inline-block"
            />
          </template>
        </el-table-column>
      </el-table>
    </section>
  </div>
</template>

<style scoped>
.page-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border: 1px solid #f1f5f9;
  border-radius: 16px;
  background: #ffffff;
  padding: 24px;
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.04);
}

.eyebrow {
  margin: 0 0 6px;
  color: #64748b;
  font-size: 12px;
  font-weight: 900;
  text-transform: uppercase;
}

.page-head h1 {
  margin: 0;
  color: #0f172a;
  font-size: 24px;
  font-weight: 900;
}

.subtitle {
  margin: 8px 0 0;
  color: #64748b;
  font-size: 14px;
}

.head-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.segmented {
  display: flex;
  gap: 4px;
  border-radius: 10px;
  background: #f8fafc;
  padding: 4px;
}

.segment-button {
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: #64748b;
  cursor: pointer;
  font-size: 12px;
  font-weight: 800;
  padding: 8px 10px;
}

.segment-button.active {
  background: #ffffff;
  color: #4f46e5;
  box-shadow: 0 1px 6px rgba(15, 23, 42, 0.08);
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.metric,
.panel {
  border: 1px solid #f1f5f9;
  border-radius: 12px;
  background: #ffffff;
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.04);
}

.metric {
  min-width: 0;
  padding: 18px;
}

.metric span {
  display: block;
  color: #64748b;
  font-size: 12px;
  font-weight: 900;
}

.metric strong {
  display: block;
  margin-top: 8px;
  color: #0f172a;
  font-size: 26px;
  font-weight: 900;
  line-height: 1.1;
}

.metric p {
  margin: 8px 0 0;
  color: #94a3b8;
  font-size: 12px;
  font-weight: 700;
}

.panel {
  min-width: 0;
  padding: 18px;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
}

.section-head h2 {
  margin: 0;
  color: #0f172a;
  font-size: 16px;
  font-weight: 900;
}

.section-head p {
  margin: 4px 0 0;
  color: #64748b;
  font-size: 12px;
}

@media (max-width: 1180px) {
  .page-head {
    align-items: stretch;
    flex-direction: column;
  }

  .head-actions {
    align-items: stretch;
    flex-direction: column;
  }

  .segmented {
    overflow-x: auto;
  }

  .metric-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .metric-grid {
    grid-template-columns: 1fr;
  }
}

.count-active { color: #16a34a; font-weight: 700; }
.count-invalid { color: #dc2626; font-weight: 700; }
.count-warn { color: #d97706; font-weight: 700; }
.count-zero { color: #94a3b8; }

.strategy-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 6px;
  font-size: 11px;
  font-weight: 700;
  background: #f1f5f9;
  color: #475569;
}
.strategy-badge.weighted {
  background: #ede9fe;
  color: #6d28d9;
}
</style>
