<script setup>
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { useRouter } from 'vue-router'
import { Refresh, Warning } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import {
  formatCredits,
  formatTimestamp,
  getDashboardSummary,
  getOAuthPoolHealth,
  getSystemStatus,
  listAnalyticsDailyTrend,
  listDashboardRecentErrors,
  listDashboardTopModels,
  listDashboardTopTenants
} from '@/api/aiGateway'

const router = useRouter()

const DAY_OPTIONS = [
  { label: '近24小时', value: 1 },
  { label: '近7天', value: 7 },
  { label: '近30天', value: 30 },
  { label: '近90天', value: 90 }
]

const selectedDays = shallowRef(1)
const loading = shallowRef(false)

const periodLabel = computed(() => DAY_OPTIONS.find(o => o.value === selectedDays.value)?.label || '近24小时')
const showTrend = computed(() => selectedDays.value >= 7)

// ── 概览数据 ─────────────────────────────────────────────────────────────────
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
  total_provider_credits: 0,
  total_platform_credits: 0,
  total_user_credits: 0,
  avg_latency_ms: 0
})
const topModels = shallowRef([])
const topTenants = shallowRef([])
const recentErrors = shallowRef([])
const oauthPoolHealth = shallowRef([])

const successRate = computed(() => {
  const total = Number(summary.total_requests) || 0
  if (!total) return '0%'
  return `${((Number(summary.successful_requests) || 0) * 100 / total).toFixed(1)}%`
})

const statusType = s => ({ success: 'success', failed: 'danger', rejected: 'warning', partial: 'warning' }[s] || 'info')

// ── 趋势数据 ─────────────────────────────────────────────────────────────────
const rows = shallowRef([])

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
  const mx = maxVal(d, r => r.platform_credits, r => r.provider_credits)
  return [polyline(d, r => r.platform_credits, '#4f46e5', mx), polyline(d, r => r.provider_credits, '#e11d48', mx), xLabels(d)].join('')
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

// ── 基础设施异常徽标 ──────────────────────────────────────────────────────────
const infraAlert = shallowRef(null) // { reason: string }

const fetchInfraStatus = async () => {
  try {
    const sys = await getSystemStatus()
    const health = sys?.health
    const reasons = []
    if (sys?.db?.status && sys.db.status !== 'ok') reasons.push('数据库异常')
    if (sys?.redis?.status && sys.redis.status !== 'ok' && sys.redis.status !== 'disabled') reasons.push('Redis 异常')
    if (health?.open_count > 0) reasons.push(`${health.open_count} 个熔断`)
    infraAlert.value = reasons.length ? { reason: reasons.join(' · ') } : null
  } catch {
    infraAlert.value = null
  }
}

// ── 拉取 ──────────────────────────────────────────────────────────────────────
const refreshAll = async () => {
  loading.value = true
  try {
    const params = { days: selectedDays.value }
    const tasks = [
      getDashboardSummary(params).then(r => Object.assign(summary, r || {})),
      listDashboardTopModels({ ...params, limit: 8 }).then(r => { topModels.value = r || [] }),
      listDashboardTopTenants({ ...params, limit: 8 }).then(r => { topTenants.value = r || [] }),
      listDashboardRecentErrors({ ...params, limit: 8 }).then(r => { recentErrors.value = r || [] }),
      getOAuthPoolHealth().then(r => { oauthPoolHealth.value = r || [] }),
      fetchInfraStatus()
    ]
    if (showTrend.value) {
      tasks.push(
        listAnalyticsDailyTrend({ days: selectedDays.value })
          .then(r => { rows.value = r || [] })
          .catch(err => {
            rows.value = []
            ElMessage.error('加载趋势数据失败：' + (err?.message || '未知错误'))
          })
      )
    } else {
      rows.value = []
    }
    await Promise.all(tasks)
  } finally {
    loading.value = false
  }
}

