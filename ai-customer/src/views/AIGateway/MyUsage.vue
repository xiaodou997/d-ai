<script setup>
import { computed, nextTick, onMounted, onUnmounted, shallowRef, watch } from 'vue'
import { Refresh, Wallet, DataLine, TrendCharts } from '@element-plus/icons-vue'
import { listMyUsageLogs, getMyUsageSummary, formatCredits } from '@/api/aiGateway'
import * as echarts from 'echarts'

const LOG_LIMIT = 500

const loading = shallowRef(false)
const usageLogs = shallowRef([])
const summary = shallowRef(null)

const chartModelRef = shallowRef(null)
const chartTimelineRef = shallowRef(null)
let chartModel = null
let chartTimeline = null

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

const fetchUsageData = async () => {
  loading.value = true
  try {
    const [logsRes, summaryRes] = await Promise.all([
      listMyUsageLogs({ limit: LOG_LIMIT }),
      getMyUsageSummary()
    ])
    usageLogs.value = logsRes || []
    summary.value = summaryRes || null
  } finally {
    loading.value = false
  }
}

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
}

const formatDate = (value) => {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

const handleResize = () => {
  chartModel?.resize()
  chartTimeline?.resize()
}

watch(usageLogs, () => {
  renderCharts()
})

onMounted(() => {
  fetchUsageData()
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  chartModel?.dispose()
  chartTimeline?.dispose()
})
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div class="page-title">
        <p class="eyebrow">Usage Analytics</p>
        <h1>使用统计</h1>
        <p>查看个人 AI 调用消耗、模型分布和近 7 天趋势，图表基于最近 {{ LOG_LIMIT }} 条调用日志。</p>
      </div>
      <el-button :icon="Refresh" @click="fetchUsageData" :loading="loading">刷新</el-button>
    </header>

    <main class="page-main">
      <section class="summary-stats mb-6">
        <div class="stats-grid">
          <div class="stat-card">
            <div class="stat-icon bg-blue-100">
              <el-icon class="text-blue-500" :size="20"><Wallet /></el-icon>
            </div>
            <div class="stat-content">
              <p class="stat-label">总消耗积分</p>
              <p class="stat-value">{{ formatCredits(summary?.total_user_cost || 0) }}</p>
            </div>
          </div>
          <div class="stat-card">
            <div class="stat-icon bg-green-100">
              <el-icon class="text-green-500" :size="20"><DataLine /></el-icon>
            </div>
            <div class="stat-content">
              <p class="stat-label">总 Token 数</p>
              <p class="stat-value">{{ formatCredits(summary?.total_tokens || 0) }}</p>
            </div>
          </div>
          <div class="stat-card">
            <div class="stat-icon bg-purple-100">
              <el-icon class="text-purple-500" :size="20"><TrendCharts /></el-icon>
            </div>
            <div class="stat-content">
              <p class="stat-label">总请求次数</p>
              <p class="stat-value">{{ summary?.request_count || 0 }}</p>
            </div>
          </div>
        </div>
      </section>

      <section class="charts-section mb-6">
        <div class="charts-grid">
          <div class="chart-card">
            <h3 class="chart-title">模型消耗分布</h3>
            <div ref="chartModelRef" class="chart" style="height: 280px"></div>
          </div>
          <div class="chart-card">
            <h3 class="chart-title">近 7 天消耗趋势</h3>
            <div ref="chartTimelineRef" class="chart" style="height: 280px"></div>
          </div>
        </div>
      </section>

      <section class="logs-panel">
        <h3 class="panel-title">使用记录</h3>
        <el-table :data="usageLogs" v-loading="loading" stripe>
          <el-table-column prop="model_code" label="模型" min-width="140" />
          <el-table-column label="消耗积分" min-width="100">
            <template #default="{ row }">
              {{ formatCredits(row.user_cost || 0) }}
            </template>
          </el-table-column>
          <el-table-column label="Token 数" min-width="100">
            <template #default="{ row }">
              {{ formatCredits(row.total_tokens || 0) }}
            </template>
          </el-table-column>
          <el-table-column prop="request_status" label="状态" width="100" />
          <el-table-column prop="request_id" label="请求 ID" min-width="160" show-overflow-tooltip />
          <el-table-column label="时间" min-width="160">
            <template #default="{ row }">
              {{ formatDate(row.created_at) }}
            </template>
          </el-table-column>
        </el-table>
      </section>
    </main>
  </div>
</template>

<style scoped>
.page-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: #f5f7fa;
}

.page-header {
  padding: 16px 24px;
  background: #ffffff;
  border-bottom: 1px solid #e4e7ed;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.page-title {
  display: flex;
  flex-direction: column;
}

.eyebrow {
  margin: 0 0 4px;
  color: #64748b;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0;
  text-transform: uppercase;
}

.page-title h1 {
  margin: 0;
  color: #111827;
  font-size: 22px;
  font-weight: 900;
}

.page-title p {
  margin: 4px 0 0;
  color: #64748b;
  font-size: 13px;
}

.page-main {
  padding: 24px;
  flex: 1;
  min-height: 0;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}

.stat-card {
  background: #ffffff;
  border-radius: 12px;
  padding: 20px;
  display: flex;
  align-items: center;
  gap: 16px;
  border: 1px solid #e4e7ed;
}

.stat-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.stat-content {
  flex: 1;
}

.stat-label {
  font-size: 12px;
  color: #909399;
  margin-bottom: 4px;
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: #303133;
}

.charts-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.chart-card {
  background: #ffffff;
  border-radius: 12px;
  padding: 20px;
  border: 1px solid #e4e7ed;
}

.chart-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  margin: 0 0 16px;
}

.chart {
  width: 100%;
}

.logs-panel {
  background: #ffffff;
  border-radius: 12px;
  padding: 20px;
  border: 1px solid #e4e7ed;
}

.panel-title {
  font-size: 16px;
  font-weight: 700;
  color: #303133;
  margin: 0 0 16px;
}
</style>
