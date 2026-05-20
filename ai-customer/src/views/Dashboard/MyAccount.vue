<template>
  <div class="space-y-6">
    <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 class="text-2xl font-black text-slate-800 tracking-tight">我的账户</h1>
          <p class="text-slate-400 text-sm font-medium mt-1">查看我的积分余额和积分包详情</p>
        </div>
        <el-button type="primary" class="rounded-2xl! font-bold" :loading="loading" @click="fetchData">
          <template #icon><el-icon><Refresh /></el-icon></template>
          立即刷新
        </el-button>
      </div>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
      <div class="bg-white rounded-2xl p-6 border border-slate-50 shadow-soft flex items-start gap-4">
        <div class="w-12 h-12 rounded-2xl bg-primary-50 flex items-center justify-center shrink-0">
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
        <div class="w-12 h-12 rounded-2xl bg-amber-50 flex items-center justify-center shrink-0">
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
        <div class="w-12 h-12 rounded-2xl bg-emerald-50 flex items-center justify-center shrink-0">
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
          <h2 class="text-base font-bold text-slate-800">我的积分包</h2>
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
              :color="pkg.status === 1 ? '#06b6d4' : '#94a3b8'"
            />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { Refresh, Coin, Lock, Check, Loading, Box } from '@element-plus/icons-vue'
import { getBalance } from '@/api/customer'
import dayjs from 'dayjs'

const loading = ref(false)
const pkgLoading = ref(false)

const stats = reactive({
  totalCredits: 0,
  frozenCredits: 0,
  availableCredits: 0
})

const packages = ref([])

const formatTime = (ts) => {
  if (!ts) return '—'
  return dayjs(ts).format('YYYY-MM-DD')
}

const fetchBalance = async () => {
  loading.value = true
  try {
    const data = await getBalance()
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
    console.error('获取余额失败:', e)
  } finally {
    loading.value = false
  }
}

const fetchPackages = async () => {
  pkgLoading.value = false
}

const fetchData = async () => {
  await Promise.all([fetchBalance(), fetchPackages()])
}

onMounted(() => {
  fetchData()
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
