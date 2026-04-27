<template>
  <div class="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">

    <!-- 顶部：标题 + 时间筛选 + 刷新 -->
    <div class="bg-white p-6 rounded-3xl border border-slate-50 shadow-soft flex flex-col lg:flex-row lg:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-black text-slate-800 tracking-tight flex items-center gap-3">
          URM 系统概览
          <span class="px-2 py-1 bg-primary-50 text-primary-600 text-[10px] font-black uppercase rounded-2xl tracking-widest">Live</span>
        </h1>
        <p class="text-slate-400 text-sm font-medium mt-1">实时资产计费监控看板</p>
      </div>
      <div class="flex items-center gap-3">
        <!-- 时间段选择 -->
        <div class="flex items-center bg-slate-50 rounded-2xl p-1 gap-1">
          <button
            v-for="opt in DAY_OPTIONS" :key="opt.value"
            @click="handleDaysChange(opt.value)"
            class="px-3 py-1.5 rounded-xl text-xs font-bold transition-all"
            :class="selectedDays === opt.value
              ? 'bg-white text-primary-600 shadow-sm'
              : 'text-slate-400 hover:text-slate-600'"
          >{{ opt.label }}</button>
        </div>
        <!-- 自动刷新状态 -->
        <div class="flex items-center bg-slate-50 rounded-2xl px-4 py-2 border border-slate-100">
          <div class="w-2 h-2 rounded-full mr-2 bg-emerald-500 animate-pulse"></div>
          <span class="text-xs font-bold text-slate-500">{{ countdown }}s</span>
        </div>
        <el-button type="primary" class="!rounded-2xl !px-5 font-bold" :loading="loading" @click="fetchAll">
          <el-icon class="mr-1"><Refresh /></el-icon>刷新
        </el-button>
      </div>
    </div>

    <!-- ① 平台 → 租户（平台积分体系） -->
    <div class="space-y-3">
      <div class="flex items-center gap-2 px-1">
        <div class="w-1 h-4 bg-indigo-500 rounded-full"></div>
        <span class="text-xs font-black text-slate-500 uppercase tracking-widest">平台 → 租户（平台积分体系）</span>
        <span class="text-[10px] text-slate-300 font-medium">充值类数据为{{ periodLabel }}汇总，余额为当前实时值</span>
      </div>
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <div class="bg-white p-5 rounded-3xl border border-slate-50 shadow-soft">
          <p class="text-[10px] font-black text-slate-400 uppercase tracking-widest mb-2">租户充值金额</p>
          <h3 class="text-2xl font-black text-slate-800 tracking-tighter">
            ¥ {{ fmtYuan(stats.tenantRechargeAmount) }}
          </h3>
          <p class="text-[10px] text-indigo-500 font-bold mt-2">{{ periodLabel }}平台向租户实收</p>
        </div>
        <div class="bg-white p-5 rounded-3xl border border-slate-50 shadow-soft">
          <p class="text-[10px] font-black text-slate-400 uppercase tracking-widest mb-2">租户充值积分</p>
          <h3 class="text-2xl font-black text-slate-800 tracking-tighter">
            {{ fmtNum(stats.tenantRechargeCredits) }} <span class="text-sm text-slate-400">积分</span>
          </h3>
          <p class="text-[10px] text-indigo-500 font-bold mt-2">{{ periodLabel }}向租户发放</p>
        </div>
        <div class="bg-white p-5 rounded-3xl border border-slate-50 shadow-soft">
          <p class="text-[10px] font-black text-slate-400 uppercase tracking-widest mb-2">活跃租户数</p>
          <h3 class="text-2xl font-black text-slate-800 tracking-tighter">
            {{ stats.activeTenants }} <span class="text-sm text-slate-400">个</span>
          </h3>
          <p class="text-[10px] text-slate-400 font-bold mt-2">当前状态正常的租户</p>
        </div>
        <div class="bg-white p-5 rounded-3xl border border-slate-50 shadow-soft">
          <p class="text-[10px] font-black text-slate-400 uppercase tracking-widest mb-2">租户积分余额</p>
          <h3 class="text-2xl font-black text-indigo-600 tracking-tighter">
            {{ fmtNum(stats.tenantTotalCredits) }} <span class="text-sm text-slate-400">积分</span>
          </h3>
          <p class="text-[10px] text-slate-400 font-bold mt-2">所有租户当前余额合计</p>
        </div>
      </div>
    </div>

    <!-- ② 租户 → 用户（用户积分体系） -->
    <div class="space-y-3">
      <div class="flex items-center gap-2 px-1">
        <div class="w-1 h-4 bg-emerald-500 rounded-full"></div>
        <span class="text-xs font-black text-slate-500 uppercase tracking-widest">租户 → 用户（用户积分体系）</span>
        <span class="text-[10px] text-slate-300 font-medium">充值类数据为{{ periodLabel }}汇总，余额为当前实时值</span>
      </div>
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <div class="bg-white p-5 rounded-3xl border border-slate-50 shadow-soft">
          <p class="text-[10px] font-black text-slate-400 uppercase tracking-widest mb-2">用户充值金额</p>
          <h3 class="text-2xl font-black text-slate-800 tracking-tighter">
            ¥ {{ fmtYuan(stats.userRechargeAmount) }}
          </h3>
          <p class="text-[10px] text-emerald-500 font-bold mt-2">{{ periodLabel }}租户向用户实收</p>
        </div>
        <div class="bg-white p-5 rounded-3xl border border-slate-50 shadow-soft">
          <p class="text-[10px] font-black text-slate-400 uppercase tracking-widest mb-2">用户充值积分</p>
          <h3 class="text-2xl font-black text-slate-800 tracking-tighter">
            {{ fmtNum(stats.userRechargeCredits) }} <span class="text-sm text-slate-400">积分</span>
          </h3>
          <p class="text-[10px] text-emerald-500 font-bold mt-2">{{ periodLabel }}向用户发放</p>
        </div>
        <div class="bg-white p-5 rounded-3xl border border-slate-50 shadow-soft">
          <p class="text-[10px] font-black text-slate-400 uppercase tracking-widest mb-2">新增终端用户</p>
          <h3 class="text-2xl font-black text-slate-800 tracking-tighter">
            {{ fmtNum(stats.newUsers) }} <span class="text-sm text-slate-400">名</span>
          </h3>
          <p class="text-[10px] text-emerald-500 font-bold mt-2">{{ periodLabel }}注册用户数</p>
        </div>
        <div class="bg-white p-5 rounded-3xl border border-slate-50 shadow-soft">
          <p class="text-[10px] font-black text-slate-400 uppercase tracking-widest mb-2">用户积分余额</p>
          <h3 class="text-2xl font-black text-emerald-600 tracking-tighter">
            {{ fmtNum(stats.userTotalCredits) }} <span class="text-sm text-slate-400">积分</span>
          </h3>
          <p class="text-[10px] text-slate-400 font-bold mt-2">所有用户当前余额合计</p>
        </div>
      </div>
    </div>

    <!-- ③ 消费趋势 + 运营告警 -->
    <div class="grid grid-cols-12 gap-6">
      <!-- 消费趋势图 -->
      <div class="col-span-12 lg:col-span-8">
        <div class="bg-white p-8 rounded-2xl border border-slate-50 shadow-soft">
          <div class="flex items-center justify-between mb-6">
            <div>
              <h3 class="text-base font-bold text-slate-800">资产消费趋势</h3>
              <p class="text-xs text-slate-400 mt-0.5">{{ periodLabel }}业务流水扣减曲线</p>
            </div>
            <div class="flex items-center gap-2">
              <el-select v-model="trendAccountType" style="width:110px" class="modern-select" size="small" @change="fetchTrend">
                <el-option label="全部" value="" />
                <el-option label="租户" value="1" />
                <el-option label="终端用户" value="2" />
              </el-select>
            </div>
          </div>
          <div v-loading="trendLoading">
            <div ref="trendChartRef" style="width:100%;height:320px" />
          </div>
        </div>
      </div>

      <!-- 运营告警 -->
      <div class="col-span-12 lg:col-span-4">
        <AlertList
          class="h-full"
          :timeout-pre-auths="alerts.timeoutPreAuths"
          :failed-transactions="alerts.failedTransactions"
        />
      </div>
    </div>

    <!-- ④ 业务系统消耗分布 -->
    <div class="bg-white p-8 rounded-2xl border border-slate-50 shadow-soft">
      <div class="flex items-center justify-between mb-6">
        <div>
          <h3 class="text-base font-bold text-slate-800">业务系统消耗分布</h3>
          <p class="text-xs text-slate-400 mt-0.5">{{ periodLabel }}各业务系统消耗分布</p>
        </div>
      </div>
      <div v-loading="resourceLoading" class="grid grid-cols-1 lg:grid-cols-12 gap-8 items-center">
        <div ref="resourceChartRef" class="lg:col-span-7" style="height:360px" />
        <div class="lg:col-span-5 space-y-3">
          <div
            v-for="(item, index) in resourceData?.resources"
            :key="item.appKey || item.appName"
            class="p-4 rounded-2xl bg-slate-50 flex items-center justify-between"
          >
            <div class="flex items-center gap-3">
              <span class="w-3 h-3 rounded-full flex-shrink-0" :style="{ backgroundColor: COLORS[index % COLORS.length] }"></span>
              <span class="text-sm font-bold text-slate-700">{{ item.appName || '—' }}</span>
            </div>
            <div class="text-right">
              <p class="text-sm font-black text-slate-800">{{ fmtNum(item.credits) }} <span class="text-xs text-slate-400">积分</span></p>
              <p class="text-[10px] text-slate-400 font-bold">{{ item.percentage }}%</p>
            </div>
          </div>
          <div v-if="!resourceData?.resources?.length" class="text-center text-xs text-slate-300 py-8">暂无消耗数据</div>
        </div>
      </div>
    </div>

  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import * as echarts from 'echarts'
