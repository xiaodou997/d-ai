<template>
  <div class="page-container space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <!-- 筛选面板 -->
    <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <div class="flex justify-between items-center mb-6">
        <h2 class="text-lg font-bold text-slate-800">交易流水</h2>
      </div>
      <el-form :model="queryForm" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 items-end">
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">租户名称</label>
          <el-input v-model="queryForm.tenantName" placeholder="搜索租户名称" clearable class="modern-input" @keyup.enter="handleSearch" />
        </div>
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">用户名</label>
          <el-input v-model="queryForm.username" placeholder="搜索用户名" clearable class="modern-input" @keyup.enter="handleSearch" />
        </div>
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">APP 名称</label>
          <el-input v-model="queryForm.appName" placeholder="搜索 APP 名称" clearable class="modern-input" @keyup.enter="handleSearch" />
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

    <!-- 数据表格 -->
    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
      <el-table v-loading="loading" :data="tableData" border stripe class="modern-table w-full">
        <el-table-column prop="transactionId" label="交易流水" min-width="180" show-overflow-tooltip />
        <el-table-column prop="tenantName" label="租户" min-width="130" show-overflow-tooltip />
        <el-table-column prop="username" label="用户名" min-width="110" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.username">{{ row.username }}</span>
            <span v-else class="text-slate-300">—</span>
          </template>
        </el-table-column>
        <el-table-column label="APP 名称" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.appName" class="text-xs font-medium text-indigo-600 bg-indigo-50 px-2 py-0.5 rounded-lg">{{ row.appName }}</span>
            <span v-else class="text-slate-300">—</span>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="描述" min-width="180" show-overflow-tooltip />
        <el-table-column label="租户积分" align="right" min-width="110">
          <template #default="{ row }">
            <span v-if="row.tenantCredits" class="font-black text-indigo-600">{{ (row.tenantCredits || 0).toLocaleString() }}</span>
            <span v-else class="text-slate-300">—</span>
          </template>
        </el-table-column>
        <el-table-column label="用户积分" align="right" min-width="110">
          <template #default="{ row }">
            <span v-if="row.userCredits" class="font-black text-emerald-600">{{ (row.userCredits || 0).toLocaleString() }}</span>
            <span v-else class="text-slate-300">—</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <div class="flex items-center">
              <span class="w-1.5 h-1.5 rounded-full mr-2" :class="statusDotClass(row.status)"></span>
              <span class="text-xs font-bold">{{ statusText(row.status) }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="交易时间" width="170">
          <template #default="{ row }">
            <span class="text-xs text-slate-400 font-medium">{{ formatTime(row.createdTime) }}</span>
          </template>
        </el-table-column>
      </el-table>

      <div class="p-6 border-t border-slate-50 flex justify-end">
        <el-pagination
          v-model:current-page="pagination.page"
          v-model:page-size="pagination.size"
          :total="pagination.total"
          layout="total, prev, pager, next"
          @current-change="fetchData"
          class="modern-pagination"
        />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { Search } from '@element-plus/icons-vue'
import { getTransactions } from '@/api/finance'

const queryForm = reactive({ tenantName: '', username: '', appName: '' })
const loading = ref(false)
const tableData = ref([])
const pagination = reactive({ page: 1, size: 20, total: 0 })

const statusText = (s) => ({ 0: '进行中', 1: '成功', 2: '取消', 3: '退款', 4: '已释放' }[s] ?? '—')
const statusDotClass = (s) => s === 1 ? 'bg-emerald-500' : s === 0 ? 'bg-amber-400' : 'bg-rose-500'

const fetchData = async () => {
  loading.value = true
  try {
    const params = {
      page: pagination.page,
      size: pagination.size,
      tenantName: queryForm.tenantName || undefined,
      username: queryForm.username || undefined,
      appName: queryForm.appName || undefined
    }
    const response = await getTransactions(params)
    tableData.value = response.records || []
    pagination.total = response.total || 0
  } catch {
    tableData.value = []
  } finally {
    loading.value = false
  }
}

const handleSearch = () => { pagination.page = 1; fetchData() }
const handleReset = () => {
  queryForm.tenantName = ''
  queryForm.username = ''
  queryForm.appName = ''
  pagination.page = 1
  fetchData()
}

const formatTime = (ts) => {
  if (!ts) return '—'
  return new Date(ts).toLocaleString('zh-CN', { hour12: false })
}

onMounted(fetchData)
</script>
