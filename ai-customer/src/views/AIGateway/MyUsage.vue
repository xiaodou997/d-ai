<script setup>
import { computed, nextTick, onMounted, onUnmounted, reactive, shallowRef, watch } from 'vue'
import { Refresh, Wallet, DataLine, TrendCharts, Coin, Lock, Check, Upload, Download } from '@element-plus/icons-vue'
import { listMyUsageLogs, getMyUsageSummary, formatCredits } from '@/api/aiGateway'
import { getBalance } from '@/api/customer'
import * as echarts from 'echarts'

const LOG_LIMIT = 500

const loading = shallowRef(false)
const usageLogs = shallowRef([])
const summary = shallowRef(null)
const requestSource = shallowRef('')

const balanceInfo = reactive({
  totalCredits: 0,
  frozenCredits: 0,
  availableCredits: 0
})

const chartModelRef = shallowRef(null)
const chartTimelineRef = shallowRef(null)
const chartTokenRef = shallowRef(null)
let chartModel = null
let chartTimeline = null
let chartToken = null

const last7DayKeys = () => {
  const keys = []
  for (let i = 6; i >= 0; i -= 1) {
    const date = new Date()
    date.setHours(0, 0, 0, 0)
    date.setDate(date.getDate() - i)
    keys.push(date.toISOString().slice(0, 10))
  }
  return keys
}

const modelDistribution = computed(() => {
  const grouped = new Map()
  for (const row of usageLogs.value) {
    const modelCode = row.model_code || 'unknown'
    const cost = Number(row.user_cost) || 0
    grouped.set(modelCode, (grouped.get(modelCode) || 0) + cost)
  }
  return Array.from(grouped.entries())
    .map(([name, value]) => ({ name, value }))
    .sort((a, b) => b.value - a.value)
    .slice(0, 8)
})

const timelineDayKeys = computed(() => last7DayKeys())

const timelineLabels = computed(() =>
  timelineDayKeys.value.map((dayKey) =>
    new Date(`${dayKey}T00:00:00`).toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' })
  )
)

const timelineValues = computed(() => {
  const grouped = new Map()
  for (const row of usageLogs.value) {
    if (!row.created_at) continue
    const createdAt = new Date(row.created_at)
    if (Number.isNaN(createdAt.getTime())) continue
    const dayKey = createdAt.toISOString().slice(0, 10)
    grouped.set(dayKey, (grouped.get(dayKey) || 0) + (Number(row.user_cost) || 0))
  }
  return timelineDayKeys.value.map((dayKey) => grouped.get(dayKey) || 0)
})

const timelinePromptTokens = computed(() => {
  const grouped = new Map()
  for (const row of usageLogs.value) {
    if (!row.created_at) continue
    const createdAt = new Date(row.created_at)
    if (Number.isNaN(createdAt.getTime())) continue
    const dayKey = createdAt.toISOString().slice(0, 10)
    grouped.set(dayKey, (grouped.get(dayKey) || 0) + (Number(row.prompt_tokens) || 0))
  }
  return timelineDayKeys.value.map((dayKey) => grouped.get(dayKey) || 0)
})

const timelineCompletionTokens = computed(() => {
  const grouped = new Map()
  for (const row of usageLogs.value) {
    if (!row.created_at) continue
    const createdAt = new Date(row.created_at)
    if (Number.isNaN(createdAt.getTime())) continue
    const dayKey = createdAt.toISOString().slice(0, 10)
    grouped.set(dayKey, (grouped.get(dayKey) || 0) + (Number(row.completion_tokens) || 0))
  }
  return timelineDayKeys.value.map((dayKey) => grouped.get(dayKey) || 0)
})

const fetchBalance = async () => {
  try {
    const data = await getBalance()
    if (data) {
      balanceInfo.totalCredits = data.totalCredits ?? 0
      balanceInfo.frozenCredits = data.frozenCredits ?? 0
      balanceInfo.availableCredits = data.availableCredits ?? 0
    }
  } catch (e) {
    console.error('获取余额失败:', e)
  }
}

const fetchUsageData = async () => {
  loading.value = true
  try {
    const params = {}
    if (requestSource.value) params.request_source = requestSource.value
    const [logsRes, summaryRes] = await Promise.all([
      listMyUsageLogs({ limit: LOG_LIMIT, ...params }),
      getMyUsageSummary(params)
    ])
    usageLogs.value = logsRes || []
    summary.value = summaryRes || null
  } finally {
    loading.value = false
  }
}

const fetchAllData = async () => {
  await Promise.all([fetchUsageData(), fetchBalance()])
}

const requestSourceOptions = [
  { label: '全部来源', value: '' },
  { label: 'API Key 调用', value: 'api_key' },
  { label: '网页对话', value: 'web_chat' }
]

const requestSourceLabel = (value) =>
  requestSourceOptions.find((item) => item.value === value)?.label || value || '-'

