<script setup>
import { onMounted, shallowRef, ref, computed } from 'vue'
import { Refresh, TrendCharts } from '@element-plus/icons-vue'
import { listUsageSummary, formatCredits } from '@/api/aiGateway'
import { getUsers } from '@/api/tenant'
import * as echarts from 'echarts'

const loading = shallowRef(false)
const usageData = shallowRef([])
const users = shallowRef([])
const selectedUser = shallowRef(null)
const detailDialogVisible = shallowRef(false)
const activeTab = ref('model')

const chartModelRef = shallowRef(null)
const chartTimelineRef = shallowRef(null)
let chartModel = null
let chartTimeline = null

const fetchUsageData = async () => {
  loading.value = true
  try {
    // 获取用户列表（URM）
    const usersRes = await getUsers({ page: 1, size: 100 })
    users.value = usersRes.records || []
    
    // 获取 AI 使用汇总（按用户维度）
    const usageRes = await listUsageSummary()
    usageData.value = usageRes || []
  } finally {
    loading.value = false
  }
}

const openUserDetail = (user) => {
  selectedUser.value = user
  detailDialogVisible.value = true
  // 延迟初始化图表
  setTimeout(() => {
    initCharts()
  }, 100)
}

const initCharts = () => {
  // 模型分布饼图
  if (chartModelRef.value) {
    if (chartModel) {
      chartModel.dispose()
    }
    chartModel = echarts.init(chartModelRef.value)
    chartModel.setOption({
      tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
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
    if (chartTimeline) {
      chartTimeline.dispose()
    }
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
  if (!selectedUser.value) return []
  // 模拟数据，实际需要从 API 获取
  const userId = selectedUser.value.id
  const userUsage = usageData.value.filter((u) => u.user_id === userId)
  
  // 暂时返回模拟数据
  return [
    { value: 1048, name: 'gpt-4' },
    { value: 735, name: 'gpt-3.5-turbo' },
    { value: 580, name: 'claude-3' },
    { value: 484, name: 'deepseek-chat' }
  ]
}

const getTimelineDates = () => {
  // 近7天日期
  const dates = []
  for (let i = 6; i >= 0; i--) {
    const d = new Date()
    d.setDate(d.getDate() - i)
    dates.push(d.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' }))
  }
  return dates
}

const getTimelineValues = () => {
  // 模拟数据
  return [120, 132, 101, 134, 90, 230, 210]
}

const getUserConsumption = (userId) => {
  const data = usageData.value.find((u) => u.user_id === userId)
  if (data) {
    return {
      totalCost: data.total_cost || 0,
      totalTokens: data.total_tokens || 0,
      requestCount: data.request_count || 0
    }
  }
  return { totalCost: 0, totalTokens: 0, requestCount: 0 }
}

const formatDate = (value) => {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

onMounted(fetchUsageData)

// 窗口大小变化时重新渲染图表
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
        <p class="eyebrow">User Consumption</p>
        <h1>用户消耗统计</h1>
        <p>查看终端用户的 AI 调用消耗、模型分布和趋势分析</p>
      </div>
      <el-button :icon="Refresh" @click="fetchUsageData" :loading="loading">刷新</el-button>
    </header>

    <!-- Content -->
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
          <el-table-column prop="status" label="状态" min-width="80">
            <template #default="{ row }">
              <el-tag :type="row.status === 1 ? 'success' : 'danger'" size="small">
                {{ row.status === 1 ? '正常' : '停用' }}
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

    <!-- User Detail Dialog -->
    <el-dialog v-model="detailDialogVisible" title="用户消耗详情" width="800px" append-to-body destroy-on-close>
      <template v-if="selectedUser">
        <div class="user-info mb-4">
          <p class="font-bold">{{ selectedUser.username }}</p>
          <p class="text-sm text-slate-500">{{ selectedUser.email }}</p>
        </div>

        <el-tabs v-model="activeTab">
          <el-tab-pane label="模型分布" name="model">
            <div class="chart-container">
              <div ref="chartModelRef" class="chart" style="height: 300px"></div>
            </div>
          </el-tab-pane>
          <el-tab-pane label="时间趋势" name="timeline">
            <div class="chart-container">
              <div ref="chartTimelineRef" class="chart" style="height: 300px"></div>
            </div>
          </el-tab-pane>
        </el-tabs>

        <el-divider />

        <div class="summary-stats">
          <div class="stat-item">
            <span class="label">总消耗积分</span>
            <span class="value">{{ formatCredits(getUserConsumption(selectedUser.id).totalCost) }}</span>
          </div>
          <div class="stat-item">
            <span class="label">总 Token 数</span>
            <span class="value">{{ formatCredits(getUserConsumption(selectedUser.id).totalTokens) }}</span>
          </div>
          <div class="stat-item">
            <span class="label">总请求次数</span>
            <span class="value">{{ getUserConsumption(selectedUser.id).requestCount }}</span>
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
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.stat-item .label {
  font-size: 12px;
  color: #909399;
}

.stat-item .value {
  font-size: 18px;
  font-weight: 700;
  color: #303133;
}
</style>
