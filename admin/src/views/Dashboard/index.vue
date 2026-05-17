<script setup>
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { Refresh, CircleCheck, CircleClose, Warning } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import {
  formatCredits,
  formatTimestamp,
  getDashboardSummary,
  getOAuthPoolHealth,
  listAnalyticsDailyTrend,
  listDashboardRecentErrors,
  listDashboardTopModels,
  listDashboardTopTenants,
  getSystemStatus
} from '@/api/aiGateway'

// ── 通用 ────────────────────────────────────────────────────────────────────
const DAY_OPTIONS = [
  { label: '近24小时', value: 1 },
  { label: '近7天', value: 7 },
  { label: '近30天', value: 30 },
  { label: '近90天', value: 90 },
  { label: '全部', value: 0 }
]

const activeTab = shallowRef('overview')
const selectedDays = shallowRef(1)
const globalLoading = shallowRef(false)

// ── 概览 Tab ─────────────────────────────────────────────────────────────────
const overviewLoading = shallowRef(false)
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

const periodLabel = computed(() => DAY_OPTIONS.find(o => o.value === selectedDays.value)?.label || '近24小时')

const successRate = computed(() => {
  const total = Number(summary.total_requests) || 0
  if (!total) return '0%'
  return `${((Number(summary.successful_requests) || 0) * 100 / total).toFixed(1)}%`
})

const statusType = s => ({ success: 'success', failed: 'danger', rejected: 'warning', partial: 'warning' }[s] || 'info')

const fetchOverview = async () => {
  overviewLoading.value = true
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
    overviewLoading.value = false
  }
}

// ── 趋势 Tab ─────────────────────────────────────────────────────────────────
const trendLoading = shallowRef(false)
const trendDays = shallowRef(30)
const TREND_DAY_OPTIONS = [
  { label: '近7天', value: 7 },
  { label: '近30天', value: 30 },
  { label: '近90天', value: 90 }
]
const rows = shallowRef([])

const totals = computed(() =>
  rows.value.reduce(
    (acc, r) => {
      acc.requests += Number(r.request_count)
      acc.success += Number(r.success_count)
      acc.failed += Number(r.failed_count)
      acc.tokens += Number(r.total_tokens)
      acc.platformCost += Number(r.platform_cost)
      acc.providerCost += Number(r.provider_cost)
      return acc
    },
    { requests: 0, success: 0, failed: 0, tokens: 0, platformCost: 0, providerCost: 0 }
  )
)

const trendSuccessRate = computed(() => {
  if (!totals.value.requests) return '—'
  return ((totals.value.success / totals.value.requests) * 100).toFixed(1) + '%'
})

const CHART_W = 600
const CHART_H = 120
const PAD = { t: 8, r: 8, b: 24, l: 8 }

function polyline(data, getY, color, maxY) {
  if (!data.length) return ''
  const n = data.length
  const xStep = (CHART_W - PAD.l - PAD.r) / Math.max(n - 1, 1)
  const yRange = CHART_H - PAD.t - PAD.b
  const pts = data.map((d, i) => {
    const x = PAD.l + i * xStep
    const y = maxY ? PAD.t + yRange * (1 - getY(d) / maxY) : PAD.t + yRange
    return `${x},${y}`
  })
  return `<polyline fill="none" stroke="${color}" stroke-width="2" stroke-linejoin="round" stroke-linecap="round" points="${pts.join(' ')}" />`
}

function xLabels(data) {
  if (!data.length) return ''
  const n = data.length
  const xStep = (CHART_W - PAD.l - PAD.r) / Math.max(n - 1, 1)
  const step = Math.ceil(n / 6)
  return data
    .flatMap((d, i) => {
      if (i % step !== 0 && i !== n - 1) return []
      const x = PAD.l + i * xStep
      return [`<text x="${x}" y="${CHART_H - 4}" text-anchor="middle" font-size="10" fill="#94a3b8">${d.date ? d.date.slice(5) : ''}</text>`]
    })
    .join('')
}

function maxVal(data, ...fns) {
  return Math.max(1, ...data.flatMap(d => fns.map(fn => fn(d))))
}

