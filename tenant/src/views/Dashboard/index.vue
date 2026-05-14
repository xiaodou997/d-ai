<template>
  <div class="space-y-6">
    <!-- 顶部欢迎栏 -->
    <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-4 bg-white p-6 rounded-xl border border-slate-50 shadow-soft">
      <div>
        <h1 class="text-2xl font-black text-slate-800 tracking-tight flex items-center">
          租户概览
          <span class="ml-3 px-2 py-1 bg-primary-50 text-primary-600 text-[10px] font-black uppercase rounded-lg tracking-widest">Live</span>
        </h1>
        <p class="text-slate-400 text-sm font-medium mt-1">
          欢迎回来，{{ authStore.username }} — 这是您的租户实时数据面板
        </p>
      </div>
      <el-button
        type="primary"
        class="!rounded-2xl !px-6 font-bold"
        :loading="loading"
        @click="fetchData"
      >
        <template #icon><el-icon><Refresh /></el-icon></template>
        立即刷新
      </el-button>
    </div>

    <!-- 统计卡片（第一行：基础统计） -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      <!-- 终端用户数 -->
      <div class="bg-white rounded-2xl p-6 border border-slate-50 shadow-soft flex items-start gap-4 hover:-translate-y-1 transition-transform duration-300">
        <div class="w-12 h-12 rounded-2xl bg-primary-50 flex items-center justify-center flex-shrink-0">
          <el-icon class="text-primary-500" :size="24"><User /></el-icon>
        </div>
        <div>
          <p class="text-xs font-bold text-slate-400 uppercase tracking-widest mb-1">终端用户数</p>
          <p class="text-3xl font-black text-slate-800">
            <span v-if="overviewLoading" class="text-slate-200">—</span>
            <span v-else>{{ overview.endUserCount ?? 0 }}</span>
          </p>
          <p class="text-xs text-slate-400 mt-1">累计注册用户</p>
        </div>
      </div>

      <!-- 活跃用户数 -->
      <div class="bg-white rounded-2xl p-6 border border-slate-50 shadow-soft flex items-start gap-4 hover:-translate-y-1 transition-transform duration-300">
        <div class="w-12 h-12 rounded-2xl bg-emerald-50 flex items-center justify-center flex-shrink-0">
          <el-icon class="text-emerald-500" :size="24"><Avatar /></el-icon>
        </div>
        <div>
          <p class="text-xs font-bold text-slate-400 uppercase tracking-widest mb-1">活跃用户数</p>
          <p class="text-3xl font-black text-slate-800">
            <span v-if="overviewLoading" class="text-slate-200">—</span>
            <span v-else class="text-emerald-600">{{ overview.activeUserCount ?? 0 }}</span>
          </p>
          <p class="text-xs text-slate-400 mt-1">30天内有交易的用户</p>
        </div>
      </div>

      <!-- 邀请码数量 -->
      <div class="bg-white rounded-2xl p-6 border border-slate-50 shadow-soft flex items-start gap-4 hover:-translate-y-1 transition-transform duration-300">
        <div class="w-12 h-12 rounded-2xl bg-indigo-50 flex items-center justify-center flex-shrink-0">
          <el-icon class="text-indigo-500" :size="24"><Ticket /></el-icon>
        </div>
        <div>
          <p class="text-xs font-bold text-slate-400 uppercase tracking-widest mb-1">邀请码数量</p>
          <p class="text-3xl font-black text-slate-800">
            <span v-if="overviewLoading" class="text-slate-200">—</span>
            <span v-else>{{ overview.inviteCodeCount ?? 0 }}</span>
          </p>
          <p class="text-xs text-slate-400 mt-1">已创建邀请码</p>
        </div>
      </div>
    </div>

    <!-- 统计卡片（第二行：积分相关） -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      <!-- 租户积分余额 -->
      <div class="bg-white rounded-2xl p-6 border border-slate-50 shadow-soft flex items-start gap-4 hover:-translate-y-1 transition-transform duration-300">
        <div class="w-12 h-12 rounded-2xl bg-amber-50 flex items-center justify-center flex-shrink-0">
          <el-icon class="text-amber-500" :size="24"><Coin /></el-icon>
        </div>
        <div>
          <p class="text-xs font-bold text-slate-400 uppercase tracking-widest mb-1">租户积分余额</p>
          <p class="text-3xl font-black text-slate-800">
            <span v-if="balanceLoading" class="text-slate-200">—</span>
            <span v-else class="text-amber-600">{{ balance.availableCredits ?? 0 }}</span>
          </p>
          <p class="text-xs text-slate-400 mt-1">可用积分（冻结 {{ balance.frozenCredits ?? 0 }}）</p>
        </div>
      </div>

      <!-- 用户总积分 -->
      <div class="bg-white rounded-2xl p-6 border border-slate-50 shadow-soft flex items-start gap-4 hover:-translate-y-1 transition-transform duration-300">
        <div class="w-12 h-12 rounded-2xl bg-sky-50 flex items-center justify-center flex-shrink-0">
          <el-icon class="text-sky-500" :size="24"><Wallet /></el-icon>
        </div>
        <div>
          <p class="text-xs font-bold text-slate-400 uppercase tracking-widest mb-1">用户总积分</p>
          <p class="text-3xl font-black text-slate-800">
            <span v-if="overviewLoading" class="text-slate-200">—</span>
            <span v-else class="text-sky-600">{{ overview.userTotalCredits ?? 0 }}</span>
          </p>
          <p class="text-xs text-slate-400 mt-1">所有用户的积分余额总和</p>
        </div>
      </div>

      <!-- 总扣费积分 -->
      <div class="bg-white rounded-2xl p-6 border border-slate-50 shadow-soft flex items-start gap-4 hover:-translate-y-1 transition-transform duration-300">
        <div class="w-12 h-12 rounded-2xl bg-rose-50 flex items-center justify-center flex-shrink-0">
          <el-icon class="text-rose-500" :size="24"><TrendCharts /></el-icon>
        </div>
        <div>
          <p class="text-xs font-bold text-slate-400 uppercase tracking-widest mb-1">总扣费积分</p>
          <p class="text-3xl font-black text-slate-800">
            <span v-if="overviewLoading" class="text-slate-200">—</span>
            <span v-else>{{ overview.totalDeduction ?? 0 }}</span>
          </p>
          <p class="text-xs text-slate-400 mt-1">历史累计扣费</p>
        </div>
      </div>
    </div>

    <!-- AI Gateway 统计区 -->
    <div class="bg-gradient-to-r from-blue-50 to-indigo-50 rounded-2xl border border-blue-100 p-6">
      <div class="flex items-center gap-3 mb-4">
        <el-icon class="text-blue-600" :size="20"><Box /></el-icon>
        <h2 class="text-base font-bold text-blue-800">AI Gateway 统计</h2>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        <!-- 授权模型数 -->
        <div class="bg-white rounded-xl p-4 border border-blue-50 flex items-start gap-3">
          <div class="w-10 h-10 rounded-xl bg-blue-100 flex items-center justify-center flex-shrink-0">
            <el-icon class="text-blue-500" :size="20"><Box /></el-icon>
          </div>
          <div>
            <p class="text-xs font-bold text-slate-400 mb-1">授权模型</p>
            <p class="text-2xl font-black text-blue-700">
              <span v-if="aiLoading" class="text-slate-200">—</span>
              <span v-else>{{ aiStats.modelCount ?? 0 }}</span>
            </p>
            <p class="text-xs text-slate-400 mt-1">平台已授权的模型</p>
          </div>
        </div>

        <!-- API Key 数 -->
        <div class="bg-white rounded-xl p-4 border border-blue-50 flex items-start gap-3">
          <div class="w-10 h-10 rounded-xl bg-green-100 flex items-center justify-center flex-shrink-0">
            <el-icon class="text-green-500" :size="20"><Key /></el-icon>
          </div>
          <div>
            <p class="text-xs font-bold text-slate-400 mb-1">租户 API Key</p>
            <p class="text-2xl font-black text-green-700">
              <span v-if="aiLoading" class="text-slate-200">—</span>
              <span v-else>{{ aiStats.apiKeyCount ?? 0 }}</span>
            </p>
            <p class="text-xs text-slate-400 mt-1">已创建的 API Key</p>
          </div>
        </div>

        <!-- 本月消耗 -->
        <div class="bg-white rounded-xl p-4 border border-blue-50 flex items-start gap-3">
          <div class="w-10 h-10 rounded-xl bg-purple-100 flex items-center justify-center flex-shrink-0">
            <el-icon class="text-purple-500" :size="20"><DataLine /></el-icon>
          </div>
          <div>
            <p class="text-xs font-bold text-slate-400 mb-1">本月 AI 消耗</p>
            <p class="text-2xl font-black text-purple-700">
              <span v-if="aiLoading" class="text-slate-200">—</span>
              <span v-else>{{ aiStats.monthCost ?? 0 }}</span>
            </p>
            <p class="text-xs text-slate-400 mt-1">积分 · 租户支付</p>
          </div>
        </div>
      </div>

      <div class="mt-4 flex justify-end">
        <el-button text type="primary" size="small" @click="$router.push('/ai/models')">
          查看 AI Gateway <el-icon class="ml-1"><ArrowRight /></el-icon>
        </el-button>
      </div>
    </div>

    <!-- APP 消耗分布 + 近期用户充值记录 -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <!-- APP 消耗分布饼状图 -->
      <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
        <div class="flex items-center justify-between p-6 border-b border-slate-50">
          <div>
            <h2 class="text-base font-bold text-slate-800">APP 消耗分布</h2>
            <p class="text-xs text-slate-400 mt-0.5">最近 30 天按应用系统的消耗占比</p>
          </div>
        </div>

        <div v-if="appLoading" class="flex items-center justify-center py-16">
          <el-icon class="text-slate-300 animate-spin" :size="32"><Loading /></el-icon>
        </div>

        <div v-else-if="appConsumption.length === 0" class="flex flex-col items-center justify-center py-16 text-slate-400">
          <el-icon :size="48"><PieChart /></el-icon>
          <p class="mt-4 text-sm">暂无消耗数据</p>
        </div>

        <div v-else class="p-4">
          <div ref="pieChartRef" style="height: 280px;"></div>
        </div>
      </div>

      <!-- 近期用户充值记录 -->
      <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
        <div class="flex items-center justify-between p-6 border-b border-slate-50">
          <div>
            <h2 class="text-base font-bold text-slate-800">近期用户充值记录</h2>
            <p class="text-xs text-slate-400 mt-0.5">给终端用户充值积分的历史（最近 5 条）</p>
          </div>
          <el-button
            text
            type="primary"
            class="!text-xs font-bold"
            @click="$router.push('/finance/user-recharge-records')"
          >
            查看全部 <el-icon class="ml-1"><ArrowRight /></el-icon>
          </el-button>
        </div>

        <div v-if="rechargeLoading" class="flex items-center justify-center py-12">
          <el-icon class="text-slate-300 animate-spin" :size="32"><Loading /></el-icon>
        </div>

        <el-table
          v-else
          :data="recentRecharges"
          empty-text="暂无充值记录"
          class="w-full"
        >
          <el-table-column prop="rechargeNo" label="充值单号" min-width="140" show-overflow-tooltip />
          <el-table-column prop="username" label="用户名" width="100">
            <template #default="{ row }">
              <span v-if="row.username">{{ row.username }}</span>
              <span v-else class="text-slate-300">—</span>
            </template>
          </el-table-column>
          <el-table-column label="到账积分" width="100">
            <template #default="{ row }">
              <span class="font-bold text-emerald-600">+{{ row.creditAmount?.toLocaleString() ?? 0 }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="createdTime" label="充值时间" width="150">
            <template #default="{ row }">
              {{ formatTime(row.createdTime) }}
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>

    <!-- 近期交易流水 -->
    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
      <div class="flex items-center justify-between p-6 border-b border-slate-50">
        <div>
          <h2 class="text-base font-bold text-slate-800">近期交易流水</h2>
          <p class="text-xs text-slate-400 mt-0.5">最近 10 条扣费流水记录</p>
        </div>
        <el-button
          text
          type="primary"
          class="!text-xs font-bold"
          @click="$router.push('/finance/transactions')"
        >
          查看全部 <el-icon class="ml-1"><ArrowRight /></el-icon>
        </el-button>
      </div>

      <div v-if="txLoading" class="flex items-center justify-center py-12">
        <el-icon class="text-slate-300 animate-spin" :size="32"><Loading /></el-icon>
      </div>

      <el-table
        v-else
        :data="recentTransactions"
        empty-text="暂无数据"
        class="w-full"
      >
        <el-table-column prop="transactionId" label="流水号" min-width="140" show-overflow-tooltip />
        <el-table-column prop="username" label="用户名" width="100">
          <template #default="{ row }">
            <span v-if="row.username">{{ row.username }}</span>
            <span v-else class="text-slate-300">—</span>
          </template>
        </el-table-column>
        <el-table-column label="租户积分" align="right" width="90">
          <template #default="{ row }">
            <span v-if="row.tenantCredits" class="font-bold text-rose-500">-{{ row.tenantCredits?.toLocaleString() }}</span>
            <span v-else class="text-slate-300">—</span>
          </template>
        </el-table-column>
        <el-table-column label="用户积分" align="right" width="90">
          <template #default="{ row }">
            <span v-if="row.userCredits" class="font-bold text-rose-500">-{{ row.userCredits?.toLocaleString() }}</span>
            <span v-else class="text-slate-300">—</span>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="描述" min-width="120" show-overflow-tooltip />
        <el-table-column label="状态" width="70">
          <template #default="{ row }">
            <div class="flex items-center">
              <span class="w-1.5 h-1.5 rounded-full mr-1.5" :class="statusDotClass(row.status)"></span>
              <span class="text-xs font-bold">{{ statusText(row.status) }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="createdTime" label="时间" width="150">
          <template #default="{ row }">
            {{ formatTime(row.createdTime) }}
          </template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onUnmounted, watch } from 'vue'
import { Refresh, User, Avatar, Ticket, Coin, Wallet, TrendCharts, ArrowRight, Loading, PieChart, Box, Key, DataLine } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { getAnalyticsOverview, getAppConsumption, getAccountBalance, getTransactions, getUserRechargeRecords } from '@/api/tenant'
import { listTenantModelGrants, listTenantAPIKeys, getDashboardSummary } from '@/api/aiGateway'
import * as echarts from 'echarts'
import dayjs from 'dayjs'

const authStore = useAuthStore()
const loading = ref(false)
const overviewLoading = ref(false)
const balanceLoading = ref(false)
const txLoading = ref(false)
const rechargeLoading = ref(false)
const appLoading = ref(false)
const aiLoading = ref(false)

const overview = reactive({
  endUserCount: 0,
  inviteCodeCount: 0,
  totalDeduction: 0,
  userTotalCredits: 0,
  activeUserCount: 0
})

const balance = reactive({
  totalCredits: 0,
  frozenCredits: 0,
  availableCredits: 0
})

const recentTransactions = ref([])
const recentRecharges = ref([])
const appConsumption = ref([])

const aiStats = reactive({
  modelCount: 0,
  apiKeyCount: 0,
  monthCost: 0
})

const pieChartRef = ref(null)
let pieChartInstance = null

const formatTime = (ts) => {
  if (!ts) return '—'
  return dayjs(ts).format('YYYY-MM-DD HH:mm:ss')
}

const statusText = (s) => ({ 0: '进行中', 1: '成功', 2: '取消', 3: '退款', 4: '已释放' }[s] ?? '—')
const statusDotClass = (s) => s === 1 ? 'bg-emerald-500' : s === 0 ? 'bg-amber-400' : 'bg-rose-500'

const fetchOverview = async () => {
  overviewLoading.value = true
  try {
    const data = await getAnalyticsOverview()
    if (data) Object.assign(overview, data)
  } catch (e) {
    console.error('获取统计数据失败:', e)
  } finally {
    overviewLoading.value = false
  }
}

const fetchBalance = async () => {
  balanceLoading.value = true
  try {
    const data = await getAccountBalance(false)
    if (data) {
      balance.totalCredits = data.totalCredits ?? 0
      balance.frozenCredits = data.frozenCredits ?? 0
      balance.availableCredits = data.availableCredits ?? 0
    }
  } catch (e) {
    console.error('获取余额失败:', e)
  } finally {
    balanceLoading.value = false
  }
}

const fetchTransactions = async () => {
  txLoading.value = true
  try {
    const res = await getTransactions({ page: 1, size: 10 })
    recentTransactions.value = Array.isArray(res) ? res : (res?.list || res?.records || [])
  } catch (e) {
    console.error('获取流水数据失败:', e)
    recentTransactions.value = []
  } finally {
    txLoading.value = false
  }
}

const fetchRecharges = async () => {
  rechargeLoading.value = true
  try {
    const res = await getUserRechargeRecords({ page: 1, size: 5 })
    recentRecharges.value = Array.isArray(res) ? res : (res?.list || res?.records || [])
  } catch (e) {
    console.error('获取充值记录失败:', e)
    recentRecharges.value = []
  } finally {
    rechargeLoading.value = false
  }
}

const fetchAppConsumption = async () => {
  appLoading.value = true
  try {
    const data = await getAppConsumption(30)
    appConsumption.value = Array.isArray(data) ? data : []
    renderPieChart()
  } catch (e) {
    console.error('获取APP消耗数据失败:', e)
    appConsumption.value = []
  } finally {
    appLoading.value = false
  }
}

const fetchAIStats = async () => {
  aiLoading.value = true
  try {
    const [modelsRes, keysRes, summaryRes] = await Promise.all([
      listTenantModelGrants().catch(() => []),
      listTenantAPIKeys().catch(() => []),
      getDashboardSummary().catch(() => ({}))
    ])
    aiStats.modelCount = modelsRes.length
    aiStats.apiKeyCount = keysRes.length
    aiStats.monthCost = summaryRes.total_user_cost || 0
  } catch (e) {
    console.error('获取AI统计失败:', e)
  } finally {
    aiLoading.value = false
  }
}

const renderPieChart = () => {
  if (!pieChartRef.value || appConsumption.value.length === 0) return

  if (pieChartInstance) {
    pieChartInstance.dispose()
  }

  pieChartInstance = echarts.init(pieChartRef.value)

  const colors = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16']

  const option = {
    tooltip: {
      trigger: 'item',
      formatter: '{b}: {c} 积分 ({d}%)'
    },
    legend: {
      orient: 'vertical',
      right: 10,
      top: 'center',
      textStyle: {
        fontSize: 12,
        color: '#64748b'
      }
    },
    series: [
      {
        type: 'pie',
        radius: ['40%', '70%'],
        center: ['35%', '50%'],
        avoidLabelOverlap: false,
        itemStyle: {
          borderRadius: 6,
          borderColor: '#fff',
          borderWidth: 2
        },
        label: {
          show: false
        },
        emphasis: {
          label: {
            show: true,
            fontSize: 14,
            fontWeight: 'bold'
          }
        },
        labelLine: {
          show: false
        },
        data: appConsumption.value.map((item, index) => ({
          name: item.appName || '其他/未知',
          value: item.credits,
          itemStyle: { color: colors[index % colors.length] }
        }))
      }
    ]
  }

  pieChartInstance.setOption(option)
}

const fetchData = async () => {
  loading.value = true
  try {
    await Promise.all([fetchOverview(), fetchBalance(), fetchTransactions(), fetchRecharges(), fetchAppConsumption(), fetchAIStats()])
  } finally {
    loading.value = false
  }
}

// 监听窗口大小变化，调整图表
const handleResize = () => {
  if (pieChartInstance) {
    pieChartInstance.resize()
  }
}

watch(appConsumption, () => {
  renderPieChart()
})

onMounted(() => {
  fetchData()
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  if (pieChartInstance) {
    pieChartInstance.dispose()
  }
  window.removeEventListener('resize', handleResize)
})
</script>

<style scoped>
.animate-spin {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
