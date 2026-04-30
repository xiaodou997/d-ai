<script setup>
import { onMounted, shallowRef, ref } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { listMyUsageLogs, getMyUsageSummary, formatCredits } from '@/api/aiGateway'
import * as echarts from 'echarts'

const loading = shallowRef(false)
const usageLogs = shallowRef([])
const summary = shallowRef(null)

const chartModelRef = shallowRef(null)
const chartTimelineRef = shallowRef(null)
let chartModel = null
let chartTimeline = null

const fetchUsageData = async () => {
  loading.value = true
  try {
    const [logsRes, summaryRes] = await Promise.all([
      listMyUsageLogs(),
      getMyUsageSummary()
    ])
    usageLogs.value = logsRes.data || []
    summary.value = summaryRes.data || null
    initCharts()
  } finally {
    loading.value = false
  }
}

const initCharts = () => {
  // 模型分布饼图
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
          data: getModelDistribution()
        }
      ]
    })
  }

  // 时间趋势折线图
  if (chartTimelineRef.value) {
    if (chartTimeline) chartTimeline.dispose()
    chartTimeline = echarts.init(chartTimelineRef.value)
    chartTimeline.setOption({
      tooltip: { trigger: 'axis' },
      grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: getTimelineDates()
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
          data: getTimelineValues()
        }
      ]
    })
  }
}

const getModelDistribution = () => {
  // 模拟数据，实际需要从 API 获取模型分布
  return [
    { value: 1048, name: 'gpt-4' },
    { value: 735, name: 'gpt-3.5-turbo' },
    { value: 580, name: 'claude-3' }
  ]
}

const getTimelineDates = () => {
  const dates = []
  for (let i = 6; i >= 0; i--) {
    const d = new Date()
    d.setDate(d.getDate() - i)
    dates.push(d.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' }))
  }
  return dates
}

const getTimelineValues = () => {
  return [120, 132, 101, 134, 90, 230, 210]
}

const formatDate = (value) => {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

onMounted(fetchUsageData)

window.addEventListener('resize', () => {
  chartModel?.resize()
  chartTimeline?.resize()
})
</script>

<template>
  <div class="page-container">
    <!-- Header -->
    <header class="page-header">
      <div class="page-title">
        <p class="eyebrow">Usage Analytics</p>
        <h1>使用统计</h1>
        <p>查看个人 AI 调用消耗、模型分布和时间趋势分析</p>
      </div>
      <el-button :icon="Refresh" @click="fetchUsageData" :loading="loading">刷新</el-button>
    </header>

    <!-- Content -->
    <main class="page-main">
      <!-- Summary Stats -->
      <section class="summary-stats mb-6">
        <div class="stats-grid">
          <div class="stat-card">
            <div class="stat-icon bg-blue-100">
              <el-icon class="text-blue-500" :size="20"><Wallet /></el-icon>
            </div>
            <div class="stat-content">
              <p class="stat-label">总消耗积分</p>
              <p class="stat-value">{{ formatCredits(summary?.total_cost || 0) }}</p>
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

      <!-- Charts -->
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

      <!-- Usage Logs Table -->
      <section class="logs-panel">
        <h3 class="panel-title">使用记录</h3>
        <el-table :data="usageLogs" v-loading="loading" stripe>
          <el-table-column prop="model_code" label="模型" min-width="140" />
          <el-table-column label="消耗积分" min-width="100">
            <template #default="{ row }">
              {{ formatCredits(row.cost || 0) }}
            </template>
          </el-table-column>
          <el-table-column label="Token 数" min-width="100">
            <template #default="{ row }">
              {{ formatCredits(row.total_tokens || 0) }}
            </template>
          </el-table-column>
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

<script>
import { Wallet, DataLine, TrendCharts } from '@element-plus/icons-vue'
export default {
  components: { Wallet, DataLine, TrendCharts }
}
</script>

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
  margin-bottom: 16px;
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
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 16px;
}
</style>