const renderCharts = async () => {
  await nextTick()

  if (chartModelRef.value) {
    if (chartModel) chartModel.dispose()
    chartModel = echarts.init(chartModelRef.value)
    chartModel.setOption({
      tooltip: { trigger: 'item', formatter: '{b}: {c} 积分 ({d}%)' },
      legend: { orient: 'vertical', right: 10, top: 'center' },
      series: [
        {
          type: 'pie',
          radius: ['40%', '70%'],
          avoidLabelOverlap: false,
          itemStyle: { borderRadius: 10, borderColor: '#fff', borderWidth: 2 },
          label: { show: false },
          emphasis: { label: { show: true, fontSize: 14, fontWeight: 'bold' } },
          labelLine: { show: false },
          data: modelDistribution.value
        }
      ]
    })
  }

  if (chartTimelineRef.value) {
    if (chartTimeline) chartTimeline.dispose()
    chartTimeline = echarts.init(chartTimelineRef.value)
    chartTimeline.setOption({
      tooltip: { trigger: 'axis' },
      grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: timelineLabels.value
      },
      yAxis: { type: 'value', name: '积分' },
      series: [
        {
          name: '消耗',
          type: 'line',
          smooth: true,
          areaStyle: { opacity: 0.3 },
          lineStyle: { width: 3 },
          emphasis: { focus: 'series' },
          data: timelineValues.value
        }
      ]
    })
  }

  if (chartTokenRef.value) {
    if (chartToken) chartToken.dispose()
    chartToken = echarts.init(chartTokenRef.value)
    chartToken.setOption({
      tooltip: { trigger: 'axis' },
      legend: { data: ['输入 Token', '输出 Token'], bottom: 0 },
      grid: { left: '3%', right: '4%', bottom: '12%', top: '8%', containLabel: true },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: timelineLabels.value
      },
      yAxis: { type: 'value', name: 'Token' },
      series: [
        {
          name: '输入 Token',
          type: 'line',
          smooth: true,
          stack: 'tokens',
          areaStyle: { opacity: 0.3 },
          lineStyle: { width: 2 },
          itemStyle: { color: '#3b82f6' },
          emphasis: { focus: 'series' },
          data: timelinePromptTokens.value
        },
        {
          name: '输出 Token',
          type: 'line',
          smooth: true,
          stack: 'tokens',
          areaStyle: { opacity: 0.3 },
          lineStyle: { width: 2 },
          itemStyle: { color: '#f59e0b' },
          emphasis: { focus: 'series' },
          data: timelineCompletionTokens.value
        }
      ]
    })
  }
}

