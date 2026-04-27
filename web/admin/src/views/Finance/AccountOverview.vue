<template>
  <div class="page-container space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <!-- 筛选面板 -->
    <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <div class="flex justify-between items-center mb-6">
        <h2 class="text-lg font-bold text-slate-800">账户全景</h2>
      </div>
      <el-form :model="queryForm" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 items-end">
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">账户类型</label>
          <el-select v-model="queryForm.accountType" class="modern-select w-full" placeholder="账户类型">
            <el-option label="租户账户" :value="1" />
            <el-option label="用户账户" :value="2" />
          </el-select>
        </div>
        <div class="space-y-1 lg:col-span-2">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">账户 ID</label>
          <el-input
            v-model="queryForm.accountId"
            :placeholder="queryForm.accountType === 1 ? '输入租户 ID（如 T_xxx）' : '输入用户 ID（如 EU_xxx）'"
            clearable
            @keyup.enter="handleSearch"
            class="modern-input"
          />
        </div>
        <div class="flex gap-2">
          <el-button type="primary" class="!rounded-2xl font-bold px-8 h-11" @click="handleSearch">
            <el-icon class="mr-1"><Search /></el-icon>
            筛选
          </el-button>
          <el-button @click="handleReset" class="!rounded-2xl h-11 px-6">重置</el-button>
        </div>
      </el-form>
    </div>

    <!-- 账户全景 -->
    <template v-if="info">

      <!-- 顶部三栏：账户状态 + 积分构成 + 维度信息 -->
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">

        <!-- ① 账户余额卡 -->
        <div class="lg:col-span-1 bg-white p-7 rounded-2xl border border-slate-50 shadow-soft relative overflow-hidden">
          <div class="absolute -right-8 -top-8 w-36 h-36 bg-indigo-500/5 rounded-full"></div>
          <p class="text-[10px] font-black text-slate-400 uppercase tracking-widest mb-2">当前可用积分</p>
          <h2 class="text-4xl font-black tracking-tighter mb-1" :class="info.credits < 0 ? 'text-rose-600' : 'text-slate-800'">
            {{ (info.credits || 0).toLocaleString() }}
            <span class="text-base font-bold text-slate-400 ml-1">积分</span>
          </h2>
          <div class="flex flex-wrap gap-2 mt-3 mb-6">
            <el-tag :type="info.status === 1 ? 'success' : 'danger'" effect="dark" class="!border-none rounded-2xl font-bold">
              {{ info.statusDisplay }}
            </el-tag>
            <el-tag v-if="info.isOverdraft" type="danger" effect="plain" class="rounded-2xl font-bold">
              已透支 {{ info.overdraftCredits?.toLocaleString() }} 积分
            </el-tag>
          </div>
          <div class="pt-4 border-t border-slate-50 space-y-2">
            <div class="flex justify-between text-xs">
              <span class="text-slate-400 font-medium">永久有效积分</span>
              <span class="font-bold text-slate-700">{{ (info.permanentCredits || 0).toLocaleString() }}</span>
            </div>
            <div class="flex justify-between text-xs">
              <span class="text-slate-400 font-medium">限时有效积分</span>
              <span class="font-bold text-amber-600">{{ (info.temporaryCredits || 0).toLocaleString() }}</span>
            </div>
            <div class="flex justify-between text-xs">
              <span class="text-slate-400 font-medium">近30天消耗</span>
              <span class="font-bold text-rose-500">-{{ (info.recentConsumedCredits || 0).toLocaleString() }}</span>
            </div>
            <div class="flex justify-between text-xs">
              <span class="text-slate-400 font-medium">历史总消耗</span>
              <span class="font-bold text-slate-500">{{ (info.consumedCredits || 0).toLocaleString() }}</span>
            </div>
          </div>
        </div>

        <!-- ② 积分构成饼图 -->
        <div class="lg:col-span-1 bg-white p-7 rounded-2xl border border-slate-50 shadow-soft">
          <p class="text-[10px] font-black text-slate-400 uppercase tracking-widest mb-4">积分构成</p>
          <div v-if="(info.permanentCredits || 0) + (info.temporaryCredits || 0) > 0">
            <div ref="pieRef" style="width:100%;height:180px" />
            <div class="flex justify-around mt-2">
              <div class="text-center">
                <p class="text-[10px] text-slate-400 font-bold">永久</p>
                <p class="text-sm font-black text-indigo-600">{{ fmtNum(info.permanentCredits) }}</p>
              </div>
              <div class="text-center">
                <p class="text-[10px] text-slate-400 font-bold">限时</p>
                <p class="text-sm font-black text-amber-500">{{ fmtNum(info.temporaryCredits) }}</p>
              </div>
            </div>
          </div>
          <div v-else class="h-[180px] flex items-center justify-center text-slate-300 text-sm">暂无有效积分</div>
        </div>

        <!-- ③ 维度信息（租户 vs 用户） -->
        <div class="lg:col-span-1 bg-gradient-to-br from-indigo-600 to-indigo-500 p-7 rounded-2xl shadow-lg shadow-indigo-100 text-white relative overflow-hidden">
          <div class="absolute right-[-15%] bottom-[-15%] w-40 h-40 bg-white/10 rounded-full blur-xl"></div>
          <div class="relative z-10">
            <!-- 租户视图 -->
            <template v-if="info.accountType === 1">
              <p class="text-[10px] font-black text-white/60 uppercase tracking-widest mb-4">租户全景</p>
              <div class="grid grid-cols-2 gap-3">
                <div class="bg-white/10 p-3 rounded-2xl">
                  <p class="text-[10px] text-white/60 font-bold">组织用户</p>
                  <p class="text-2xl font-black">{{ info.orgUserCount || 0 }}</p>
                  <p class="text-[10px] text-white/50">人</p>
                </div>
                <div class="bg-white/10 p-3 rounded-2xl">
                  <p class="text-[10px] text-white/60 font-bold">终端用户</p>
                  <p class="text-2xl font-black">{{ info.endUserCount || 0 }}</p>
                  <p class="text-[10px] text-white/50">人</p>
                </div>
                <div class="bg-white/10 p-3 rounded-2xl">
                  <p class="text-[10px] text-white/60 font-bold">接入应用</p>
                  <p class="text-2xl font-black">{{ info.appCount || 0 }}</p>
                  <p class="text-[10px] text-white/50">个</p>
                </div>
                <div class="bg-white/10 p-3 rounded-2xl">
                  <p class="text-[10px] text-white/60 font-bold">用户积分总量</p>
                  <p class="text-lg font-black leading-tight">{{ fmtNum(info.userCreditsTotal) }}</p>
                  <p class="text-[10px] text-white/50">积分</p>
                </div>
              </div>
            </template>
            <!-- 用户视图 -->
            <template v-else>
              <p class="text-[10px] font-black text-white/60 uppercase tracking-widest mb-4">用户全景</p>
              <div class="space-y-3">
                <div class="bg-white/10 p-3 rounded-2xl">
                  <p class="text-[10px] text-white/60 font-bold mb-1">所属租户</p>
                  <p class="text-base font-black">{{ info.tenantName || '—' }}</p>
                </div>
                <div class="bg-white/10 p-3 rounded-2xl">
                  <p class="text-[10px] text-white/60 font-bold">用户 ID</p>
                  <p class="text-xs font-mono font-bold mt-1 break-all">{{ info.accountId }}</p>
                </div>
                <div class="bg-white/10 p-3 rounded-2xl">
                  <p class="text-[10px] text-white/60 font-bold">账户状态</p>
                  <p class="text-base font-black mt-1">{{ info.statusDisplay }}</p>
                </div>
              </div>
            </template>
          </div>
        </div>
      </div>

      <!-- 最近流水 -->
      <div class="bg-white p-8 rounded-2xl border border-slate-50 shadow-soft">
        <h3 class="text-base font-bold text-slate-800 mb-6 flex items-center gap-2">
          <el-icon class="text-primary-500"><List /></el-icon> 最近流水（最新15条）
        </h3>
        <el-table :data="info.recentTransactions || []" class="modern-table">
          <el-table-column label="交易 ID" min-width="190">
            <template #default="{ row }">
              <span class="text-xs font-mono text-slate-500">{{ row.transactionId }}</span>
            </template>
          </el-table-column>
          <el-table-column label="类型" width="110">
            <template #default="{ row }">
              <el-tag :type="getTxTypeTag(row.transactionType)" size="small" class="rounded-2xl font-bold">
                {{ row.transactionTypeDisplay }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="变动积分" align="right" width="140">
            <template #default="{ row }">
              <span :class="row.transactionType === 5 ? 'text-emerald-600' : 'text-rose-600'" class="font-black">
                {{ row.transactionType === 5 ? '+' : '-' }}{{ (row.credits || 0).toLocaleString() }}
              </span>
              <span class="text-xs text-slate-400 ml-1">积分</span>
            </template>
          </el-table-column>
          <el-table-column label="资源类型" width="130" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="text-xs text-slate-500">{{ row.resourceTypeCode || '—' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <div class="flex items-center gap-1">
                <span class="w-1.5 h-1.5 rounded-full" :class="row.status === 1 ? 'bg-emerald-500' : row.status === 0 ? 'bg-amber-400' : 'bg-rose-500'"></span>
                <span class="text-xs text-slate-600 font-medium">{{ row.statusDisplay }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="时间" width="170">
            <template #default="{ row }">
              <span class="text-xs text-slate-400">{{ formatTime(row.createdTime) }}</span>
            </template>
          </el-table-column>
        </el-table>
        <div v-if="!info.recentTransactions?.length" class="text-center py-10 text-slate-300 text-sm">该账户暂无交易记录</div>
      </div>
    </template>

    <!-- 空状态 -->
    <div v-if="searched && !info" class="py-20 bg-white rounded-2xl border border-dashed border-slate-200 flex flex-col items-center justify-center">
      <div class="w-20 h-20 rounded-full bg-slate-50 flex items-center justify-center mb-4 text-slate-300">
        <el-icon :size="40"><Wallet /></el-icon>
      </div>
      <p class="text-slate-400 font-bold">未找到该账户的资产信息</p>
      <p class="text-xs text-slate-300 mt-1">请检查账户 ID 或类型是否正确</p>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, nextTick, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Search, Wallet, List } from '@element-plus/icons-vue'
import * as echarts from 'echarts'
import { getAccountDetail } from '@/api/tenant'

const queryForm = reactive({ accountType: 1, accountId: '' })
const info = ref(null)
const searched = ref(false)
const loading = ref(false)

const pieRef = ref(null)
let pieChart = null

const getTxTypeTag = (type) => {
  return { 1: 'danger', 2: 'warning', 3: 'warning', 4: 'info', 5: 'success' }[type] || ''
}

const fmtNum = (n) => (n || 0).toLocaleString()

const formatTime = (ts) => {
  if (!ts) return '—'
  return new Date(ts).toLocaleString('zh-CN', { hour12: false })
}

const renderPie = (data) => {
  nextTick(() => {
    if (!pieRef.value) return
    if (!pieChart) pieChart = echarts.init(pieRef.value)
    const permanent = data.permanentCredits || 0
    const temporary = data.temporaryCredits || 0
    pieChart.setOption({
      color: ['#6366f1', '#f59e0b'],
      tooltip: { trigger: 'item', formatter: '{b}: {c} 积分 ({d}%)' },
      series: [{
        type: 'pie',
        radius: ['45%', '75%'],
        avoidLabelOverlap: false,
        itemStyle: { borderRadius: 8, borderColor: '#fff', borderWidth: 3 },
        label: { show: false },
        data: [
          { name: '永久积分', value: permanent },
          { name: '限时积分', value: temporary },
        ].filter(d => d.value > 0),
      }],
    })
  })
}

const handleSearch = async () => {
  if (!queryForm.accountId.trim()) return ElMessage.warning('请输入账户 ID')
  loading.value = true
  try {
    const response = await getAccountDetail({ accountType: queryForm.accountType, accountId: queryForm.accountId.trim() })
    searched.value = true
    info.value = data
    if (data) renderPie(data)
  } catch {
    info.value = null
    searched.value = true
  } finally {
    loading.value = false
  }
}

watch(() => info.value, (val) => {
  if (!val) {
    pieChart?.dispose()
    pieChart = null
  }
})
</script>