const costSvg = computed(() => {
  const d = rows.value
  if (!d.length) return ''
  const mx = maxVal(d, r => r.platform_cost, r => r.provider_cost)
  return [polyline(d, r => r.platform_cost, '#4f46e5', mx), polyline(d, r => r.provider_cost, '#e11d48', mx), xLabels(d)].join('')
})

const volumeSvg = computed(() => {
  const d = rows.value
  if (!d.length) return ''
  const mx = maxVal(d, r => r.request_count)
  return [polyline(d, r => r.request_count, '#0ea5e9', mx), polyline(d, r => r.success_count, '#16a34a', mx), polyline(d, r => r.failed_count, '#dc2626', mx), xLabels(d)].join('')
})

const tokenSvg = computed(() => {
  const d = rows.value
  if (!d.length) return ''
  const mx = maxVal(d, r => r.prompt_tokens, r => r.completion_tokens)
  return [polyline(d, r => r.prompt_tokens, '#8b5cf6', mx), polyline(d, r => r.completion_tokens, '#f59e0b', mx), xLabels(d)].join('')
})

const latencySvg = computed(() => {
  const d = rows.value
  if (!d.length) return ''
  const mx = maxVal(d, r => r.avg_latency_ms)
  return [polyline(d, r => r.avg_latency_ms, '#06b6d4', mx), xLabels(d)].join('')
})

const fetchTrend = async () => {
  trendLoading.value = true
  try {
    rows.value = (await listAnalyticsDailyTrend({ days: trendDays.value })) || []
  } catch (err) {
    rows.value = []
    ElMessage.error('加载趋势数据失败：' + (err?.message || '未知错误'))
  } finally {
    trendLoading.value = false
  }
}

// ── 系统 Tab ─────────────────────────────────────────────────────────────────
const systemLoading = shallowRef(false)
const sysStatus = shallowRef(null)

const componentTag = s => s === 'ok' ? 'success' : s === 'disabled' ? 'info' : 'danger'
const componentIcon = s => s === 'ok' ? CircleCheck : s === 'disabled' ? Warning : CircleClose
const formatUntil = ts => ts ? new Date(ts).toLocaleString('zh-CN') : '—'

const fetchSystem = async () => {
  systemLoading.value = true
  try {
    sysStatus.value = await getSystemStatus()
  } catch (err) {
    sysStatus.value = null
    ElMessage.error('加载系统状态失败：' + (err?.message || '未知错误'))
  } finally {
    systemLoading.value = false
  }
}

// ── 全局刷新 ──────────────────────────────────────────────────────────────────
const handleDaysChange = (days) => {
  selectedDays.value = days
  fetchOverview()
}

const refreshAll = async () => {
  globalLoading.value = true
  try {
    await Promise.all([fetchOverview(), fetchTrend(), fetchSystem()])
  } finally {
    globalLoading.value = false
  }
}

onMounted(refreshAll)
</script>