const formatDate = (value) => {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

const handleResize = () => {
  chartModel?.resize()
  chartTimeline?.resize()
  chartToken?.resize()
}

watch(usageLogs, () => {
  renderCharts()
})

onMounted(() => {
  fetchAllData()
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  chartModel?.dispose()
  chartTimeline?.dispose()
  chartToken?.dispose()
})
</script>

<template>
  <div class="space-y-6">
    <!-- Header Card -->
    <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 class="text-2xl font-black text-slate-800 tracking-tight">工作台</h1>
          <p class="text-slate-400 text-sm font-medium mt-1">账户余额与 AI 调用概览，模型分布和近 7 天趋势基于最近 {{ LOG_LIMIT }} 条调用日志</p>
        </div>
        <div class="flex items-center gap-3">
          <el-select v-model="requestSource" placeholder="全部来源" style="width: 140px" @change="fetchUsageData">
            <el-option
              v-for="item in requestSourceOptions"
              :key="item.value"
              :label="item.label"
              :value="item.value"
            />
          </el-select>
          <el-button type="primary" class="rounded-2xl! font-bold" :loading="loading" @click="fetchAllData">
            <template #icon><el-icon><Refresh /></el-icon></template>
            刷新
          </el-button>
        </div>
      </div>
    </div>

    <!-- Balance Cards -->
    <div class="grid grid-cols-3 gap-4">
      <div class="bg-white rounded-2xl border border-slate-50 shadow-soft p-5 flex items-center gap-4">
        <div class="w-12 h-12 rounded-xl bg-cyan-100 flex items-center justify-center shrink-0">
          <el-icon class="text-cyan-500" :size="20"><Coin /></el-icon>
        </div>
        <div class="min-w-0">
          <p class="text-xs text-slate-400 mb-1">总积分</p>
          <p class="text-2xl font-bold text-slate-800 truncate">{{ balanceInfo.totalCredits.toLocaleString() }}</p>
        </div>
      </div>
      <div class="bg-white rounded-2xl border border-slate-50 shadow-soft p-5 flex items-center gap-4">
        <div class="w-12 h-12 rounded-xl bg-amber-100 flex items-center justify-center shrink-0">
          <el-icon class="text-amber-500" :size="20"><Lock /></el-icon>
        </div>
        <div class="min-w-0">
          <p class="text-xs text-slate-400 mb-1">冻结积分</p>
          <p class="text-2xl font-bold text-slate-800 truncate">{{ balanceInfo.frozenCredits.toLocaleString() }}</p>
        </div>
      </div>
      <div class="bg-white rounded-2xl border border-slate-50 shadow-soft p-5 flex items-center gap-4">
        <div class="w-12 h-12 rounded-xl bg-emerald-100 flex items-center justify-center shrink-0">
          <el-icon class="text-emerald-500" :size="20"><Check /></el-icon>
        </div>
        <div class="min-w-0">
          <p class="text-xs text-slate-400 mb-1">可用积分</p>
          <p class="text-2xl font-bold text-slate-800 truncate">{{ balanceInfo.availableCredits.toLocaleString() }}</p>
        </div>
      </div>
    </div>

    <!-- Usage Stats Cards -->
    <div class="grid grid-cols-4 gap-4">
      <div class="bg-white rounded-2xl border border-slate-50 shadow-soft p-5 flex items-center gap-4">
        <div class="w-12 h-12 rounded-xl bg-blue-100 flex items-center justify-center shrink-0">
          <el-icon class="text-blue-500" :size="20"><Wallet /></el-icon>
        </div>
        <div class="min-w-0">
          <p class="text-xs text-slate-400 mb-1">总消耗积分</p>
          <p class="text-2xl font-bold text-slate-800 truncate">{{ formatCredits(summary?.total_user_cost || 0) }}</p>
        </div>
      </div>
      <div class="bg-white rounded-2xl border border-slate-50 shadow-soft p-5 flex items-center gap-4">
        <div class="w-12 h-12 rounded-xl bg-sky-100 flex items-center justify-center shrink-0">
          <el-icon class="text-sky-500" :size="20"><Upload /></el-icon>
        </div>
        <div class="min-w-0">
          <p class="text-xs text-slate-400 mb-1">输入 Token</p>
          <p class="text-2xl font-bold text-slate-800 truncate">{{ formatCredits(summary?.total_prompt_tokens || 0) }}</p>
        </div>
      </div>
      <div class="bg-white rounded-2xl border border-slate-50 shadow-soft p-5 flex items-center gap-4">
        <div class="w-12 h-12 rounded-xl bg-amber-100 flex items-center justify-center shrink-0">
          <el-icon class="text-amber-500" :size="20"><Download /></el-icon>
        </div>
        <div class="min-w-0">
          <p class="text-xs text-slate-400 mb-1">输出 Token</p>
          <p class="text-2xl font-bold text-slate-800 truncate">{{ formatCredits(summary?.total_completion_tokens || 0) }}</p>
        </div>
      </div>
      <div class="bg-white rounded-2xl border border-slate-50 shadow-soft p-5 flex items-center gap-4">
        <div class="w-12 h-12 rounded-xl bg-purple-100 flex items-center justify-center shrink-0">
          <el-icon class="text-purple-500" :size="20"><TrendCharts /></el-icon>
        </div>
        <div class="min-w-0">
          <p class="text-xs text-slate-400 mb-1">总请求次数</p>
          <p class="text-2xl font-bold text-slate-800 truncate">{{ summary?.request_count || 0 }}</p>
        </div>
      </div>
    </div>

    <!-- Charts -->
    <div class="grid grid-cols-3 gap-4">
      <div class="bg-white rounded-2xl border border-slate-50 shadow-soft p-6">
        <h3 class="text-base font-bold text-slate-800 mb-4">模型消耗分布</h3>
        <div ref="chartModelRef" style="height: 280px; width: 100%"></div>
      </div>
      <div class="bg-white rounded-2xl border border-slate-50 shadow-soft p-6">
        <h3 class="text-base font-bold text-slate-800 mb-4">近 7 天消耗趋势</h3>
        <div ref="chartTimelineRef" style="height: 280px; width: 100%"></div>
      </div>
      <div class="bg-white rounded-2xl border border-slate-50 shadow-soft p-6">
        <h3 class="text-base font-bold text-slate-800 mb-4">近 7 天 Token 趋势</h3>
        <div ref="chartTokenRef" style="height: 280px; width: 100%"></div>
      </div>
    </div>

    <!-- Logs Table -->
    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft p-6">
      <h3 class="text-base font-bold text-slate-800 mb-4">使用记录</h3>
      <el-table :data="usageLogs" v-loading="loading" stripe>
        <el-table-column prop="model_code" label="模型" min-width="140" />
        <el-table-column label="消耗积分" min-width="100">
          <template #default="{ row }">
            {{ formatCredits(row.user_cost || 0) }}
          </template>
        </el-table-column>
        <el-table-column label="输入 Token" min-width="90">
          <template #default="{ row }">
            {{ formatCredits(row.prompt_tokens || 0) }}
          </template>
        </el-table-column>
        <el-table-column label="输出 Token" min-width="90">
          <template #default="{ row }">
            {{ formatCredits(row.completion_tokens || 0) }}
          </template>
        </el-table-column>
        <el-table-column prop="request_status" label="状态" width="100" />
        <el-table-column label="来源" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="row.request_source === 'web_chat' ? 'success' : 'info'">
              {{ requestSourceLabel(row.request_source) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="request_id" label="请求 ID" min-width="160" show-overflow-tooltip />
        <el-table-column label="时间" min-width="160">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>
