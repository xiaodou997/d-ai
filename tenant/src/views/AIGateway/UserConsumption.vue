<script setup>
import { computed, nextTick, onMounted, onUnmounted, shallowRef, watch } from 'vue'
import { Refresh, TrendCharts } from '@element-plus/icons-vue'
import { listUsageLogs, formatCredits } from '@/api/aiGateway'
import { getUsers } from '@/api/tenant'
import * as echarts from 'echarts'

const LOG_LIMIT = 500

const loading = shallowRef(false)
const usageLogs = shallowRef([])
const users = shallowRef([])
const selectedUser = shallowRef(null)
const detailDialogVisible = shallowRef(false)

const chartModelRef = shallowRef(null)
const chartTimelineRef = shallowRef(null)
let chartModel = null
let chartTimeline = null

const normalizeUserId = (value) => {
  if (value === null || value === undefined) return ''
  return String(value)
}

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

const formatDayLabel = (dayKey) => {
  const date = new Date(`${dayKey}T00:00:00`)
  return date.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' })
}

const userStatsMap = computed(() => {
  const stats = new Map()

  for (const row of usageLogs.value) {
    const userId = normalizeUserId(row.user_id)
    if (!userId) continue

    if (!stats.has(userId)) {
      stats.set(userId, {
        totalCost: 0,
        totalTokens: 0,
        requestCount: 0,
        modelCosts: new Map(),
        timelineCosts: new Map()
      })
    }

    const item = stats.get(userId)
    const cost = Number(row.user_cost) || 0
    const tokens = Number(row.total_tokens) || 0
    const modelCode = row.model_code || 'unknown'
    const createdAt = row.created_at ? new Date(row.created_at) : null
    const dayKey = createdAt && !Number.isNaN(createdAt.getTime())
      ? createdAt.toISOString().slice(0, 10)
      : ''

    item.totalCost += cost
    item.totalTokens += tokens
    item.requestCount += 1
    item.modelCosts.set(modelCode, (item.modelCosts.get(modelCode) || 0) + cost)
    if (dayKey) {
      item.timelineCosts.set(dayKey, (item.timelineCosts.get(dayKey) || 0) + cost)
    }
  }

  return stats
})

const selectedUserStats = computed(() => {
  const userId = normalizeUserId(selectedUser.value?.id)
  if (!userId) {
    return {
      totalCost: 0,
      totalTokens: 0,
      requestCount: 0,
      modelCosts: new Map(),
      timelineCosts: new Map()
    }
  }
  return userStatsMap.value.get(userId) || {
    totalCost: 0,
    totalTokens: 0,
    requestCount: 0,
    modelCosts: new Map(),
    timelineCosts: new Map()
  }
})

const modelDistribution = computed(() =>
  Array.from(selectedUserStats.value.modelCosts.entries())
    .map(([name, value]) => ({ name, value }))
    .sort((a, b) => b.value - a.value)
    .slice(0, 8)
)

const timelineDayKeys = computed(() => last7DayKeys())

const timelineLabels = computed(() => timelineDayKeys.value.map(formatDayLabel))

const timelineValues = computed(() =>
  timelineDayKeys.value.map((key) => selectedUserStats.value.timelineCosts.get(key) || 0)
)

