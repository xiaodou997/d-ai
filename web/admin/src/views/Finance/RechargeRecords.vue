<template>
  <div class="page-container space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <!-- 筛选面板 -->
    <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <div class="flex justify-between items-center mb-6">
        <h2 class="text-lg font-bold text-slate-800">充值记录</h2>
      </div>
      <el-form :model="queryForm" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 items-end">
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">租户名称</label>
          <el-input v-model="queryForm.tenantName" placeholder="搜索租户名称" clearable class="modern-input" @keyup.enter="handleSearch" />
        </div>
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">类型</label>
          <el-select v-model="queryForm.rechargeType" placeholder="全部" clearable class="w-full">
            <el-option label="全部" :value="0" />
            <el-option label="租户充值" :value="1" />
            <el-option label="用户充值" :value="2" />
          </el-select>
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
        <el-table-column prop="rechargeNo" label="充值单号" min-width="200" show-overflow-tooltip />
        <el-table-column prop="tenantName" label="租户名称" min-width="140" show-overflow-tooltip />
        <el-table-column prop="username" label="用户名" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.username">{{ row.username }}</span>
            <span v-else class="text-slate-300">—</span>
          </template>
        </el-table-column>
        <el-table-column label="实付金额（元）" width="140" align="right">
          <template #default="{ row }">
            <span class="font-bold text-slate-700">¥ {{ (row.paidAmount / 100).toFixed(2) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="到账积分" width="140" align="right">
          <template #default="{ row }">
            <span class="font-bold text-emerald-600">+{{ (row.creditAmount || 0).toLocaleString() }}</span>
            <span class="text-xs text-slate-400 ml-1">积分</span>
          </template>
        </el-table-column>
        <el-table-column label="类型" width="110" align="center">
          <template #default="{ row }">
            <el-tag
              :type="row.rechargeType === 1 ? 'primary' : 'success'"
              size="small"
              class="!rounded-full font-bold"
            >
              {{ row.rechargeType === 1 ? '租户充值' : '用户充值' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="remark" label="备注" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="text-slate-400 text-xs" v-if="!row.remark">—</span>
            <span v-else>{{ row.remark }}</span>
          </template>
        </el-table-column>
        <el-table-column label="充值时间" width="170">
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
import { getRechargeRecords } from '@/api/finance'

const queryForm = reactive({ tenantName: '', rechargeType: 0 })
const loading = ref(false)
const tableData = ref([])
const pagination = reactive({ page: 1, size: 20, total: 0 })

const fetchData = async () => {
  loading.value = true
  try {
    const params = {
      page: pagination.page,
      size: pagination.size,
      tenantName: queryForm.tenantName || undefined,
      rechargeType: queryForm.rechargeType || undefined
    }
    const response = await getRechargeRecords(params)
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
  queryForm.rechargeType = 0
  pagination.page = 1
  fetchData()
}

const formatTime = (ts) => {
  if (!ts) return '—'
  return new Date(ts).toLocaleString('zh-CN', { hour12: false })
}

onMounted(fetchData)
</script>
