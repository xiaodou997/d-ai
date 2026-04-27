<template>
  <div class="page-container space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <!-- 筛选面板 -->
    <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <div class="flex justify-between items-center mb-6">
        <h2 class="text-lg font-bold text-slate-800">租户补发记录</h2>
      </div>
      <el-form :model="queryForm" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 items-end">
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">租户 ID</label>
          <el-input
            v-model="queryForm.tenantId"
            placeholder="租户 ID"
            clearable
            class="modern-input"
            @keyup.enter="handleSearch"
          />
        </div>
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">补发类型</label>
          <el-select v-model="queryForm.resourceType" placeholder="补发类型" clearable class="modern-select w-full">
            <el-option label="积分" value="CREDIT_POINT" />
            <el-option label="次数" value="TIMES" />
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
        <el-table-column prop="grantNo" label="补发单号" min-width="200" show-overflow-tooltip />
        <el-table-column prop="tenantId" label="租户 ID" min-width="150" />
        <el-table-column prop="tenantName" label="租户名称" min-width="150" />
        <el-table-column label="补发积分" width="140" align="right">
          <template #default="{ row }">
            <span class="font-bold text-emerald-600">+{{ row.credits.toLocaleString() }}</span>
            <span class="text-xs text-slate-400 ml-1">积分</span>
          </template>
        </el-table-column>
        <el-table-column prop="resourceType" label="资源类型" width="120">
          <template #default="{ row }">
            <span class="text-xs font-mono text-slate-500">{{ row.resourceType }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="reason" label="补发原因" min-width="200" show-overflow-tooltip />
        <el-table-column prop="operatorName" label="操作人" width="130" />
        <el-table-column label="补发时间" width="170">
          <template #default="{ row }">
            {{ formatTime(row.createdTime) }}
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
import { getTenantGrantLogs } from '@/api/finance'

const queryForm = reactive({ tenantId: '', resourceType: '' })
const loading = ref(false)
const tableData = ref([])
const pagination = reactive({ page: 1, size: 20, total: 0 })

const fetchData = async () => {
  loading.value = true
  try {
    const params = {
      page: pagination.page,
      size: pagination.size,
      tenantId: queryForm.tenantId || undefined,
      resourceType: queryForm.resourceType || undefined
    }
    const response = await getTenantGrantLogs(params)
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
  queryForm.tenantId = ''
  queryForm.resourceType = ''
  pagination.page = 1
  fetchData()
}

const formatTime = (ts) => {
  if (!ts) return '—'
  return new Date(ts).toLocaleString('zh-CN', { hour12: false })
}

onMounted(fetchData)
</script>
