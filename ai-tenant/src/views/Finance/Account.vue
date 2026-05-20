<template>
  <div class="space-y-6">
    <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 class="text-2xl font-black text-slate-800 tracking-tight">我的账户</h1>
          <p class="text-slate-400 text-sm font-medium mt-1">查看租户积分余额和积分包详情</p>
        </div>
        <el-button type="primary" class="!rounded-2xl font-bold" :loading="loading" @click="fetchData">
          <template #icon><el-icon><Refresh /></el-icon></template>
          立即刷新
        </el-button>
      </div>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
      <div class="bg-white rounded-2xl p-6 border border-slate-50 shadow-soft flex items-start gap-4">
        <div class="w-12 h-12 rounded-2xl bg-primary-50 flex items-center justify-center flex-shrink-0">
          <el-icon class="text-primary-500" :size="24"><Coin /></el-icon>
        </div>
        <div>
          <p class="text-xs font-bold text-slate-400 uppercase tracking-widest mb-1">总积分</p>
          <p class="text-3xl font-black text-slate-800">
            <span v-if="loading" class="text-slate-200">—</span>
            <span v-else>{{ stats.totalCredits ?? 0 }}</span>
          </p>
          <p class="text-xs text-slate-400 mt-1">所有积分包剩余总和</p>
        </div>
      </div>

      <div class="bg-white rounded-2xl p-6 border border-slate-50 shadow-soft flex items-start gap-4">
        <div class="w-12 h-12 rounded-2xl bg-amber-50 flex items-center justify-center flex-shrink-0">
          <el-icon class="text-amber-500" :size="24"><Lock /></el-icon>
        </div>
        <div>
          <p class="text-xs font-bold text-slate-400 uppercase tracking-widest mb-1">冻结积分</p>
          <p class="text-3xl font-black text-slate-800">
            <span v-if="loading" class="text-slate-200">—</span>
            <span v-else>{{ stats.frozenCredits ?? 0 }}</span>
          </p>
          <p class="text-xs text-slate-400 mt-1">预授权冻结中</p>
        </div>
      </div>

      <div class="bg-white rounded-2xl p-6 border border-slate-50 shadow-soft flex items-start gap-4">
        <div class="w-12 h-12 rounded-2xl bg-emerald-50 flex items-center justify-center flex-shrink-0">
          <el-icon class="text-emerald-500" :size="24"><Check /></el-icon>
        </div>
        <div>
          <p class="text-xs font-bold text-slate-400 uppercase tracking-widest mb-1">可用积分</p>
          <p class="text-3xl font-black text-slate-800">
            <span v-if="loading" class="text-slate-200">—</span>
            <span v-else>{{ stats.availableCredits ?? 0 }}</span>
          </p>
          <p class="text-xs text-slate-400 mt-1">总积分 - 冻结积分</p>
        </div>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
      <div class="flex items-center justify-between p-6 border-b border-slate-50">
        <div>
          <h2 class="text-base font-bold text-slate-800">有效积分包</h2>
          <p class="text-xs text-slate-400 mt-0.5">共 {{ packages.length }} 个积分包</p>
        </div>
      </div>

      <div v-if="pkgLoading" class="flex items-center justify-center py-16">
        <el-icon class="text-slate-300 animate-spin" :size="36"><Loading /></el-icon>
      </div>

      <div v-else-if="packages.length === 0" class="flex flex-col items-center justify-center py-16 text-slate-400">
        <el-icon :size="48"><Box /></el-icon>
        <p class="mt-4 text-sm">暂无积分包</p>
      </div>

      <div v-else class="p-6 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        <div
          v-for="pkg in packages"
          :key="pkg.packageId"
          class="p-4 rounded-xl border transition-all"
          :class="pkg.status === 1 ? 'border-primary-100 bg-primary-50/30' : 'border-slate-100 bg-slate-50'"
        >
          <div class="flex items-start justify-between mb-3">
            <div class="flex items-center gap-2">
              <el-tag :type="pkg.status === 1 ? 'success' : 'info'" size="small" round>
                {{ pkg.status === 1 ? '可用' : pkg.status === 2 ? '已过期' : '已耗尽' }}
              </el-tag>
              <span class="text-xs text-slate-400">{{ pkg.packageId }}</span>
            </div>
          </div>

          <div class="space-y-2">
            <div class="flex items-center justify-between">
              <span class="text-xs text-slate-500">剩余积分</span>
              <span class="text-lg font-bold text-slate-800">{{ pkg.remainingCredits?.toLocaleString() ?? 0 }}</span>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-xs text-slate-500">总积分</span>
              <span class="text-sm font-semibold text-slate-600">{{ pkg.totalCredits?.toLocaleString() ?? 0 }}</span>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-xs text-slate-500">过期时间</span>
              <span class="text-xs text-slate-600">{{ pkg.expireTime ? formatTime(pkg.expireTime) : '永久有效' }}</span>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-xs text-slate-500">来源</span>
              <span class="text-xs text-slate-600">{{ pkg.source || '管理员充值' }}</span>
            </div>
          </div>

          <div class="mt-3">
            <el-progress
              :percentage="pkg.totalCredits > 0 ? Math.round((pkg.remainingCredits / pkg.totalCredits) * 100) : 0"
              :stroke-width="6"
              :show-text="false"
              :color="pkg.status === 1 ? '#3b82f6' : '#94a3b8'"
            />
          </div>
        </div>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
      <div class="flex items-center justify-between p-6 border-b border-slate-50">
        <div>
          <h2 class="text-base font-bold text-slate-800">我的充值记录</h2>
          <p class="text-xs text-slate-400 mt-0.5">管理员对本租户的历史充值明细</p>
        </div>
      </div>

      <div v-if="rechargeLoading" class="flex items-center justify-center py-16">
        <el-icon class="text-slate-300 animate-spin" :size="36"><Loading /></el-icon>
      </div>

      <el-table v-else :data="rechargeList" empty-text="暂无数据">
        <el-table-column prop="orderId" label="充值单号" width="140" show-overflow-tooltip />
        <el-table-column label="实付金额" width="140">
          <template #default="{ row }">
            <span class="font-bold text-slate-700">¥{{ (row.paidAmount / 100).toFixed(2) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="到账积分" width="160">
          <template #default="{ row }">
            <span class="font-bold text-base text-emerald-600">+{{ row.creditAmount?.toLocaleString() }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="110">
          <template #default="{ row }">
            <el-tag :type="rechargeStatusTag(row.status)" size="small" round>
              {{ rechargeStatusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="note" label="备注" min-width="180" show-overflow-tooltip />
        <el-table-column prop="createdTime" label="充值时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.createdTime) }}
          </template>
        </el-table-column>
      </el-table>

      <div class="flex justify-end px-6 py-4 border-t border-slate-50" v-if="rechargeTotal > 0">
        <el-pagination
          v-model:current-page="rechargePage"
          v-model:page-size="rechargePageSize"
          :total="rechargeTotal"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          background
          @change="fetchRechargeRecords"
        />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { Refresh, Coin, Lock, Check, Loading, Box } from '@element-plus/icons-vue'
import { getAccountBalance, getRechargeRecords } from '@/api/tenant'
import dayjs from 'dayjs'

const loading = ref(false)
const pkgLoading = ref(false)

const stats = reactive({
  totalCredits: 0,
  frozenCredits: 0,
  availableCredits: 0
})

const packages = ref([])

const rechargeList = ref([])
const rechargePage = ref(1)
const rechargePageSize = ref(20)
const rechargeTotal = ref(0)
const rechargeLoading = ref(false)

const formatTime = (ts) => {
  if (!ts) return '—'
  return dayjs(ts).format('YYYY-MM-DD HH:mm:ss')
}

const rechargeStatusTag = (status) => {
  const map = { active: 'success', reversed: 'info' }
  return map[status] || 'info'
}

const rechargeStatusText = (status) => {
  const map = { active: '有效', reversed: '已撤销' }
  return map[status] || status || '—'
}

const fetchRechargeRecords = async () => {
  rechargeLoading.value = true
  try {
    const res = await getRechargeRecords({ page: rechargePage.value, size: rechargePageSize.value, rechargeType: 1 })
    rechargeList.value = Array.isArray(res) ? res : (res?.list || res?.records || [])
    rechargeTotal.value = res?.total || rechargeList.value.length
  } catch (e) {
    console.error('获取充值记录失败:', e)
    rechargeList.value = []
    rechargeTotal.value = 0
  } finally {
    rechargeLoading.value = false
  }
}

const fetchData = async () => {
  loading.value = true
  pkgLoading.value = true
  try {
    const data = await getAccountBalance(true)
    if (data) {
      stats.totalCredits = data.totalCredits ?? 0
      stats.frozenCredits = data.frozenCredits ?? 0
      stats.availableCredits = data.availableCredits ?? 0
      if (data.packages && Array.isArray(data.packages)) {
        packages.value = data.packages.map(pkg => ({
          packageId: pkg.packageId,
          totalCredits: pkg.totalCredits,
          remainingCredits: pkg.remainingCredits,
          expireTime: pkg.expireTime,
          source: pkg.source || '充值',
          status: pkg.expireTime && pkg.expireTime < Date.now() ? 2 : (pkg.remainingCredits <= 0 ? 3 : 1)
        }))
      }
    }
  } catch (e) {
    console.error('获取账户信息失败:', e)
  } finally {
    loading.value = false
    pkgLoading.value = false
  }
}

onMounted(() => {
  fetchData()
  fetchRechargeRecords()
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
