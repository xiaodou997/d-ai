<script setup>
import { computed, onMounted, onUnmounted, shallowRef, watch } from 'vue'
import { Refresh, Search, InfoFilled } from '@element-plus/icons-vue'
import { listUsageLogs, formatCredits } from '@/api/aiGateway'
import { getUsers } from '@/api/tenant'

const PAGE_SIZE = 20

const loading = shallowRef(false)
const total = shallowRef(0)
const records = shallowRef([])
const stats = shallowRef({ total_requests: 0, success_count: 0, failed_count: 0, total_tokens: 0, total_cost: 0, avg_latency_ms: 0 })
const users = shallowRef([])
const drawerVisible = shallowRef(false)
const selectedRecord = shallowRef(null)

const defaultDateRange = () => {
  const to = new Date()
  const from = new Date()
  from.setDate(from.getDate() - 6)
  from.setHours(0, 0, 0, 0)
  to.setHours(23, 59, 59, 999)
  return [from, to]
}

const filters = shallowRef({
  dateRange: defaultDateRange(),
  userId: '',
  modelCode: '',
  requestStatus: '',
  requestSource: '',
})

const currentPage = shallowRef(1)

const userMap = computed(() => {
  const m = new Map()
  for (const u of users.value) {
    m.set(String(u.id), u)
  }
  return m
})

const successRate = computed(() => {
  if (!stats.value.total_requests) return '-'
  return ((stats.value.success_count / stats.value.total_requests) * 100).toFixed(1) + '%'
})

const buildParams = () => {
  const f = filters.value
  const params = {
    limit: PAGE_SIZE,
    offset: (currentPage.value - 1) * PAGE_SIZE,
  }
  if (f.userId) params.user_id = f.userId
  if (f.modelCode) params.model_code = f.modelCode
  if (f.requestStatus) params.request_status = f.requestStatus
  if (f.requestSource) params.request_source = f.requestSource
  if (f.dateRange && f.dateRange[0]) params.date_from = new Date(f.dateRange[0]).toISOString()
  if (f.dateRange && f.dateRange[1]) params.date_to = new Date(f.dateRange[1]).toISOString()
  return params
}

const fetchData = async () => {
  loading.value = true
  try {
    const res = await listUsageLogs(buildParams())
    total.value = res.total ?? 0
    stats.value = res.stats ?? stats.value
    records.value = res.records ?? []
  } finally {
    loading.value = false
  }
}

const fetchUsers = async () => {
  const res = await getUsers({ page: 1, size: 200 })
  users.value = res.records || []
}

const handleSearch = () => {
  currentPage.value = 1
  fetchData()
}

const handleReset = () => {
  filters.value = {
    dateRange: defaultDateRange(),
    userId: '',
    modelCode: '',
    requestStatus: '',
    requestSource: '',
  }
  currentPage.value = 1
  fetchData()
}

const handlePageChange = (page) => {
  currentPage.value = page
  fetchData()
}

const openDetail = (row) => {
  selectedRecord.value = row
  drawerVisible.value = true
}

const getUserLabel = (row) => {
  if (row.user_id?.value || row.user_id) {
    const uid = row.user_id?.value || row.user_id
    const u = userMap.value.get(uid)
    if (u) return u.username || u.email || uid
    return uid
  }
  if (row.external_user_id?.value) return row.external_user_id.value
  return '-'
}

const statusTagType = (status) => {
  const map = { success: 'success', failed: 'danger', error: 'danger', pending: 'warning' }
  return map[status] || 'info'
}

const formatMs = (val) => {
  const n = val?.value ?? val
  if (n == null || n === 0) return '-'
  if (n >= 1000) return (n / 1000).toFixed(1) + ' s'
  return n + ' ms'
}

