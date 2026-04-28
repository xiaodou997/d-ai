<script setup>
import { onMounted, reactive, shallowRef } from 'vue'
import { Search } from '@element-plus/icons-vue'
import { getTenantRechargeRecords } from '@/api/finance'

const loading = shallowRef(false)
const tableData = shallowRef([])
const queryForm = reactive({
  tenantName: ''
})
const pagination = reactive({
  page: 1,
  size: 20,
  total: 0
})

const yuan = (cents) => ((Number(cents) || 0) / 100).toFixed(2)
const credits = (value) => (Number(value) || 0).toLocaleString('zh-CN')
const formatTime = (value) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '-'

const fetchData = async () => {
  loading.value = true
  try {
    const data = await getTenantRechargeRecords({
      page: pagination.page,
      size: pagination.size,
      tenantName: queryForm.tenantName || undefined
    })
    tableData.value = data.records || []
    pagination.total = data.total || 0
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  pagination.page = 1
  fetchData()
}

const handleReset = () => {
  queryForm.tenantName = ''
  handleSearch()
}

onMounted(fetchData)
</script>

<template>
  <div class="page-container space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <section class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <div class="flex justify-between items-center mb-6">
        <div>
          <h2 class="text-lg font-black text-slate-800">租户充值记录</h2>
          <p class="text-sm text-slate-400 mt-1">仅展示租户充值记录，不包含退款、用户充值、补发和全量交易流水。</p>
        </div>
      </div>
      <el-form :model="queryForm" class="grid grid-cols-1 md:grid-cols-[minmax(220px,360px)_auto] gap-4 items-end">
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">租户名称</label>
          <el-input v-model="queryForm.tenantName" placeholder="搜索租户名称" clearable @keyup.enter="handleSearch" />
        </div>
        <div class="flex gap-2">
          <el-button type="primary" class="!rounded-2xl font-bold px-8 h-11" @click="handleSearch">
            <el-icon class="mr-1"><Search /></el-icon>
            筛选
          </el-button>
          <el-button class="!rounded-2xl h-11 px-6" @click="handleReset">重置</el-button>
        </div>
      </el-form>
    </section>

    <section class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
      <el-table v-loading="loading" :data="tableData" border stripe class="modern-table w-full">
        <el-table-column prop="rechargeNo" label="充值单号" min-width="200" show-overflow-tooltip />
        <el-table-column prop="tenantName" label="租户名称" min-width="150" show-overflow-tooltip />
        <el-table-column label="实付金额（元）" width="140" align="right">
          <template #default="{ row }">
            <span class="font-bold text-slate-700">¥ {{ yuan(row.paidAmount) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="到账积分" width="140" align="right">
          <template #default="{ row }">
            <span class="font-black text-emerald-600">+{{ credits(row.creditAmount) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="类型" width="110" align="center">
          <template #default>
            <el-tag type="primary" size="small" class="!rounded-full font-bold">租户充值</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="remark" label="备注" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.remark">{{ row.remark }}</span>
            <span v-else class="text-slate-300">-</span>
          </template>
        </el-table-column>
        <el-table-column label="充值时间" width="180">
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
    </section>
  </div>
</template>
