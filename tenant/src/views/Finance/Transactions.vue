<template>
  <div class="page-container space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
      <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
        <div class="flex items-center justify-between mb-2">
          <div class="text-[10px] font-black text-slate-400 uppercase">总流水数</div>
          <el-icon class="text-slate-300"><Document /></el-icon>
        </div>
        <div class="text-3xl font-black text-slate-700">{{ stats.totalCount }}</div>
      </div>

      <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
        <div class="flex items-center justify-between mb-2">
          <div class="text-[10px] font-black text-slate-400 uppercase">租户总消耗</div>
          <el-icon class="text-slate-300"><Coin /></el-icon>
        </div>
        <div class="text-3xl font-black text-red-500">{{ stats.totalTenantCost }}</div>
      </div>

      <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
        <div class="flex items-center justify-between mb-2">
          <div class="text-[10px] font-black text-slate-400 uppercase">用户总消耗</div>
          <el-icon class="text-slate-300"><Coin /></el-icon>
        </div>
        <div class="text-3xl font-black text-amber-500">{{ stats.totalUserCost }}</div>
      </div>

      <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
        <div class="flex items-center justify-between mb-2">
          <div class="text-[10px] font-black text-slate-400 uppercase">成功率</div>
          <el-icon class="text-slate-300"><CircleCheck /></el-icon>
        </div>
        <div class="text-3xl font-black text-emerald-500">{{ stats.successRate }}%</div>
      </div>
    </div>

    <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <div class="flex justify-between items-center mb-4">
        <h2 class="text-lg font-bold text-slate-800">积分流水</h2>
        <el-button @click="loadTransactions" :loading="loading" class="!rounded-xl h-11 px-6">
          <el-icon class="mr-1"><Refresh /></el-icon>
          刷新
        </el-button>
      </div>

      <el-tabs v-model="activeTab" @tab-click="handleTabChange" class="mb-4">
        <el-tab-pane label="全部流水" name="all" />
        <el-tab-pane label="租户流水" name="tenant" />
        <el-tab-pane label="用户流水" name="user" />
      </el-tabs>

      <el-form :model="queryForm" class="flex gap-4 items-end">
        <div class="flex-1 max-w-md">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">描述搜索</label>
          <el-input
            v-model="queryForm.description"
            placeholder="模糊搜索描述内容"
            clearable
            @keyup.enter="handleSearch"
            class="modern-input"
          />
        </div>
        <el-button type="primary" class="!rounded-xl font-bold px-6 h-11" @click="handleSearch">搜索</el-button>
        <el-button @click="handleReset" class="!rounded-xl h-11 px-6">重置</el-button>
      </el-form>
    </div>

    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
      <el-table v-loading="loading" :data="displayList" border stripe class="modern-table w-full">
        <el-table-column prop="createdTime" label="时间" width="180">
          <template #default="{ row }">
            <span class="text-xs text-slate-500">{{ formatTime(row.createdTime) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="eventId" label="流水号" width="200" show-overflow-tooltip />
        <el-table-column prop="description" label="描述" min-width="180" show-overflow-tooltip />
        <el-table-column label="租户扣费" width="100" align="right">
          <template #default="{ row }">
            <span v-if="row.tenantCredits" class="font-black text-red-500">{{ row.tenantCredits }}</span>
            <span v-else class="text-slate-400">-</span>
          </template>
        </el-table-column>
        <el-table-column label="用户扣费" width="100" align="right">
          <template #default="{ row }">
            <span v-if="row.userCredits || row.credits" class="font-black text-amber-500">{{ row.userCredits || row.credits }}</span>
            <span v-else class="text-slate-400">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="username" label="用户名" width="120" />
        <el-table-column label="状态" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)" size="small" class="rounded-lg font-bold">
              {{ getStatusLabel(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
      </el-table>

      <div class="p-6 border-t border-slate-50 flex justify-end">
        <el-pagination
          v-model:current-page="pagination.page"
          v-model:page-size="pagination.size"
          :total="pagination.total"
          layout="total, prev, pager, next"
          @current-change="loadTransactions"
          class="modern-pagination"
        />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Document, Coin, CircleCheck } from '@element-plus/icons-vue'
import { getTransactions } from '@/api/tenant'

const loading = ref(false)
const activeTab = ref('all')
const transactions = ref([])
const pagination = reactive({ page: 1, size: 20, total: 0 })
const stats = reactive({ totalCount: 0, totalTenantCost: 0, totalUserCost: 0, successRate: 0 })
const queryForm = reactive({ description: '' })

const displayList = computed(() => {
  let list = transactions.value

  if (activeTab.value === 'tenant') {
    list = list.filter(r => r.tenantCredits)
  } else if (activeTab.value === 'user') {
    list = list.filter(r => r.userCredits || r.credits)
  }

  if (queryForm.description) {
    const keyword = queryForm.description.toLowerCase()
    list = list.filter(r => r.description?.toLowerCase().includes(keyword))
  }

  return list
})

const formatTime = (time) => {
  if (!time) return '-'
  return new Date(time).toLocaleString('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false
  }).replace(/\//g, '-')
}

const getStatusType = (status) => {
  const types = { pending: 'warning', succeeded: 'success', cancelled: 'info', refunded: 'info', released: 'info', reversed: 'info' }
  return types[status] || 'danger'
}

const getStatusLabel = (status) => {
  const labels = { pending: '处理中', succeeded: '成功', cancelled: '已取消', refunded: '已退款', released: '已释放', reversed: '已撤销' }
  return labels[status] || status || '未知'
}

const loadTransactions = async () => {
  loading.value = true
  try {
    const res = await getTransactions({ page: pagination.page, size: pagination.size })
    const records = res?.records || []
    transactions.value = records
    pagination.total = res?.total || 0

    stats.totalCount = pagination.total
    stats.totalTenantCost = records.reduce((s, r) => s + (r.tenantCredits || 0), 0)
    stats.totalUserCost = records.reduce((s, r) => s + (r.userCredits || r.credits || 0), 0)
    const successCount = records.filter(r => r.status === 'succeeded').length
    stats.successRate = records.length > 0 ? Math.round((successCount / records.length) * 100) : 0
  } catch (error) {
    console.error('加载流水失败:', error)
    ElMessage.error('加载流水失败')
  } finally {
    loading.value = false
  }
}

const handleTabChange = () => {
  pagination.page = 1
}

const handleSearch = () => {
  pagination.page = 1
}

const handleReset = () => {
  queryForm.description = ''
  pagination.page = 1
}

onMounted(() => { loadTransactions() })
</script>

<style scoped>
.modern-table :deep(.el-table__header th) {
  background-color: #f8fafc;
  color: #64748b;
  font-weight: 700;
  font-size: 11px;
  text-transform: uppercase;
}
</style>
