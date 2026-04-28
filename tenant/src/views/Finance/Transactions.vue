<template>
  <div class="space-y-6">
    <!-- 筛选面板 -->
    <div class="bg-white p-6 rounded-xl border border-slate-50 shadow-soft">
      <div class="flex justify-between items-center mb-6">
        <div>
          <h1 class="text-2xl font-black text-slate-800 tracking-tight">交易流水</h1>
          <p class="text-slate-400 text-sm font-medium mt-1">查看本租户所有积分扣费流水记录</p>
        </div>
      </div>
      <el-form :model="queryForm" class="grid grid-cols-1 md:grid-cols-3 gap-6 items-end">
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
      <el-table v-loading="loading" :data="list" border stripe empty-text="暂无数据" class="w-full">
        <el-table-column prop="transactionId" label="交易流水" min-width="180" show-overflow-tooltip />
        <el-table-column label="用户名" min-width="110" show-overflow-tooltip>
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

      <div class="p-6 border-t border-slate-50 flex justify-end" v-if="total > 0">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="total"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          background
          @change="fetchList"
        />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { Search } from '@element-plus/icons-vue'
import { getTransactions } from '@/api/tenant'
import dayjs from 'dayjs'

const queryForm = reactive({ username: '', appName: '' })
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const loading = ref(false)
const list = ref([])

const statusText = (s) => ({ 0: '进行中', 1: '成功', 2: '取消', 3: '退款', 4: '已释放' }[s] ?? '—')
const statusDotClass = (s) => s === 1 ? 'bg-emerald-500' : s === 0 ? 'bg-amber-400' : 'bg-rose-500'

const formatTime = (ts) => {
  if (!ts) return '—'
  return dayjs(ts).format('YYYY-MM-DD HH:mm:ss')
}

const fetchList = async () => {
  loading.value = true
  try {
    const res = await getTransactions({
      page: page.value,
      size: pageSize.value,
      username: queryForm.username || undefined,
      appName: queryForm.appName || undefined
    })
    list.value = res?.records || []
    total.value = res?.total || 0
  } catch (e) {
    console.error('获取流水列表失败:', e)
    list.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

const handleSearch = () => { page.value = 1; fetchList() }
const handleReset = () => {
  queryForm.username = ''
  queryForm.appName = ''
  page.value = 1
  fetchList()
}

onMounted(fetchList)
</script>
