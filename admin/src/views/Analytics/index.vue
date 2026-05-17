<script setup>
import { computed, onMounted, shallowRef } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { formatCredits, listAnalyticsDailyTrend } from '@/api/aiGateway'

const DAY_OPTIONS = [
  { label: '近7天', value: 7 },
  { label: '近30天', value: 30 },
  { label: '近90天', value: 90 },
]

const selectedDays = shallowRef(30)
const loading = shallowRef(false)
const rows = shallowRef([])

const fetchTrend = async () => {
  loading.value = true
  try {
    rows.value = (await listAnalyticsDailyTrend({ days: selectedDays.value })) || []
  } catch (err) {
    rows.value = []
    ElMessage.error('加载趋势数据失败：' + (err?.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

onMounted(fetchTrend)

// ── Derived totals ──────────────────────────────────────────────────────────
const totals = computed(() => {
  return rows.value.reduce(
    (acc, r) => {
      acc.requests += Number(r.request_count)
      acc.success += Number(r.success_count)
      acc.failed += Number(r.failed_count)
      acc.tokens += Number(r.total_tokens)
      acc.platformCost += Number(r.platform_cost)
      acc.providerCost += Number(r.provider_cost)
      return acc
    },
    { requests: 0, success: 0, failed: 0, tokens: 0, platformCost: 0, providerCost: 0 },
  )
})

const successRate = computed(() => {
  if (!totals.value.requests) return '—'
  return ((totals.value.success / totals.value.requests) * 100).toFixed(1) + '%'
})

// ── SVG chart helpers ───────────────────────────────────────────────────────
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
    .flatMap((d, origIdx) => {
      if (origIdx % step !== 0 && origIdx !== n - 1) return []
      const x = PAD.l + origIdx * xStep
      const label = d.date ? d.date.slice(5) : ''
      return [`<text x="${x}" y="${CHART_H - 4}" text-anchor="middle" font-size="10" fill="#94a3b8">${label}</text>`]
    })
    .join('')
}

function maxVal(data, ...fns) {
  return Math.max(1, ...data.flatMap((d) => fns.map((fn) => fn(d))))
}

// Cost chart
const costSvg = computed(() => {
  const data = rows.value
  if (!data.length) return ''
  const mx = maxVal(data, (d) => d.platform_cost, (d) => d.provider_cost)
  return [
    polyline(data, (d) => d.platform_cost, '#4f46e5', mx),
    polyline(data, (d) => d.provider_cost, '#e11d48', mx),
    xLabels(data),
  ].join('')
})

// Request volume chart
const volumeSvg = computed(() => {
  const data = rows.value
  if (!data.length) return ''
  const mx = maxVal(data, (d) => d.request_count)
  return [
    polyline(data, (d) => d.request_count, '#0ea5e9', mx),
    polyline(data, (d) => d.success_count, '#16a34a', mx),
    polyline(data, (d) => d.failed_count, '#dc2626', mx),
    xLabels(data),
  ].join('')
})

// Token chart
const tokenSvg = computed(() => {
  const data = rows.value
  if (!data.length) return ''
  const mx = maxVal(data, (d) => d.prompt_tokens, (d) => d.completion_tokens)
  return [
    polyline(data, (d) => d.prompt_tokens, '#8b5cf6', mx),
    polyline(data, (d) => d.completion_tokens, '#f59e0b', mx),
    xLabels(data),
  ].join('')
})

// Latency chart
const latencySvg = computed(() => {
  const data = rows.value
  if (!data.length) return ''
  const mx = maxVal(data, (d) => d.avg_latency_ms)
  return [polyline(data, (d) => d.avg_latency_ms, '#06b6d4', mx), xLabels(data)].join('')
})
</script>

<template>
  <div class="page-container space-y-5 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <section class="page-head">
      <div>
        <p class="eyebrow">Uni AI API</p>
        <h1>趋势分析</h1>
        <p class="subtitle">按天聚合的调用量、Token 消耗、成本和延迟趋势。</p>
      </div>
      <div class="head-actions">
        <div class="segmented">
          <button
            v-for="opt in DAY_OPTIONS"
            :key="opt.value"
            class="segment-button"
            :class="{ active: selectedDays === opt.value }"
            @click="selectedDays = opt.value; fetchTrend()"
          >
            {{ opt.label }}
          </button>
        </div>
        <el-button type="primary" :icon="Refresh" :loading="loading" @click="fetchTrend">刷新</el-button>
      </div>
    </section>

    <!-- summary metrics -->
    <section class="metric-grid" v-loading="loading">
      <div class="metric">
        <span>总请求数</span>
        <strong>{{ formatCredits(totals.requests) }}</strong>
        <p>周期内累计</p>
      </div>
      <div class="metric">
        <span>成功率</span>
        <strong>{{ successRate }}</strong>
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

    <!-- charts row 1 -->
    <section class="grid grid-cols-1 xl:grid-cols-2 gap-5">
      <div class="chart-panel" v-loading="loading">
        <div class="chart-head">
          <h2>成本趋势</h2>
          <div class="legend">
            <span class="dot" style="background:#4f46e5"></span>平台成本
            <span class="dot" style="background:#e11d48; margin-left:10px"></span>Provider 成本
          </div>
        </div>
        <svg
          :viewBox="`0 0 ${600} ${120}`"
          class="chart-svg"
          v-html="costSvg"
        />
        <p v-if="!rows.length && !loading" class="no-data">暂无数据</p>
      </div>

      <div class="chart-panel" v-loading="loading">
        <div class="chart-head">
          <h2>请求量趋势</h2>
          <div class="legend">
            <span class="dot" style="background:#0ea5e9"></span>总请求
            <span class="dot" style="background:#16a34a; margin-left:10px"></span>成功
            <span class="dot" style="background:#dc2626; margin-left:10px"></span>失败
          </div>
        </div>
        <svg :viewBox="`0 0 ${600} ${120}`" class="chart-svg" v-html="volumeSvg" />
        <p v-if="!rows.length && !loading" class="no-data">暂无数据</p>
      </div>
    </section>

    <!-- charts row 2 -->
    <section class="grid grid-cols-1 xl:grid-cols-2 gap-5">
      <div class="chart-panel" v-loading="loading">
        <div class="chart-head">
          <h2>Token 趋势</h2>
          <div class="legend">
            <span class="dot" style="background:#8b5cf6"></span>Prompt
            <span class="dot" style="background:#f59e0b; margin-left:10px"></span>Completion
          </div>
        </div>
        <svg :viewBox="`0 0 ${600} ${120}`" class="chart-svg" v-html="tokenSvg" />
        <p v-if="!rows.length && !loading" class="no-data">暂无数据</p>
      </div>

      <div class="chart-panel" v-loading="loading">
        <div class="chart-head">
          <h2>平均延迟趋势</h2>
          <div class="legend">
            <span class="dot" style="background:#06b6d4"></span>Avg Latency (ms)
          </div>
        </div>
        <svg :viewBox="`0 0 ${600} ${120}`" class="chart-svg" v-html="latencySvg" />
        <p v-if="!rows.length && !loading" class="no-data">暂无数据</p>
      </div>
    </section>

    <!-- daily breakdown table -->
    <section class="panel">
      <div class="section-head">
        <div>
          <h2>每日明细</h2>
          <p>按自然日汇总，最新在上。</p>
        </div>
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
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.04);
}
.eyebrow {
  margin: 0 0 6px;
  color: #64748b;
  font-size: 12px;
  font-weight: 900;
  text-transform: uppercase;
}
.page-head h1 { margin: 0; color: #0f172a; font-size: 24px; font-weight: 900; }
.subtitle { margin: 8px 0 0; color: #64748b; font-size: 14px; }
.head-actions { display: flex; align-items: center; gap: 12px; }
.segmented { display: flex; gap: 4px; border-radius: 10px; background: #f8fafc; padding: 4px; }
.segment-button {
  border: 0; border-radius: 8px; background: transparent;
  color: #64748b; cursor: pointer; font-size: 12px; font-weight: 800; padding: 8px 10px;
}
.segment-button.active { background: #fff; color: #4f46e5; box-shadow: 0 1px 6px rgba(15,23,42,.08); }

.metric-grid { display: grid; grid-template-columns: repeat(6, minmax(0,1fr)); gap: 14px; }
.metric, .panel, .chart-panel {
  border: 1px solid #f1f5f9; border-radius: 12px; background: #fff;
  box-shadow: 0 10px 30px rgba(15,23,42,.04);
}
.metric { min-width: 0; padding: 18px; }
.metric span { display: block; color: #64748b; font-size: 12px; font-weight: 900; }
.metric strong { display: block; margin-top: 8px; color: #0f172a; font-size: 22px; font-weight: 900; line-height: 1.1; }
.metric p { margin: 8px 0 0; color: #94a3b8; font-size: 12px; font-weight: 700; }

.chart-panel { padding: 16px; min-width: 0; }
.chart-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.chart-head h2 { margin: 0; font-size: 15px; font-weight: 900; color: #0f172a; }
.legend { display: flex; align-items: center; font-size: 12px; color: #64748b; }
.dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin-right: 4px; }
.chart-svg { width: 100%; height: auto; display: block; }
.no-data { text-align: center; color: #94a3b8; font-size: 13px; margin: 20px 0; }

.panel { min-width: 0; padding: 18px; }
.section-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; }
.section-head h2 { margin: 0; color: #0f172a; font-size: 16px; font-weight: 900; }
.section-head p { margin: 4px 0 0; color: #64748b; font-size: 12px; }

@media (max-width: 1180px) {
  .metric-grid { grid-template-columns: repeat(3, minmax(0,1fr)); }
}
@media (max-width: 640px) {
  .metric-grid { grid-template-columns: repeat(2, minmax(0,1fr)); }
}
</style>