const handleDaysChange = (days) => {
  selectedDays.value = days
  refreshAll()
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
        <p class="subtitle">请求统计、历史趋势与异常监控的一站式视图。</p>
      </div>
      <div class="page-head-actions">
        <button
          v-if="infraAlert"
          class="infra-alert-badge"
          @click="router.push('/system-status')"
        >
          <el-icon :size="14"><Warning /></el-icon>
          基础设施：{{ infraAlert.reason }}
        </button>
        <el-button type="primary" :icon="Refresh" :loading="loading" @click="refreshAll">刷新</el-button>
      </div>
    </section>

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
        <strong>{{ formatCredits(summary.total_user_credits) }}</strong>
        <p>按业务扣费口径统计</p>
      </div>
      <div class="metric">
        <span>Token</span>
        <strong>{{ formatCredits(summary.total_tokens) }}</strong>
        <p>Chat / Responses / Embedding</p>
      </div>
      <div class="metric">
        <span>平台成本</span>
        <strong>{{ formatCredits(summary.total_platform_credits) }}</strong>
        <p>平台承担成本</p>
      </div>
      <div class="metric">
        <span>异常数</span>
        <strong>{{ formatCredits(summary.failed_requests) }}</strong>
        <p>均延迟 {{ formatCredits(summary.avg_latency_ms) }} ms</p>
      </div>
    </section>

    <!-- 趋势图区（仅 ≥7 天显示） -->
    <template v-if="showTrend">
      <section class="grid grid-cols-1 xl:grid-cols-2 gap-5">
        <div class="chart-panel" v-loading="loading">
          <div class="chart-head">
            <h2>成本趋势</h2>
            <div class="legend">
              <span class="dot" style="background:#4f46e5"></span>平台成本
              <span class="dot" style="background:#e11d48;margin-left:10px"></span>Provider 成本
            </div>
          </div>
          <svg :viewBox="`0 0 ${CHART_W} ${CHART_H}`" class="chart-svg" v-html="costSvg" />
          <p v-if="!rows.length && !loading" class="no-data">暂无数据</p>
        </div>
        <div class="chart-panel" v-loading="loading">
          <div class="chart-head">
            <h2>请求量趋势</h2>
            <div class="legend">
              <span class="dot" style="background:#0ea5e9"></span>总请求
              <span class="dot" style="background:#16a34a;margin-left:10px"></span>成功
              <span class="dot" style="background:#dc2626;margin-left:10px"></span>失败
            </div>
          </div>
          <svg :viewBox="`0 0 ${CHART_W} ${CHART_H}`" class="chart-svg" v-html="volumeSvg" />
          <p v-if="!rows.length && !loading" class="no-data">暂无数据</p>
        </div>
      </section>

      <section class="grid grid-cols-1 xl:grid-cols-2 gap-5">
        <div class="chart-panel" v-loading="loading">
          <div class="chart-head">
            <h2>Token 趋势</h2>
            <div class="legend">
              <span class="dot" style="background:#8b5cf6"></span>Prompt
              <span class="dot" style="background:#f59e0b;margin-left:10px"></span>Completion
            </div>
          </div>
          <svg :viewBox="`0 0 ${CHART_W} ${CHART_H}`" class="chart-svg" v-html="tokenSvg" />
          <p v-if="!rows.length && !loading" class="no-data">暂无数据</p>
        </div>
        <div class="chart-panel" v-loading="loading">
          <div class="chart-head">
            <h2>平均延迟趋势</h2>
            <div class="legend">
              <span class="dot" style="background:#06b6d4"></span>Avg Latency (ms)
            </div>
          </div>
          <svg :viewBox="`0 0 ${CHART_W} ${CHART_H}`" class="chart-svg" v-html="latencySvg" />
          <p v-if="!rows.length && !loading" class="no-data">暂无数据</p>
        </div>
      </section>
    </template>
    <div v-else class="trend-hint">
      趋势按天粒度聚合，请选择 <strong>近 7 天</strong> 或更长时间段查看趋势图与每日明细。
    </div>

    <!-- Top 模型 / 租户 -->
    <section class="grid grid-cols-1 xl:grid-cols-2 gap-5">
      <div class="panel">
        <div class="section-head">
          <div><h2>Top 模型</h2><p>按积分消耗和请求量排序</p></div>
        </div>
        <el-table v-loading="loading" :data="topModels" border stripe class="w-full">
          <el-table-column prop="model_code" label="模型" min-width="150" show-overflow-tooltip />
          <el-table-column label="请求数" width="100" align="right">
            <template #default="{ row }">{{ formatCredits(row.request_count) }}</template>
          </el-table-column>
          <el-table-column label="业务计费" width="110" align="right">
            <template #default="{ row }">{{ formatCredits(row.total_credits) }}</template>
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
        <el-table v-loading="loading" :data="topTenants" border stripe class="w-full">
          <el-table-column prop="tenant_id" label="租户" min-width="170" show-overflow-tooltip />
          <el-table-column label="请求数" width="100" align="right">
            <template #default="{ row }">{{ formatCredits(row.request_count) }}</template>
          </el-table-column>
          <el-table-column label="Token" width="110" align="right">
            <template #default="{ row }">{{ formatCredits(row.total_tokens) }}</template>
          </el-table-column>
          <el-table-column label="业务计费" width="120" align="right">
            <template #default="{ row }">{{ formatCredits(row.total_credits) }}</template>
          </el-table-column>
        </el-table>
      </div>
    </section>

    <!-- 最近错误 -->
    <section class="panel">
      <div class="section-head">
        <div><h2>最近错误</h2><p>用于快速定位上游失败、鉴权拒绝和计费异常。</p></div>
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

    <!-- OAuth 凭据池 -->
    <section v-if="oauthPoolHealth.length > 0" class="panel">
      <div class="section-head">
        <div><h2>OAuth 凭据池</h2><p>各账号池 OAuth 凭据状态汇总，快速识别失效或即将过期的凭据。</p></div>
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

    <!-- 每日明细（仅 ≥7 天显示） -->
    <section v-if="showTrend" class="panel">
      <div class="section-head">
        <div><h2>每日明细</h2><p>按自然日汇总，最新在上。</p></div>
      </div>
      <el-table v-loading="loading" :data="[...rows].reverse()" border stripe class="w-full" size="small">
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
          <template #default="{ row }">{{ formatCredits(row.platform_credits) }}</template>
        </el-table-column>
        <el-table-column label="Provider成本" width="120" align="right">
          <template #default="{ row }">{{ formatCredits(row.provider_credits) }}</template>
        </el-table-column>
        <el-table-column label="均延迟(ms)" width="110" align="right">
          <template #default="{ row }">{{ formatCredits(row.avg_latency_ms) }}</template>
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
  background: #fff;
  padding: 24px;
  box-shadow: 0 10px 30px rgba(15,23,42,.04);
}
.page-head-actions { display: flex; align-items: center; gap: 12px; }
.eyebrow { margin: 0 0 6px; color: #64748b; font-size: 12px; font-weight: 900; text-transform: uppercase; }
.page-head h1 { margin: 0; color: #0f172a; font-size: 24px; font-weight: 900; }
.subtitle { margin: 8px 0 0; color: #64748b; font-size: 14px; }

.infra-alert-badge {
  display: inline-flex; align-items: center; gap: 6px;
  border: 1px solid #fecaca; background: #fff5f5; color: #b91c1c;
  border-radius: 999px; padding: 6px 14px; cursor: pointer;
  font-size: 12px; font-weight: 800;
  transition: background .2s;
}
.infra-alert-badge:hover { background: #fee2e2; }

/* 时间段选择 */
.segmented { display: flex; gap: 4px; border-radius: 10px; background: #f8fafc; padding: 4px; }
.segment-button {
  border: 0; border-radius: 8px; background: transparent;
  color: #64748b; cursor: pointer; font-size: 12px; font-weight: 800; padding: 8px 10px;
}
.segment-button.active { background: #fff; color: #4f46e5; box-shadow: 0 1px 6px rgba(15,23,42,.08); }

/* 指标卡 */
.metric-grid { display: grid; grid-template-columns: repeat(6, minmax(0,1fr)); gap: 14px; }
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

.trend-hint {
  border: 1px dashed #cbd5e1; border-radius: 12px;
  background: #f8fafc; color: #64748b;
  padding: 18px 24px; font-size: 13px; text-align: center;
}
.trend-hint strong { color: #4f46e5; font-weight: 800; }

/* 面板 */
.panel { min-width: 0; padding: 18px; }
.section-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; }
.section-head h2 { margin: 0; color: #0f172a; font-size: 16px; font-weight: 900; }
.section-head p { margin: 4px 0 0; color: #64748b; font-size: 12px; }

/* OAuth 凭据池 */
.count-active { color: #16a34a; font-weight: 700; }
.count-invalid { color: #dc2626; font-weight: 700; }
.count-warn { color: #d97706; font-weight: 700; }
.count-zero { color: #94a3b8; }
.strategy-badge { display: inline-block; padding: 2px 8px; border-radius: 6px; font-size: 11px; font-weight: 700; background: #f1f5f9; color: #475569; }
.strategy-badge.weighted { background: #ede9fe; color: #6d28d9; }

@media (max-width: 1180px) {
  .metric-grid { grid-template-columns: repeat(3, minmax(0,1fr)); }
}
@media (max-width: 640px) {
  .metric-grid { grid-template-columns: repeat(2, minmax(0,1fr)); }
}
</style>
