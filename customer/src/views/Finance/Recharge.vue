<template>
  <div class="space-y-6">
    <div class="bg-white p-6 rounded-3xl border border-slate-50 shadow-soft">
      <h1 class="text-2xl font-black text-slate-800 tracking-tight">充值记录</h1>
      <p class="text-slate-400 text-sm font-medium mt-1">查看我的的历史充值明细</p>
    </div>

    <div class="bg-white rounded-[32px] border border-slate-50 shadow-soft overflow-hidden">
      <div v-if="loading" class="flex items-center justify-center py-16">
        <el-icon class="text-slate-300 animate-spin" :size="36"><Loading /></el-icon>
      </div>

      <el-table v-else :data="list" empty-text="暂无数据">
        <el-table-column prop="orderId" label="充值单号" width="200">
          <template #default="{ row }">
            <span class="font-mono text-sm text-slate-600">{{ row.orderId }}</span>
          </template>
        </el-table-column>
        <el-table-column label="实付金额（元）" width="150">
          <template #default="{ row }">
            <span class="font-bold text-slate-700">¥ {{ ((row.paidAmount || 0) / 100).toFixed(2) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="到账积分" width="160">
          <template #default="{ row }">
            <span class="font-bold text-base text-emerald-600">+{{ (row.creditAmount || 0).toLocaleString() }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="110">
          <template #default="{ row }">
            <el-tag :type="rechargeStatusTag(row.status)" size="small" round>
              {{ rechargeStatusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="note" label="备注" min-width="180" show-overflow-tooltip />
        <el-table-column prop="createdTime" label="充值时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.createdTime) }}
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
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { Loading } from '@element-plus/icons-vue'
import { getRechargeRecords } from '@/api/customer'
import dayjs from 'dayjs'

const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const loading = ref(false)
const list = ref([])

const formatTime = (ts) => {
  if (!ts) return '—'
  return dayjs(ts).format('YYYY-MM-DD HH:mm:ss')
}

const rechargeStatusTag = (status) => {
  const map = { SUCCESS: 'success', PENDING: 'warning', FAILED: 'danger', 1: 'success', 0: 'warning', '-1': 'danger' }
  return map[status] || 'info'
}

const rechargeStatusText = (status) => {
  const map = { SUCCESS: '成功', PENDING: '处理中', FAILED: '失败', 1: '成功', 0: '处理中', '-1': '失败' }
  return map[status] || String(status)
}

const fetchList = async () => {
  loading.value = true
  try {
    const res = await getRechargeRecords({ page: page.value, size: pageSize.value })
    list.value = Array.isArray(res) ? res : (res?.list || res?.records || [])
    total.value = res?.total || list.value.length
  } catch (e) {
    console.error('获取充值记录失败:', e)
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
