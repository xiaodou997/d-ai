<template>
  <div class="space-y-6">
    <div class="bg-white p-6 rounded-xl border border-slate-50 shadow-soft">
      <h1 class="text-2xl font-black text-slate-800 tracking-tight">用户充值记录</h1>
      <p class="text-slate-400 text-sm font-medium mt-1">查看本租户下所有终端用户的积分充值记录</p>
    </div>

    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
      <div v-if="loading" class="flex items-center justify-center py-16">
        <el-icon class="text-slate-300 animate-spin" :size="36"><Loading /></el-icon>
      </div>

      <el-table v-else :data="list" empty-text="暂无数据">
        <el-table-column prop="orderId" label="充值单号" width="200" show-overflow-tooltip />
        <el-table-column prop="username" label="用户名" width="130" />
        <el-table-column label="实付金额" width="120">
          <template #default="{ row }">
            <span class="font-semibold text-slate-700">¥ {{ (row.paidAmount / 100).toFixed(2) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="到账积分" width="120">
          <template #default="{ row }">
            <span class="font-bold text-emerald-600">+{{ row.creditAmount?.toLocaleString() }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.status === 'active' ? 'success' : 'info'" size="small" round>
              {{ row.status === 'active' ? '有效' : '已撤销' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="note" label="备注" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.note">{{ row.note }}</span>
            <span v-else class="text-slate-300">—</span>
          </template>
        </el-table-column>
        <el-table-column label="时间" width="180">
          <template #default="{ row }">{{ formatTime(row.createdTime) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="80" align="center" fixed="right">
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

      <div class="flex justify-end px-6 py-4 border-t border-slate-50" v-if="total > 0">
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

    <el-dialog v-model="reverseDialogVisible" title="确认撤销充值" width="480" :close-on-click-modal="false" :append-to-body="true">
      <div class="space-y-4">
        <div class="bg-red-50 border border-red-100 rounded-xl p-4">
          <p class="text-sm text-red-700 font-bold">⚠ 此操作将回收该充值对应的积分包剩余积分，请确认操作无误。</p>
        </div>
        <div v-if="reverseRow" class="space-y-2 text-sm">
          <div class="flex justify-between"><span class="text-slate-500">充值单号</span><span class="font-mono">{{ reverseRow.orderId }}</span></div>
          <div class="flex justify-between"><span class="text-slate-500">到账积分</span><span class="font-bold">{{ (reverseRow.creditAmount || 0).toLocaleString() }}</span></div>
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
import { Loading } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { getUserRechargeRecords, reverseRecharge } from '@/api/tenant'
import dayjs from 'dayjs'

const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const loading = ref(false)
const list = ref([])
const reverseDialogVisible = ref(false)
const reverseLoading = ref(false)
const reverseRow = ref(null)
const reverseForm = reactive({ reason: '' })

const formatTime = (ts) => {
  if (!ts) return '—'
  return dayjs(ts).format('YYYY-MM-DD HH:mm:ss')
}

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
    fetchList()
  } catch (err) {
    ElMessage.error(err?.response?.data?.message || '撤销失败')
  } finally {
    reverseLoading.value = false
  }
}

const fetchList = async () => {
  loading.value = true
  try {
    const res = await getUserRechargeRecords({ page: page.value, size: pageSize.value })
    list.value = res?.records || []
    total.value = res?.total || 0
  } catch (e) {
    console.error('获取用户充值记录失败:', e)
    list.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchList()
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
