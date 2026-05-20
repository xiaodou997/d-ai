<template>
  <div class="page-container space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
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
          <el-select v-model="queryForm.orderType" placeholder="全部" clearable class="w-full">
            <el-option label="平台→租户" value="platform_to_tenant" />
            <el-option label="租户→用户" value="tenant_to_user" />
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

    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
      <el-table v-loading="loading" :data="tableData" border stripe class="modern-table w-full">
        <el-table-column prop="orderId" label="充值单号" min-width="200" show-overflow-tooltip />
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
        <el-table-column label="类型" width="120" align="center">
          <template #default="{ row }">
            <el-tag
              :type="row.orderType === 'platform_to_tenant' ? 'primary' : 'success'"
              size="small"
              class="!rounded-full font-bold"
            >
              {{ row.orderType === 'platform_to_tenant' ? '平台→租户' : '租户→用户' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="row.status === 'active' ? 'success' : 'info'" size="small" class="!rounded-full font-bold">
              {{ row.status === 'active' ? '有效' : '已撤销' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="note" label="备注" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="text-slate-400 text-xs" v-if="!row.note">—</span>
            <span v-else>{{ row.note }}</span>
          </template>
        </el-table-column>
        <el-table-column label="充值时间" width="170">
          <template #default="{ row }">
            <span class="text-xs text-slate-400 font-medium">{{ formatTime(row.createdTime) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100" align="center" fixed="right">
          <template #default="{ row }">
            <el-button
              v-if="row.status === 'active'"
              type="danger"
              size="small"
              class="!rounded-xl"
              @click="handleReverse(row)"
            >
              撤销
            </el-button>
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

    <el-dialog v-model="reverseDialogVisible" title="确认撤销充值" width="480" :close-on-click-modal="false" :append-to-body="true">
      <div class="space-y-4">
        <div class="bg-red-50 border border-red-100 rounded-xl p-4">
          <p class="text-sm text-red-700 font-bold">⚠ 此操作将回收该充值对应的积分包剩余积分，请确认操作无误。</p>
        </div>
        <div v-if="reverseRow" class="space-y-2 text-sm">
          <div class="flex justify-between"><span class="text-slate-500">充值单号</span><span class="font-mono">{{ reverseRow.orderId }}</span></div>
          <div class="flex justify-between"><span class="text-slate-500">到账积分</span><span class="font-bold">{{ (reverseRow.creditAmount || 0).toLocaleString() }}</span></div>
          <div class="flex justify-between"><span class="text-slate-500">当前状态</span><span>{{ statusLabel(reverseRow.status) }}</span></div>
        </div>
        <el-form :model="reverseForm" label-position="top">
          <el-form-item label="撤销原因" required>
            <el-input v-model="reverseForm.reason" type="textarea" :rows="3" placeholder="请输入撤销原因" />
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
        <el-button @click="reverseDialogVisible = false" class="!rounded-xl">取消</el-button>
        <el-button type="danger" :loading="reverseLoading" @click="confirmReverse" class="!rounded-xl">确认撤销</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { Search } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { getRechargeRecords, reverseRecharge } from '@/api/account'

const queryForm = reactive({ tenantName: '', orderType: '' })
const loading = ref(false)
const tableData = ref([])
const pagination = reactive({ page: 1, size: 20, total: 0 })
const reverseDialogVisible = ref(false)
const reverseLoading = ref(false)
const reverseRow = ref(null)
const reverseForm = reactive({ reason: '' })

const fetchData = async () => {
  loading.value = true
  try {
    const params = {
      page: pagination.page,
      size: pagination.size,
      tenantName: queryForm.tenantName || undefined,
      rechargeType: queryForm.orderType === 'platform_to_tenant' ? 1 : queryForm.orderType === 'tenant_to_user' ? 2 : undefined
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
  queryForm.orderType = ''
  pagination.page = 1
  fetchData()
}

const formatTime = (ts) => {
  if (!ts) return '—'
  return new Date(ts).toLocaleString('zh-CN', { hour12: false })
}

const statusLabel = (s) => ({ active: '有效', reversed: '已撤销' }[s] || s || '有效')

const handleReverse = (row) => {
  reverseRow.value = row
  reverseForm.reason = ''
  reverseDialogVisible.value = true
}

const confirmReverse = async () => {
  if (!reverseForm.reason.trim()) {
    ElMessage.warning('请输入撤销原因')
    return
  }
  reverseLoading.value = true
  try {
    const result = await reverseRecharge(reverseRow.value.orderId, { reason: reverseForm.reason })
    reverseDialogVisible.value = false
    if (result.status === 'PARTIAL_REVERSAL') {
      ElMessage.warning({
        message: `部分撤销成功：回收 ${result.reversedCredits} 积分，已消耗 ${result.lostCredits} 积分无法回收`,
        duration: 5000
      })
    } else {
      ElMessage.success('充值撤销成功')
    }
    fetchData()
  } catch (err) {
    ElMessage.error(err?.response?.data?.message || '撤销失败')
  } finally {
    reverseLoading.value = false
  }
}

onMounted(fetchData)
</script>