import { getGlobalStats, getConsumptionTrend, getResourceStatistics } from '@/api/dashboard'
import { getDashboardAlerts } from '@/api/dashboard'
import AlertList from '@/components/AlertList.vue'

const COLORS = ['#6366f1', '#818cf8', '#a5b4fc', '#c7d2fe', '#e0e7ff', '#4f46e5', '#4338ca']

const DAY_OPTIONS = [
  { label: '近7天', value: 7 },
  { label: '近30天', value: 30 },
  { label: '近90天', value: 90 },
  { label: '全部', value: 0 },
]

const selectedDays = ref(30)
const trendAccountType = ref('')
const loading = ref(false)
const trendLoading = ref(false)
const resourceLoading = ref(false)

const stats = reactive({
  tenantRechargeAmount: 0,
  tenantRechargeCredits: 0,
  activeTenants: 0,
  tenantTotalCredits: 0,
  userRechargeAmount: 0,
  userRechargeCredits: 0,
  newUsers: 0,
  userTotalCredits: 0,
})

const alerts = reactive({
  timeoutPreAuths: [],
  failedTransactions: [],
})

const trendData = ref(null)
const resourceData = ref(null)

const trendChartRef = ref(null)
const resourceChartRef = ref(null)
let trendChart = null
let resourceChart = null