const formatTimestamp = (ts) => {
  const t = ts?.value ?? ts
  if (!t) return '-'
  return new Date(t).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

const formatFullTimestamp = (ts) => {
  const t = ts?.value ?? ts
  if (!t) return '-'
  return new Date(t).toLocaleString('zh-CN')
}

const pgTextVal = (v) => v?.value ?? v ?? '-'

const modelOptions = computed(() => {
  const seen = new Set()
  for (const r of records.value) {
    if (r.model_code) seen.add(r.model_code)
  }
  return Array.from(seen).sort().map(v => ({ label: v, value: v }))
})

const statusOptions = [
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' },
  { label: '错误', value: 'error' },
  { label: '待处理', value: 'pending' },
]

const requestSourceOptions = [
  { label: 'API Key 调用', value: 'api_key' },
  { label: '网页对话', value: 'web_chat' },
]

const requestSourceLabel = (value) =>
  requestSourceOptions.find((item) => item.value === value)?.label || value || '-'

onMounted(() => {
  fetchUsers()
  fetchData()
})

let resizeTimer = null
const handleResize = () => {
  clearTimeout(resizeTimer)
  resizeTimer = setTimeout(() => {}, 100)
}
onMounted(() => window.addEventListener('resize', handleResize))
onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  clearTimeout(resizeTimer)
})
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div class="page-title">
        <p class="eyebrow">Usage Records</p>
        <h1>消耗明细</h1>
        <p>每行对应一次 AI API 调用，支持按时间、用户、模型、状态过滤。</p>
      </div>
      <el-button :icon="Refresh" @click="handleReset" :loading="loading">重置刷新</el-button>
    </header>

    <!-- 统计卡片 -->
    <section class="stats-grid">
      <div class="stat-card">
        <span class="stat-label">总请求数</span>
        <span class="stat-value">{{ stats.total_requests.toLocaleString() }}</span>
        <span class="stat-sub">
          <span class="sub-success">{{ stats.success_count.toLocaleString() }} 成功</span>
          &nbsp;/&nbsp;
          <span class="sub-fail">{{ stats.failed_count.toLocaleString() }} 失败</span>
        </span>
      </div>
      <div class="stat-card">
        <span class="stat-label">成功率</span>
        <span class="stat-value" :class="{ 'value-success': parseFloat(successRate) >= 95, 'value-warn': parseFloat(successRate) < 95 }">
          {{ successRate }}
        </span>
        <span class="stat-sub">当前过滤范围</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">总 Token</span>
        <span class="stat-value">{{ formatCredits(stats.total_tokens) }}</span>
        <span class="stat-sub">当前过滤范围</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">消耗积分</span>
        <span class="stat-value accent">{{ formatCredits(stats.total_cost) }}</span>
        <span class="stat-sub">均延 {{ Math.round(stats.avg_latency_ms) }} ms</span>
      </div>
    </section>

    <!-- 过滤栏 -->
    <section class="filter-bar">
      <el-date-picker
        v-model="filters.dateRange"
        type="datetimerange"
        range-separator="至"
        start-placeholder="开始时间"
        end-placeholder="结束时间"
        format="MM-DD HH:mm"
        value-format="x"
        :shortcuts="[
          { text: '今天', value: () => { const d = new Date(); d.setHours(0,0,0,0); return [d, new Date()] } },
          { text: '近 7 天', value: () => { const to = new Date(); const from = new Date(); from.setDate(from.getDate()-6); from.setHours(0,0,0,0); return [from, to] } },
          { text: '近 30 天', value: () => { const to = new Date(); const from = new Date(); from.setDate(from.getDate()-29); from.setHours(0,0,0,0); return [from, to] } },
        ]"
        style="width: 340px"
      />
      <el-select v-model="filters.userId" placeholder="全部用户" clearable style="width: 150px">
        <el-option v-for="u in users" :key="u.id" :label="u.username || u.email" :value="String(u.id)" />
      </el-select>
      <el-input v-model="filters.modelCode" placeholder="模型（如 gpt-4o）" clearable style="width: 180px" />
      <el-select v-model="filters.requestStatus" placeholder="全部状态" clearable style="width: 120px">
        <el-option v-for="s in statusOptions" :key="s.value" :label="s.label" :value="s.value" />
      </el-select>
      <el-select v-model="filters.requestSource" placeholder="全部来源" clearable style="width: 140px">
        <el-option v-for="s in requestSourceOptions" :key="s.value" :label="s.label" :value="s.value" />
      </el-select>
      <el-button type="primary" :icon="Search" @click="handleSearch" :loading="loading">查询</el-button>
    </section>

    <!-- 明细表格 -->
    <main class="table-panel">
      <el-table :data="records" v-loading="loading" stripe style="width: 100%">
        <el-table-column label="时间" min-width="130">
          <template #default="{ row }">
            <span class="mono text-xs">{{ formatTimestamp(row.created_at) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="用户" min-width="120">
          <template #default="{ row }">
            <span class="text-sm">{{ getUserLabel(row) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="模型" min-width="140">
          <template #default="{ row }">
            <span class="model-chip">{{ row.model_code }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.request_status)" size="small">
              {{ row.request_status }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="来源" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="row.request_source === 'web_chat' ? 'success' : 'info'">
              {{ requestSourceLabel(row.request_source) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Token" min-width="100" align="right">
          <template #default="{ row }">
            <el-tooltip :content="`Prompt: ${row.prompt_tokens}  Completion: ${row.completion_tokens}`" placement="top">
              <span class="mono">{{ row.total_tokens.toLocaleString() }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="积分" min-width="90" align="right">
          <template #default="{ row }">
            <span class="mono">{{ formatCredits(row.user_cost) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="延迟" width="90" align="right">
          <template #default="{ row }">
            <span class="mono text-xs">{{ formatMs(row.latency_ms) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="70" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" :icon="InfoFilled" @click="openDetail(row)" />
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-bar">
        <el-pagination
          :current-page="currentPage"
          :page-size="PAGE_SIZE"
          :total="total"
          layout="total, prev, pager, next"
          @current-change="handlePageChange"
        />
      </div>
    </main>

    <!-- 详情抽屉 -->
    <el-drawer v-model="drawerVisible" title="调用详情" size="480px" append-to-body destroy-on-close>
      <template v-if="selectedRecord">
        <div class="detail-sections">
          <section class="detail-section">
            <h3>基础信息</h3>
            <dl>
              <dt>Request ID</dt><dd class="mono">{{ selectedRecord.request_id }}</dd>
              <dt>Trace ID</dt><dd class="mono">{{ pgTextVal(selectedRecord.trace_id) }}</dd>
              <dt>时间</dt><dd>{{ formatFullTimestamp(selectedRecord.created_at) }}</dd>
              <dt>流式</dt><dd>{{ selectedRecord.stream ? '是' : '否' }}</dd>
              <dt>调用来源</dt><dd>{{ requestSourceLabel(selectedRecord.request_source) }}</dd>
              <dt>认证方式</dt><dd>{{ selectedRecord.auth_method }}</dd>
            </dl>
          </section>

          <section class="detail-section">
            <h3>路由信息</h3>
            <dl>
              <dt>模型</dt><dd class="model-chip">{{ selectedRecord.model_code }}</dd>
              <dt>Provider</dt><dd>{{ pgTextVal(selectedRecord.provider_code) }}</dd>
              <dt>上游模型</dt><dd class="mono">{{ pgTextVal(selectedRecord.upstream_model) }}</dd>
            </dl>
          </section>

          <section class="detail-section">
            <h3>用户</h3>
            <dl>
              <dt>内部 User ID</dt><dd class="mono">{{ pgTextVal(selectedRecord.user_id) }}</dd>
              <dt>外部 User ID</dt><dd class="mono">{{ pgTextVal(selectedRecord.external_user_id) }}</dd>
              <dt>Conversation ID</dt><dd class="mono">{{ pgTextVal(selectedRecord.conversation_id) }}</dd>
            </dl>
          </section>

          <section class="detail-section">
            <h3>状态</h3>
            <dl>
              <dt>请求状态</dt>
              <dd><el-tag :type="statusTagType(selectedRecord.request_status)" size="small">{{ selectedRecord.request_status }}</el-tag></dd>
              <dt>HTTP 状态</dt><dd>{{ selectedRecord.http_status?.value ?? '-' }}</dd>
              <dt>上游状态</dt><dd>{{ selectedRecord.upstream_status?.value ?? '-' }}</dd>
              <dt>错误码</dt><dd class="mono">{{ pgTextVal(selectedRecord.error_code) }}</dd>
              <dt>错误信息</dt><dd class="error-msg">{{ pgTextVal(selectedRecord.error_message) }}</dd>
            </dl>
          </section>

          <section class="detail-section">
            <h3>Token 用量</h3>
            <dl>
              <dt>Prompt</dt><dd class="mono">{{ selectedRecord.prompt_tokens.toLocaleString() }}</dd>
              <dt>Completion</dt><dd class="mono">{{ selectedRecord.completion_tokens.toLocaleString() }}</dd>
              <dt>Total</dt><dd class="mono"><strong>{{ selectedRecord.total_tokens.toLocaleString() }}</strong></dd>
              <dt>计费单位类型</dt><dd>{{ selectedRecord.billable_unit_type }}</dd>
              <dt>计费单位数</dt><dd class="mono">{{ selectedRecord.billable_units.toLocaleString() }}</dd>
              <dt>Token 来源</dt><dd>{{ selectedRecord.token_usage_source }}</dd>
              <dt>估算</dt><dd>{{ selectedRecord.usage_estimated ? '是' : '否' }}</dd>
            </dl>
          </section>

          <section class="detail-section">
            <h3>费用</h3>
            <dl>
              <dt>用户积分</dt><dd class="mono accent"><strong>{{ formatCredits(selectedRecord.user_cost) }}</strong></dd>
              <dt>平台积分</dt><dd class="mono">{{ formatCredits(selectedRecord.platform_cost) }}</dd>
              <dt>上游成本</dt><dd class="mono">{{ formatCredits(selectedRecord.provider_cost) }}</dd>
              <dt>计费状态</dt><dd>{{ selectedRecord.billing_status }}</dd>
              <dt>URM 交易 ID</dt><dd class="mono">{{ pgTextVal(selectedRecord.urm_transaction_id) }}</dd>
            </dl>
          </section>

          <section class="detail-section">
            <h3>性能</h3>
            <dl>
              <dt>总延迟</dt><dd class="mono">{{ formatMs(selectedRecord.latency_ms) }}</dd>
              <dt>首 Token 延迟</dt><dd class="mono">{{ formatMs(selectedRecord.first_token_latency_ms) }}</dd>
            </dl>
          </section>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<style scoped>
.page-container {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 24px;
}

.page-header {
  background: #fff;
  border: 1px solid #f1f5f9;
  border-radius: 16px;
  padding: 24px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  box-shadow: 0 4px 16px rgba(15, 23, 42, 0.04);
}

.page-title { display: flex; flex-direction: column; }
.eyebrow {
  margin: 0 0 4px;
  color: #64748b;
  font-size: 11px;
  font-weight: 900;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.page-title h1 { margin: 0; color: #0f172a; font-size: 22px; font-weight: 900; }
.page-title p  { margin: 4px 0 0; color: #64748b; font-size: 13px; }

/* 统计卡片 */
.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
}
.stat-card {
  background: #fff;
  border: 1px solid #f1f5f9;
  border-radius: 14px;
  padding: 20px 22px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.04);
}
.stat-label {
  font-size: 11px;
  font-weight: 900;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: #94a3b8;
}
.stat-value {
  font-size: 28px;
  font-weight: 900;
  color: #0f172a;
  line-height: 1;
}
.stat-value.value-success { color: #16a34a; }
.stat-value.value-warn   { color: #d97706; }
.stat-value.accent       { color: #6366f1; }
.stat-sub {
  font-size: 12px;
  color: #94a3b8;
}
.sub-success { color: #16a34a; }
.sub-fail    { color: #dc2626; }

/* 过滤栏 */
.filter-bar {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  align-items: center;
  background: #fff;
  border: 1px solid #f1f5f9;
  border-radius: 14px;
  padding: 16px 20px;
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.04);
}

/* 表格面板 */
.table-panel {
  background: #fff;
  border: 1px solid #f1f5f9;
  border-radius: 16px;
  overflow: hidden;
  box-shadow: 0 4px 16px rgba(15, 23, 42, 0.04);
}

.pagination-bar {
  display: flex;
  justify-content: flex-end;
  padding: 14px 16px;
  border-top: 1px solid #f1f5f9;
}

.mono { font-family: 'SF Mono', 'Fira Code', monospace; }
.text-xs { font-size: 12px; }
.text-sm { font-size: 13px; }

.model-chip {
  display: inline-block;
  background: #f1f5f9;
  border-radius: 6px;
  padding: 2px 8px;
  font-size: 12px;
  font-weight: 600;
  color: #475569;
  font-family: 'SF Mono', 'Fira Code', monospace;
}

/* 详情抽屉 */
.detail-sections {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.detail-section {
  padding: 16px 0;
  border-bottom: 1px solid #f1f5f9;
}
.detail-section:last-child { border-bottom: none; }

.detail-section h3 {
  margin: 0 0 12px;
  font-size: 11px;
  font-weight: 900;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: #94a3b8;
}

.detail-section dl {
  display: grid;
  grid-template-columns: 120px 1fr;
  gap: 6px 12px;
  margin: 0;
}

.detail-section dt {
  font-size: 12px;
  color: #64748b;
  font-weight: 600;
  display: flex;
  align-items: center;
}

.detail-section dd {
  margin: 0;
  font-size: 13px;
  color: #0f172a;
  word-break: break-all;
  display: flex;
  align-items: center;
}

.error-msg {
  color: #dc2626 !important;
  font-size: 12px !important;
}

:deep(.el-table__header th) {
  background: #f8fafc !important;
  color: #64748b;
  font-size: 11px;
  font-weight: 900;
  text-transform: uppercase;
}

:deep(.el-drawer__body) {
  padding: 0 20px 20px;
  overflow-y: auto;
}
</style>
