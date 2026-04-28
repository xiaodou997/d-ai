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
        <el-table-column prop="rechargeNo" label="充值单号" width="200" show-overflow-tooltip />
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
        <el-table-column label="来源" width="100">
          <template #default="{ row }">
            <el-tag :type="row.source === 'ADMIN' ? 'primary' : 'info'" size="small" class="!rounded-full">
              {{ row.source === 'ADMIN' ? '手动充值' : (row.source === 'TENANT_RECHARGE' ? '租户充值' : (row.source || '—')) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'danger'" size="small" round>
              {{ row.status === 1 ? '成功' : '失败' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="remark" label="备注" show-overflow-tooltip />
        <el-table-column label="时间" width="180">
          <template #default="{ row }">{{ formatTime(row.createdTime) }}</template>
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
import { getUserRechargeRecords } from '@/api/tenant'
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