// ── 计算属性 ──────────────────────────────────────────
const periodLabel = computed(() => {
  const map = { 7: '近7天', 30: '近30天', 90: '近90天', 0: '全部' }
  return map[selectedDays.value] ?? '近30天'
})

const fmtYuan = (cents) =>
  ((cents || 0) / 100).toLocaleString('zh-CN', { minimumFractionDigits: 0, maximumFractionDigits: 0 })

const fmtNum = (n) => (n || 0).toLocaleString()

// ── 数据加载 ──────────────────────────────────────────
const fetchStats = async () => {
  const data = await getGlobalStats({ days: selectedDays.value })
  Object.assign(stats, data)
}

const fetchAlerts = async () => {
  try {
    const data = await getDashboardAlerts()
    alerts.timeoutPreAuths = Array.isArray(data?.timeoutPreAuths) ? data.timeoutPreAuths : []
    alerts.failedTransactions = Array.isArray(data?.failedTransactions) ? data.failedTransactions : []
  } catch {
    alerts.timeoutPreAuths = []
    alerts.failedTransactions = []
  }
}

const fetchTrend = async () => {
  trendLoading.value = true
  try {
    const params = { days: selectedDays.value }
    if (trendAccountType.value) params.accountType = trendAccountType.value
    const res = await getConsumptionTrend(params)
    trendData.value = res
    updateTrendChart(res)
  } finally {
    trendLoading.value = false
  }
}