<template>
  <div class="page-container space-y-5 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <!-- 页头 -->
    <section class="page-head">
      <div>
        <p class="eyebrow">Uni AI API</p>
        <h1>数据大盘</h1>
        <p class="subtitle">请求统计、历史趋势与系统健康的一站式监控视图。</p>
      </div>
      <el-button type="primary" :icon="Refresh" :loading="globalLoading" @click="refreshAll">刷新全部</el-button>
    </section>

    <!-- Tabs -->
    <el-tabs v-model="activeTab" class="data-tabs">
      <!-- ── 概览 ──────────────────────────────────────────────────────────── -->
      <el-tab-pane label="概览" name="overview">
        <div class="space-y-5">
          <!-- 时间段选择 -->
          <div class="flex items-center justify-between">
            <div class="segmented">
              <button
                v-for="opt in DAY_OPTIONS"
                :key="opt.value"
                class="segment-button"
                :class="{ active: selectedDays === opt.value }"
                @click="handleDaysChange(opt.value)"
              >{{ opt.label }}</button>
            </div>
          </div>

          <!-- 指标卡 -->
          <section v-loading="overviewLoading" class="metric-grid">
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
              <span>平台成本</span>
              <strong>{{ formatCredits(summary.total_platform_cost) }}</strong>
              <p>平台承担成本</p>
            </div>
            <div class="metric">
              <span>异常数</span>
              <strong>{{ formatCredits(summary.failed_requests) }}</strong>
              <p>均延迟 {{ formatCredits(summary.avg_latency_ms) }} ms</p>
            </div>
          </section>

          <!-- Top 模型 / 租户 -->
          <section class="grid grid-cols-1 xl:grid-cols-2 gap-5">
            <div class="panel">
              <div class="section-head">
                <div><h2>Top 模型</h2><p>按积分消耗和请求量排序</p></div>
              </div>
              <el-table v-loading="overviewLoading" :data="topModels" border stripe class="w-full">
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
                <div><h2>Top 租户</h2><p>按积分消耗和请求量排序</p></div>
              </div>
              <el-table v-loading="overviewLoading" :data="topTenants" border stripe class="w-full">
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

          <!-- 最近错误 -->
          <section class="panel">
            <div class="section-head">
              <div><h2>最近错误</h2><p>用于快速定位上游失败、鉴权拒绝和计费异常。</p></div>
            </div>
            <el-table v-loading="overviewLoading" :data="recentErrors" border stripe class="w-full">
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

          <!-- OAuth 凭据池 -->
          <section v-if="oauthPoolHealth.length > 0" class="panel">
            <div class="section-head">
              <div><h2>OAuth 凭据池</h2><p>各账号池 OAuth 凭据状态汇总，快速识别失效或即将过期的凭据。</p></div>
            </div>
            <el-table v-loading="overviewLoading" :data="oauthPoolHealth" border stripe class="w-full">
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
                <template #default="{ row }"><span class="count-active">{{ row.active }}</span></template>
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
                    style="width:90px;display:inline-block"
                  />
                </template>
              </el-table-column>
            </el-table>
          </section>
        </div>
      </el-tab-pane>

      <!-- ── 趋势 ──────────────────────────────────────────────────────────── -->
      <el-tab-pane label="趋势分析" name="trend">
        <div class="space-y-5">
          <!-- 时间段 + 刷新 -->
          <div class="flex items-center gap-3">
            <div class="segmented">
              <button
                v-for="opt in TREND_DAY_OPTIONS"
                :key="opt.value"
                class="segment-button"
                :class="{ active: trendDays === opt.value }"
                @click="trendDays = opt.value; fetchTrend()"
              >{{ opt.label }}</button>
            </div>
            <el-button :icon="Refresh" :loading="trendLoading" @click="fetchTrend">刷新</el-button>
          </div>

          <!-- 汇总指标 -->
          <section v-loading="trendLoading" class="metric-grid-6">
            <div class="metric">
              <span>总请求数</span>
              <strong>{{ formatCredits(totals.requests) }}</strong>
              <p>周期内累计</p>
            </div>
            <div class="metric">
              <span>成功率</span>
              <strong>{{ trendSuccessRate }}</strong>
              <p>{{ formatCredits(totals.success) }} 次成功</p>
            </div>
            <div class="metric">
              <span>失败次数</span>
              <strong>{{ formatCredits(totals.failed) }}</strong>
              <p>含上游错误 / 鉴权失败</p>
            </div>
            <div class="metric">
              <span>总 Token</span>
              <strong>{{ formatCredits(totals.tokens) }}</strong>
              <p>Prompt + Completion</p>
            </div>
            <div class="metric">
              <span>平台成本</span>
              <strong>{{ formatCredits(totals.platformCost) }}</strong>
              <p>积分单位</p>
            </div>
            <div class="metric">
              <span>Provider 成本</span>
              <strong>{{ formatCredits(totals.providerCost) }}</strong>
              <p>上游实际成本</p>
            </div>
          </section>

          <!-- 图表 Row 1 -->
          <section class="grid grid-cols-1 xl:grid-cols-2 gap-5">
            <div class="chart-panel" v-loading="trendLoading">
              <div class="chart-head">
                <h2>成本趋势</h2>
                <div class="legend">
                  <span class="dot" style="background:#4f46e5"></span>平台成本
                  <span class="dot" style="background:#e11d48;margin-left:10px"></span>Provider 成本
                </div>
              </div>
              <svg :viewBox="`0 0 ${CHART_W} ${CHART_H}`" class="chart-svg" v-html="costSvg" />
              <p v-if="!rows.length && !trendLoading" class="no-data">暂无数据</p>
            </div>
            <div class="chart-panel" v-loading="trendLoading">
              <div class="chart-head">
                <h2>请求量趋势</h2>
                <div class="legend">
                  <span class="dot" style="background:#0ea5e9"></span>总请求
                  <span class="dot" style="background:#16a34a;margin-left:10px"></span>成功
                  <span class="dot" style="background:#dc2626;margin-left:10px"></span>失败
                </div>
              </div>
              <svg :viewBox="`0 0 ${CHART_W} ${CHART_H}`" class="chart-svg" v-html="volumeSvg" />
              <p v-if="!rows.length && !trendLoading" class="no-data">暂无数据</p>
            </div>
          </section>

          <!-- 图表 Row 2 -->
          <section class="grid grid-cols-1 xl:grid-cols-2 gap-5">
            <div class="chart-panel" v-loading="trendLoading">
              <div class="chart-head">
                <h2>Token 趋势</h2>
                <div class="legend">
                  <span class="dot" style="background:#8b5cf6"></span>Prompt
                  <span class="dot" style="background:#f59e0b;margin-left:10px"></span>Completion
                </div>
              </div>
              <svg :viewBox="`0 0 ${CHART_W} ${CHART_H}`" class="chart-svg" v-html="tokenSvg" />
              <p v-if="!rows.length && !trendLoading" class="no-data">暂无数据</p>
            </div>
            <div class="chart-panel" v-loading="trendLoading">
              <div class="chart-head">
                <h2>平均延迟趋势</h2>
                <div class="legend">
                  <span class="dot" style="background:#06b6d4"></span>Avg Latency (ms)
                </div>
              </div>
              <svg :viewBox="`0 0 ${CHART_W} ${CHART_H}`" class="chart-svg" v-html="latencySvg" />
              <p v-if="!rows.length && !trendLoading" class="no-data">暂无数据</p>
            </div>
          </section>

          <!-- 每日明细 -->
          <section class="panel">
            <div class="section-head">
              <div><h2>每日明细</h2><p>按自然日汇总，最新在上。</p></div>
            </div>
            <el-table v-loading="trendLoading" :data="[...rows].reverse()" border stripe class="w-full" size="small">
              <el-table-column prop="date" label="日期" width="110" />
              <el-table-column label="请求数" width="90" align="right">
                <template #default="{ row }">{{ formatCredits(row.request_count) }}</template>
              </el-table-column>
              <el-table-column label="成功" width="80" align="right">
                <template #default="{ row }">{{ formatCredits(row.success_count) }}</template>
              </el-table-column>
              <el-table-column label="失败" width="80" align="right">
                <template #default="{ row }">
                  <span :class="row.failed_count > 0 ? 'text-red-500 font-bold' : ''">{{ formatCredits(row.failed_count) }}</span>
                </template>
              </el-table-column>
              <el-table-column label="Token" width="110" align="right">
                <template #default="{ row }">{{ formatCredits(row.total_tokens) }}</template>
              </el-table-column>
              <el-table-column label="Prompt" width="100" align="right">
                <template #default="{ row }">{{ formatCredits(row.prompt_tokens) }}</template>
              </el-table-column>
              <el-table-column label="Completion" width="110" align="right">
                <template #default="{ row }">{{ formatCredits(row.completion_tokens) }}</template>
              </el-table-column>
              <el-table-column label="平台成本" width="110" align="right">
                <template #default="{ row }">{{ formatCredits(row.platform_cost) }}</template>
              </el-table-column>
              <el-table-column label="Provider成本" width="120" align="right">
                <template #default="{ row }">{{ formatCredits(row.provider_cost) }}</template>
              </el-table-column>
              <el-table-column label="均延迟(ms)" width="110" align="right">
                <template #default="{ row }">{{ formatCredits(row.avg_latency_ms) }}</template>
              </el-table-column>
            </el-table>
          </section>
        </div>
      </el-tab-pane>

      <!-- ── 系统 ──────────────────────────────────────────────────────────── -->
      <el-tab-pane label="系统状态" name="system">
        <div class="space-y-5">
          <div class="flex justify-end">
            <el-button :icon="Refresh" :loading="systemLoading" @click="fetchSystem">刷新</el-button>
          </div>

          <div v-if="sysStatus">
            <!-- 基础设施 -->
            <p class="section-label">基础设施健康</p>
            <div class="infra-cards">
              <div class="infra-card" :class="`infra-card--${componentTag(sysStatus.db?.status)}`">
                <el-icon :size="28"><component :is="componentIcon(sysStatus.db?.status)" /></el-icon>
                <div class="infra-info">
                  <p class="infra-name">PostgreSQL</p>
                  <p class="infra-status">{{ sysStatus.db?.status }}</p>
                  <p v-if="sysStatus.db?.error" class="infra-error">{{ sysStatus.db.error }}</p>
                </div>
              </div>
              <div class="infra-card" :class="`infra-card--${componentTag(sysStatus.redis?.status)}`">
                <el-icon :size="28"><component :is="componentIcon(sysStatus.redis?.status)" /></el-icon>
                <div class="infra-info">
                  <p class="infra-name">Redis</p>
                  <p class="infra-status">{{ sysStatus.redis?.status }}</p>
                  <p v-if="sysStatus.redis?.error" class="infra-error">{{ sysStatus.redis.error }}</p>
                </div>
              </div>
            </div>

            <!-- 熔断器 -->
            <p class="section-label" style="margin-top:2rem">
              熔断器状态
              <span class="cb-summary">
                跟踪中 {{ sysStatus.circuit_breaker.total_tracked }} 个部署
                <el-tag v-if="sysStatus.circuit_breaker.open_count > 0" type="danger" size="small" class="ml-2">
                  {{ sysStatus.circuit_breaker.open_count }} 个熔断
                </el-tag>
                <el-tag v-else type="success" size="small" class="ml-2">全部健康</el-tag>
              </span>
            </p>

            <div v-if="sysStatus.circuit_breaker.total_tracked === 0" class="empty-hint">
              暂无跟踪中的部署（所有线路处于默认健康状态）
            </div>
            <el-table v-else :data="sysStatus.circuit_breaker.states" stripe class="cb-table">
              <el-table-column label="部署 ID" prop="deployment_id" min-width="280">
                <template #default="{ row }"><span class="mono">{{ row.deployment_id }}</span></template>
              </el-table-column>
              <el-table-column label="状态" width="120">
                <template #default="{ row }">
                  <el-tag :type="row.open ? 'danger' : 'success'" size="small">{{ row.open ? '熔断' : '正常' }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="连续失败次数" prop="consecutive_failures" width="140" align="right" />
              <el-table-column label="冷却到期时间" min-width="180">
                <template #default="{ row }">{{ formatUntil(row.unhealthy_until) }}</template>
              </el-table-column>
            </el-table>

            <p class="page-footer">最后刷新：{{ new Date(sysStatus.timestamp).toLocaleString('zh-CN') }}</p>
          </div>

          <el-skeleton v-else-if="systemLoading" :rows="6" animated />
          <div v-else class="empty-hint">暂无数据，请点击刷新</div>
        </div>
      </el-tab-pane>
    </el-tabs>
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
  background: #fff;
  padding: 24px;
  box-shadow: 0 10px 30px rgba(15,23,42,.04);
}
.eyebrow { margin: 0 0 6px; color: #64748b; font-size: 12px; font-weight: 900; text-transform: uppercase; }
.page-head h1 { margin: 0; color: #0f172a; font-size: 24px; font-weight: 900; }
.subtitle { margin: 8px 0 0; color: #64748b; font-size: 14px; }

/* Tabs 样式覆盖 */
.data-tabs :deep(.el-tabs__header) {
  margin-bottom: 20px;
  border-bottom: 1px solid #f1f5f9;
}
.data-tabs :deep(.el-tabs__item) {
  font-size: 14px;
  font-weight: 700;
  color: #94a3b8;
}
.data-tabs :deep(.el-tabs__item.is-active) { color: #4f46e5; }
.data-tabs :deep(.el-tabs__active-bar) { background-color: #4f46e5; }

/* 时间段选择 */
.segmented { display: flex; gap: 4px; border-radius: 10px; background: #f8fafc; padding: 4px; }
.segment-button {
  border: 0; border-radius: 8px; background: transparent;
  color: #64748b; cursor: pointer; font-size: 12px; font-weight: 800; padding: 8px 10px;
}
.segment-button.active { background: #fff; color: #4f46e5; box-shadow: 0 1px 6px rgba(15,23,42,.08); }

/* 指标卡 */
.metric-grid { display: grid; grid-template-columns: repeat(6, minmax(0,1fr)); gap: 14px; }
.metric-grid-6 { display: grid; grid-template-columns: repeat(6, minmax(0,1fr)); gap: 14px; }
.metric, .panel, .chart-panel {
  border: 1px solid #f1f5f9; border-radius: 12px; background: #fff;
  box-shadow: 0 10px 30px rgba(15,23,42,.04);
}
.metric { min-width: 0; padding: 18px; }
.metric span { display: block; color: #64748b; font-size: 12px; font-weight: 900; }
.metric strong { display: block; margin-top: 8px; color: #0f172a; font-size: 22px; font-weight: 900; line-height: 1.1; }
.metric p { margin: 8px 0 0; color: #94a3b8; font-size: 12px; font-weight: 700; }

/* 图表 */
.chart-panel { padding: 16px; min-width: 0; }
.chart-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.chart-head h2 { margin: 0; font-size: 15px; font-weight: 900; color: #0f172a; }
.legend { display: flex; align-items: center; font-size: 12px; color: #64748b; }
.dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin-right: 4px; }
.chart-svg { width: 100%; height: auto; display: block; }
.no-data { text-align: center; color: #94a3b8; font-size: 13px; margin: 20px 0; }

/* 面板 */
.panel { min-width: 0; padding: 18px; }
.section-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; }
.section-head h2 { margin: 0; color: #0f172a; font-size: 16px; font-weight: 900; }
.section-head p { margin: 4px 0 0; color: #64748b; font-size: 12px; }

/* 系统状态 */
.section-label {
  display: flex; align-items: center; gap: 8px;
  font-size: 12px; font-weight: 900; letter-spacing: .05em;
  text-transform: uppercase; color: #94a3b8; margin-bottom: 12px;
}
.cb-summary { display: flex; align-items: center; gap: 8px; font-weight: 600; font-size: 13px; text-transform: none; letter-spacing: 0; color: #475569; }
.infra-cards { display: flex; gap: 16px; flex-wrap: wrap; }
.infra-card {
  display: flex; align-items: center; gap: 16px;
  padding: 20px 24px; border-radius: 16px;
  border: 1px solid #f1f5f9; background: #fff;
  min-width: 220px; box-shadow: 0 2px 8px rgba(15,23,42,.05);
}
.infra-card--success :deep(.el-icon) { color: #22c55e; }
.infra-card--danger { border-color: #fecaca; background: #fff5f5; }
.infra-card--danger :deep(.el-icon) { color: #ef4444; }
.infra-card--info :deep(.el-icon) { color: #94a3b8; }
.infra-info { display: flex; flex-direction: column; gap: 2px; }
.infra-name { margin: 0; font-weight: 800; font-size: 14px; color: #1e293b; }
.infra-status { margin: 0; font-size: 12px; color: #64748b; text-transform: uppercase; letter-spacing: .05em; }
.infra-error { margin: 0; font-size: 11px; color: #ef4444; }
.cb-table { border-radius: 12px; overflow: hidden; }
.mono { font-family: 'JetBrains Mono','Fira Code',monospace; font-size: 13px; color: #475569; }
.empty-hint { color: #94a3b8; font-size: 14px; padding: 24px; text-align: center; background: #f8fafc; border-radius: 12px; }
.page-footer { margin-top: 24px; font-size: 12px; color: #cbd5e1; text-align: right; }
.ml-2 { margin-left: 8px; }

/* OAuth 凭据池 */
.count-active { color: #16a34a; font-weight: 700; }
.count-invalid { color: #dc2626; font-weight: 700; }
.count-warn { color: #d97706; font-weight: 700; }
.count-zero { color: #94a3b8; }
.strategy-badge { display: inline-block; padding: 2px 8px; border-radius: 6px; font-size: 11px; font-weight: 700; background: #f1f5f9; color: #475569; }
.strategy-badge.weighted { background: #ede9fe; color: #6d28d9; }

@media (max-width: 1180px) {
  .metric-grid, .metric-grid-6 { grid-template-columns: repeat(3, minmax(0,1fr)); }
}
@media (max-width: 640px) {
  .metric-grid, .metric-grid-6 { grid-template-columns: repeat(2, minmax(0,1fr)); }
}
</style>
