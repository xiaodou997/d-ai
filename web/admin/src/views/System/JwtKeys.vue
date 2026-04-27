<template>
  <div class="space-y-6">
    <!-- 页面标题 -->
    <div class="bg-white p-6 rounded-3xl border border-slate-50 shadow-soft">
      <div class="flex flex-col sm:flex-row sm:items-start justify-between gap-4">
        <div>
          <h1 class="text-2xl font-black text-slate-800 tracking-tight">JWT 密钥管理</h1>
          <p class="text-slate-400 text-sm font-medium mt-1">管理用于签发和验证 JWT 的 RSA 密钥对</p>
          <div class="mt-3 text-xs text-slate-500 space-y-1">
            <p>• 业务系统通过 <code class="bg-slate-100 px-1.5 py-0.5 rounded font-mono">/public/jwks.json</code> 获取公钥</p>
            <p>• 轮换后旧密钥进入 <span class="text-amber-600 font-semibold">24 小时宽限期</span>，期间新旧 token 均有效</p>
          </div>
        </div>
        <el-button type="primary" class="!rounded-2xl font-bold" :loading="rotating" @click="confirmRotate">
          轮换密钥
        </el-button>
      </div>
    </div>

    <!-- 密钥表格 -->
    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
      <div v-if="loading" class="flex items-center justify-center py-16">
        <el-icon class="text-slate-300 animate-spin" :size="36"><Loading /></el-icon>
      </div>

      <el-table v-else :data="keys" empty-text="暂无密钥数据" row-class-name="table-row-hover">
        <el-table-column prop="kid" label="密钥 ID (kid)" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <code class="font-mono text-xs text-slate-700">{{ row.kid }}</code>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag
              :type="row.status === 'active' ? 'success' : row.status === 'grace' ? 'warning' : 'info'"
              size="small"
              round
            >
              {{ statusLabel(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="createdTime" label="创建时间" width="180">
          <template #default="{ row }">{{ formatTime(row.createdTime) }}</template>
        </el-table-column>
        <el-table-column prop="graceUntil" label="宽限期截止" width="180">
          <template #default="{ row }">
            <span v-if="row.graceUntil" class="text-amber-600">{{ formatTime(row.graceUntil) }}</span>
            <span v-else class="text-slate-300">—</span>
          </template>
        </el-table-column>
        <el-table-column prop="retiredTime" label="退役时间" width="180">
          <template #default="{ row }">
            <span v-if="row.retiredTime" class="text-slate-400">{{ formatTime(row.retiredTime) }}</span>
            <span v-else class="text-slate-300">—</span>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { Loading } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { listJwtKeys, rotateJwtKey } from '@/api/auth'
import dayjs from 'dayjs'

const loading = ref(false)
const rotating = ref(false)
const keys = ref([])

const formatTime = (ts) => {
  if (!ts) return '—'
  return dayjs(ts).format('YYYY-MM-DD HH:mm:ss')
}

const statusLabel = (status) => {
  if (status === 'active') return '签发中'
  if (status === 'grace') return '宽限期'
  return '已退役'
}

const fetchKeys = async () => {
  loading.value = true
  try {
    const res = await listJwtKeys()
    keys.value = res?.keys || []
  } catch (e) {
    ElMessage.error('获取密钥列表失败')
  } finally {
    loading.value = false
  }
}

const confirmRotate = async () => {
  try {
    await ElMessageBox.confirm(
      '确定要轮换 JWT 密钥吗？\n\n轮换后：\n• 立即生成新密钥用于签发 token\n• 旧密钥进入 24 小时宽限期，期间签发的 token 仍可正常验签\n• 24 小时后旧密钥自动退役',
      '确认密钥轮换',
      {
        confirmButtonText: '确认轮换',
        cancelButtonText: '取消',
        type: 'warning',
        dangerouslyUseHTMLString: false
      }
    )
    rotating.value = true
    await rotateJwtKey()
    ElMessage.success('密钥轮换成功，旧密钥将在 24 小时后退役')
    await fetchKeys()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(e?.message || '密钥轮换失败')
  } finally {
    rotating.value = false
  }
}

onMounted(() => {
  fetchKeys()
})
</script>

<style scoped>
:deep(.table-row-hover:hover td) {
  background-color: #f8fafc !important;
}
.animate-spin {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