const fetchResource = async () => {
  resourceLoading.value = true
  try {
    const res = await getResourceStatistics({ days: selectedDays.value })
    resourceData.value = res
    updateResourceChart(res)
  } finally {
    resourceLoading.value = false
  }
}

const fetchAll = async () => {
  loading.value = true
  try {
    await Promise.all([fetchStats(), fetchAlerts(), fetchTrend(), fetchResource()])
  } finally {
    loading.value = false
  }
}

const handleDaysChange = (val) => {
  selectedDays.value = val
  fetchAll()
}

// ── 图表 ──────────────────────────────────────────────
const updateTrendChart = (data) => {
  if (!trendChart) return
  trendChart.setOption({
    grid: { left: '2%', right: '2%', bottom: '3%', top: '8%', containLabel: true },
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(255,255,255,0.95)',
      borderRadius: 12,
      padding: 12,
      textStyle: { color: '#1e293b', fontWeight: 600, fontSize: 12 },
      shadowBlur: 10,
      shadowColor: 'rgba(0,0,0,0.05)',
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: (data?.dataPoints || []).map(p => p.timeLabel),
      axisLine: { lineStyle: { color: '#f1f5f9' } },
      axisLabel: { color: '#94a3b8', fontSize: 11, fontWeight: 600 },
    },
    yAxis: {
      type: 'value',
      splitLine: { lineStyle: { color: '#f1f5f9', type: 'dashed' } },
      axisLabel: { color: '#94a3b8', fontSize: 11, fontWeight: 600 },
    },
    series: [{
      type: 'line',
      smooth: 0.4,
      showSymbol: false,
      data: (data?.dataPoints || []).map(p => p.credits || 0),
      lineStyle: { width: 4, color: '#6366f1' },
      areaStyle: {
        color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
          { offset: 0, color: 'rgba(99,102,241,0.15)' },
          { offset: 1, color: 'rgba(99,102,241,0)' },
        ]),
      },
    }],
  })
}

const updateResourceChart = (data) => {
  if (!resourceChart) return
  resourceChart.setOption({
    color: COLORS,
    tooltip: { trigger: 'item' },
    series: [{
      type: 'pie',
      radius: ['50%', '80%'],
      center: ['50%', '50%'],
      avoidLabelOverlap: false,
      itemStyle: { borderRadius: 10, borderColor: '#fff', borderWidth: 4 },
      label: { show: false },
      data: (data?.resources || []).map(r => ({ name: r.appName, value: r.credits || 0 })),
    }],
  })
}

const handleResize = () => {
  trendChart?.resize()
  resourceChart?.resize()
}

// ── 自动刷新 ──────────────────────────────────────────
const countdown = ref(30)
let countdownTimer = null
let refreshTimer = null

const startAutoRefresh = () => {
  countdown.value = 30
  countdownTimer = setInterval(() => {
    countdown.value--
    if (countdown.value <= 0) countdown.value = 30
  }, 1000)
  refreshTimer = setInterval(fetchAll, 30000)
}

// ── 生命周期 ──────────────────────────────────────────
onMounted(() => {
  if (trendChartRef.value) trendChart = echarts.init(trendChartRef.value)
  if (resourceChartRef.value) resourceChart = echarts.init(resourceChartRef.value)
  fetchAll()
  startAutoRefresh()
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  clearInterval(countdownTimer)
  clearInterval(refreshTimer)
  window.removeEventListener('resize', handleResize)
  trendChart?.dispose()
  resourceChart?.dispose()
})
</script>

<style scoped>
:deep(.modern-select) .el-input__wrapper {
  border-radius: 12px !important;
  background-color: #f8fafc !important;
  box-shadow: none !important;
  border: 1px solid #f1f5f9 !important;
}
</style>