const renderCharts = async () => {
  if (!detailDialogVisible.value || !selectedUser.value) return

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

const fetchUsageData = async () => {
  loading.value = true
  try {
    const [usersRes, logsRes] = await Promise.all([
      getUsers({ page: 1, size: 100 }),
      listUsageLogs({ limit: LOG_LIMIT })
    ])
    users.value = usersRes.records || []
    usageLogs.value = logsRes || []
  } finally {
    loading.value = false
  }
}

const getUserConsumption = (userId) => {
  const stats = userStatsMap.value.get(normalizeUserId(userId))
  if (!stats) {
    return { totalCost: 0, totalTokens: 0, requestCount: 0 }
  }
  return {
    totalCost: stats.totalCost,
    totalTokens: stats.totalTokens,
    requestCount: stats.requestCount
  }
}

const openUserDetail = (user) => {
  selectedUser.value = user
  detailDialogVisible.value = true
}

const statusTagType = (status) => {
  const map = {
    1: 'success',
    2: 'warning',
    3: 'danger',
    4: 'info'
  }
  return map[status] || 'info'
}

const statusLabel = (status) => {
  const map = {
    1: '正常',
    2: '禁用',
    3: '冻结',
    4: '级联停用'
  }
  return map[status] || String(status ?? '-')
}

const handleResize = () => {
  chartModel?.resize()
  chartTimeline?.resize()
}

watch([detailDialogVisible, selectedUser, usageLogs], () => {
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
        <p class="eyebrow">User Consumption</p>
        <h1>用户消耗统计</h1>
        <p>基于最近 {{ LOG_LIMIT }} 条租户 AI 调用日志，查看终端用户消耗和模型分布。</p>
      </div>
      <el-button :icon="Refresh" @click="fetchUsageData" :loading="loading">刷新</el-button>
    </header>

    <main class="page-main">
      <section class="list-panel">
        <el-table :data="users" v-loading="loading" stripe>
          <el-table-column prop="username" label="用户名" min-width="140" />
          <el-table-column prop="email" label="邮箱" min-width="180" />
          <el-table-column label="消耗积分" min-width="120">
            <template #default="{ row }">
              {{ formatCredits(getUserConsumption(row.id).totalCost) }}
            </template>
          </el-table-column>
          <el-table-column label="Token 数量" min-width="120">
            <template #default="{ row }">
              {{ formatCredits(getUserConsumption(row.id).totalTokens) }}
            </template>
          </el-table-column>
          <el-table-column label="请求次数" min-width="100">
            <template #default="{ row }">
              {{ getUserConsumption(row.id).requestCount }}
            </template>
          </el-table-column>
          <el-table-column prop="status" label="状态" min-width="90">
            <template #default="{ row }">
              <el-tag :type="statusTagType(row.status)" size="small">
                {{ statusLabel(row.status) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="100" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" :icon="TrendCharts" @click="openUserDetail(row)">详情</el-button>
            </template>
          </el-table-column>
        </el-table>
      </section>
    </main>

    <el-dialog v-model="detailDialogVisible" title="用户消耗详情" width="800px" append-to-body destroy-on-close>
      <template v-if="selectedUser">
        <div class="user-info mb-4">
          <p class="font-bold">{{ selectedUser.username }}</p>
          <p class="text-sm text-slate-500">{{ selectedUser.email }}</p>
          <p class="text-xs text-slate-400 mt-1">图表基于最近 {{ LOG_LIMIT }} 条租户调用日志聚合。</p>
        </div>

        <el-tabs>
          <el-tab-pane label="模型分布" name="model">
            <div class="chart-container">
              <div ref="chartModelRef" class="chart" style="height: 300px"></div>
            </div>
          </el-tab-pane>
          <el-tab-pane label="近 7 天趋势" name="timeline">
            <div class="chart-container">
              <div ref="chartTimelineRef" class="chart" style="height: 300px"></div>
            </div>
          </el-tab-pane>
        </el-tabs>

        <el-divider />

        <div class="summary-stats">
          <div class="stat-item">
            <span class="label">总消耗积分</span>
            <span class="value">{{ formatCredits(selectedUserStats.totalCost) }}</span>
          </div>
          <div class="stat-item">
            <span class="label">总 Token 数</span>
            <span class="value">{{ formatCredits(selectedUserStats.totalTokens) }}</span>
          </div>
          <div class="stat-item">
            <span class="label">总请求次数</span>
            <span class="value">{{ selectedUserStats.requestCount }}</span>
          </div>
        </div>
      </template>
    </el-dialog>
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

.list-panel {
  background: #ffffff;
  border-radius: 8px;
  padding: 16px;
}

.user-info {
  padding: 8px 0;
}

.chart-container {
  width: 100%;
}

.chart {
  width: 100%;
}

.summary-stats {
  display: flex;
  gap: 24px;
}

.stat-item {
  flex: 1;
  text-align: center;
}

.label {
  display: block;
  font-size: 12px;
  color: #909399;
  margin-bottom: 4px;
}

.value {
  font-size: 20px;
  font-weight: 700;
  color: #303133;
}
</style>
