<template>
  <div class="page-container space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <div class="flex justify-between items-center mb-6">
        <h2 class="text-lg font-bold text-slate-800">操作日志</h2>
      </div>
      <el-form :model="queryForm" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 items-end">
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">操作人 ID</label>
          <el-input v-model="queryForm.operatorId" placeholder="操作人 ID" clearable class="modern-input" />
        </div>
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">操作类型</label>
          <el-select v-model="queryForm.operationType" placeholder="操作类型" clearable class="modern-select w-full">
            <el-option label="充值" value="RECHARGE" />
            <el-option label="退款" value="REFUND" />
            <el-option label="冻结" value="FREEZE" />
          </el-select>
        </div>
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">目标主体 ID</label>
          <el-input v-model="queryForm.targetAccountId" placeholder="目标主体 ID" clearable class="modern-input" />
        </div>
        <div class="flex gap-2 items-end">
          <el-button type="primary" class="!rounded-2xl font-bold px-8 h-11 flex-1" @click="handleSearch">筛选</el-button>
          <el-button @click="handleReset" class="!rounded-2xl h-11 px-6">重置</el-button>
        </div>
      </el-form>
    </div>

    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
      <el-table v-loading="loading" :data="tableData" border stripe class="modern-table">
        <el-table-column prop="operatorId" label="操作员" width="120" />
        <el-table-column label="动作" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="getOpTypeTag(row.operationType)" size="small" class="rounded-2xl font-bold">
              {{ getOpTypeLabel(row.operationType) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="关联主体" min-width="180">
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <el-tag size="small" class="!bg-slate-100 !text-slate-500 border-none">{{ row.targetAccountType === 1 ? '租户' : '用户' }}</el-tag>
              <span class="text-sm font-bold text-slate-700">{{ row.targetAccountId }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="变更积分" width="130" align="right">
          <template #default="{ row }">
            <span v-if="row.credits" class="font-black text-slate-700">{{ row.credits.toLocaleString() }} 积分</span>
            <span v-else class="text-slate-300">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="reason" label="变更原因" show-overflow-tooltip />
        <el-table-column label="时间" width="170">
          <template #default="{ row }">
            <span class="text-xs text-slate-400">{{ formatTime(row.operationTime) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="数据" width="80" align="center" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="showDetail(row)">详情</el-button>
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
        />
      </div>
    </div>

    <!-- 详情对话框放在这里，作为父容器的一个子节点 -->
    <el-dialog v-model="detailDialogVisible" title="操作原始报文" width="600px" class="modern-dialog">
      <div v-if="currentDetail" class="space-y-4">
        <div class="bg-slate-50 p-4 rounded-2xl">
          <pre class="text-xs font-mono text-slate-600 overflow-auto max-h-60">{{ formatJson(currentDetail.operationData) }}</pre>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <div class="p-3 bg-slate-50 rounded-xl">
            <p class="text-[10px] text-slate-400 font-bold uppercase">IP Address</p>
            <p class="text-sm font-bold text-slate-700">{{ currentDetail.ipAddress || '-' }}</p>
          </div>
          <div class="p-3 bg-slate-50 rounded-xl">
            <p class="text-[10px] text-slate-400 font-bold uppercase">Log ID</p>
            <p class="text-sm font-bold text-slate-700">#{{ currentDetail.id }}</p>
          </div>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { getOperationLogs } from '@/api/user'

const queryForm = reactive({ operatorId: '', operationType: null, targetAccountId: '' })
const pagination = reactive({ page: 1, size: 15, total: 0 })
const tableData = ref([]); const loading = ref(false)
const detailDialogVisible = ref(false); const currentDetail = ref(null)

const OP_TYPE_LABEL = {
  'RECHARGE': '充值', 'REFUND': '退款',
  'FREEZE_ACCOUNT': '冻结', 'UNFREEZE_ACCOUNT': '解冻'
}
const OP_TYPE_TAG = {
  'RECHARGE': 'success', 'REFUND': 'warning',
  'FREEZE_ACCOUNT': 'danger', 'UNFREEZE_ACCOUNT': 'info'
}
const getOpTypeTag = (t) => OP_TYPE_TAG[t] || 'info'
const getOpTypeLabel = (t) => OP_TYPE_LABEL[t] || t

const fetchData = async () => {
  loading.value = true
  try {
    const data = await getOperationLogs({ ...queryForm, page: pagination.page, size: pagination.size })
    tableData.value = data.records; pagination.total = data.total
  } finally { loading.value = false }
}

const formatTime = (ts) => ts ? new Date(ts).toLocaleString() : ''
const handleSearch = () => { pagination.page = 1; fetchData() }
const handleReset = () => { Object.assign(queryForm, { operatorId: '', operationType: null, targetAccountId: '' }); handleSearch() }
const showDetail = (r) => { currentDetail.value = r; detailDialogVisible.value = true }
const formatJson = (s) => { try { return JSON.stringify(typeof s === 'string' ? JSON.parse(s) : s, null, 2) } catch { return s } }

onMounted(fetchData)
</script>