<template>
  <div class="space-y-6">
    <!-- 页面标题 -->
    <div class="bg-white p-6 rounded-xl border border-slate-50 shadow-soft">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 class="text-2xl font-black text-slate-800 tracking-tight">邀请码管理</h1>
          <p class="text-slate-400 text-sm font-medium mt-1">创建和管理用于用户注册的邀请码</p>
        </div>
        <el-button
          type="primary"
          class="!rounded-2xl !px-6 font-bold"
          @click="openCreateDialog"
        >
          <template #icon><el-icon><Plus /></el-icon></template>
          创建邀请码
        </el-button>
      </div>
    </div>

    <!-- 邀请码表格 -->
    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
      <div v-if="loading" class="flex items-center justify-center py-16">
        <el-icon class="text-slate-300 animate-spin" :size="36"><Loading /></el-icon>
      </div>

      <el-table
        v-else
        :data="codeList"
        empty-text="暂无数据"
      >
        <el-table-column prop="code" label="邀请码" width="200">
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <span class="font-mono font-bold text-primary-600 text-sm">{{ row.code }}</span>
              <el-tooltip content="复制邀请码" placement="top">
                <el-button link type="primary" size="small" @click="copyCode(row.code)">
                  <el-icon><CopyDocument /></el-icon>
                </el-button>
              </el-tooltip>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="备注" min-width="140" show-overflow-tooltip />
        <el-table-column label="使用情况" width="120">
          <template #default="{ row }">
            <span class="text-sm font-medium text-slate-700">
              {{ row.usedCount ?? 0 }} /
              <span :class="row.maxUses === 0 ? 'text-primary-500 font-bold' : 'text-slate-500'">
                {{ row.maxUses === 0 ? '不限' : row.maxUses }}
              </span>
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="90">
          <template #default="{ row }">
            <el-tag
              :type="row.status === 1 ? 'success' : 'danger'"
              size="small"
              round
            >
              {{ row.status === 1 ? '有效' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="expireTime" label="过期时间" width="180">
          <template #default="{ row }">
            <span v-if="row.expireTime" :class="isExpired(row.expireTime) ? 'text-rose-500' : 'text-slate-600'">
              {{ formatTime(row.expireTime) }}
            </span>
            <span v-else class="text-slate-400 text-xs">永不过期</span>
          </template>
        </el-table-column>
        <el-table-column prop="createdTime" label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.createdTime) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <div class="flex items-center">
              <el-button
                :type="row.status === 1 ? 'warning' : 'success'"
                link
                @click="toggleStatus(row)"
              >
                {{ row.status === 1 ? '禁用' : '启用' }}
              </el-button>
              <el-button
                type="danger"
                link
                @click="handleDelete(row)"
              >
                删除
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页 -->
      <div class="flex justify-end px-6 py-4 border-t border-slate-50" v-if="total > 0">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="total"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          background
          @change="fetchCodes"
        />
      </div>
    </div>

    <!-- 创建邀请码 Dialog -->
    <el-dialog
      v-model="createDialogVisible"
      title="创建邀请码"
      width="480px"
      :close-on-click-modal="false"
      class="modern-dialog"
    >
      <el-form
        ref="createFormRef"
        :model="createForm"
        :rules="createRules"
        label-position="top"
        class="space-y-1"
      >
        <el-form-item label="备注说明" prop="description">
          <el-input
            v-model="createForm.description"
            placeholder="请输入邀请码用途说明（可选）"
            class="modern-input"
          />
        </el-form-item>

        <el-form-item label="最大使用次数" prop="max_uses">
          <el-input-number
            v-model="createForm.max_uses"
            :min="0"
            placeholder="0 表示不限"
            class="!w-full"
          />
          <p class="text-xs text-slate-400 mt-1">填写 0 表示不限使用次数</p>
        </el-form-item>

        <el-form-item label="过期时间" prop="expire_time">
          <el-date-picker
            v-model="createForm.expire_time"
            type="datetime"
            placeholder="不填则永不过期"
            format="YYYY-MM-DD HH:mm:ss"
            value-format="x"
            class="!w-full"
          />
        </el-form-item>
      </el-form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <el-button round @click="createDialogVisible = false">取消</el-button>
          <el-button
            type="primary"
            round
            :loading="submitting"
            @click="handleCreate"
          >
            创建
          </el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { Plus, Loading, CopyDocument } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getInviteCodes, createInviteCode, updateInviteCode, deleteInviteCode } from '@/api/tenant'
import dayjs from 'dayjs'

const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const loading = ref(false)
const codeList = ref([])

const createDialogVisible = ref(false)
const submitting = ref(false)
const createFormRef = ref(null)

const createForm = reactive({
  description: '',
  max_uses: 0,
  expire_time: null
})

const createRules = {
  max_uses: [
    { required: true, message: '请填写最大使用次数', trigger: 'blur' }
  ]
}

const formatTime = (ts) => {
  if (!ts) return '—'
  return dayjs(ts).format('YYYY-MM-DD HH:mm:ss')
}

const isExpired = (ts) => {
  if (!ts) return false
  return dayjs(ts).isBefore(dayjs())
}

const fetchCodes = async () => {
  loading.value = true
  try {
    const res = await getInviteCodes({ page: page.value, size: pageSize.value })
    codeList.value = Array.isArray(res) ? res : (res?.list || res?.records || [])
    total.value = res?.total || codeList.value.length
  } catch (e) {
    console.error('获取邀请码列表失败:', e)
    codeList.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

const openCreateDialog = () => {
  createForm.description = ''
  createForm.max_uses = 0
  createForm.expire_time = null
  createDialogVisible.value = true
}

const handleCreate = async () => {
  if (!createFormRef.value) return
  await createFormRef.value.validate(async (valid) => {
    if (!valid) return
    submitting.value = true
    try {
      await createInviteCode({
        description: createForm.description,
        max_uses: createForm.max_uses,
        expire_time: createForm.expire_time || null
      })
      ElMessage.success('邀请码创建成功')
      createDialogVisible.value = false
      fetchCodes()
    } catch (e) {
      console.error('创建邀请码失败:', e)
    } finally {
      submitting.value = false
    }
  })
}

const toggleStatus = async (row) => {
  const newStatus = row.status === 1 ? 0 : 1
  const actionText = newStatus === 1 ? '启用' : '禁用'
  try {
    await ElMessageBox.confirm(
      `确定要${actionText}邀请码 "${row.code}" 吗？`,
      '确认操作',
      { confirmButtonText: '确定', cancelButtonText: '取消', type: 'warning', roundButton: true }
    )
    await updateInviteCode(row.id, { status: newStatus })
    ElMessage.success(`已${actionText}`)
    fetchCodes()
  } catch (e) {
    if (e !== 'cancel') console.error('更新邀请码状态失败:', e)
  }
}

const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除邀请码 "${row.code}" 吗？此操作不可撤销。`,
      '确认删除',
      { confirmButtonText: '确认删除', cancelButtonText: '取消', type: 'warning', roundButton: true }
    )
    await deleteInviteCode(row.id)
    ElMessage.success('删除成功')
    fetchCodes()
  } catch (e) {
    if (e !== 'cancel') console.error('删除邀请码失败:', e)
  }
}

const copyCode = async (code) => {
  try {
    await navigator.clipboard.writeText(code)
    ElMessage.success('邀请码已复制')
  } catch {
    const textarea = document.createElement('textarea')
    textarea.value = code
    textarea.style.position = 'fixed'
    textarea.style.opacity = '0'
    document.body.appendChild(textarea)
    textarea.select()
    document.execCommand('copy')
    document.body.removeChild(textarea)
    ElMessage.success('邀请码已复制')
  }
}

onMounted(() => {
  fetchCodes()
})
</script>

<style scoped>
:deep(.modern-input .el-input__wrapper) {
  border-radius: 12px !important;
  border: 1px solid #f1f5f9 !important;
  box-shadow: none !important;
  background: #f8fafc;
}

:deep(.modern-input .el-input__wrapper.is-focus) {
  border-color: #0ea5e9 !important;
  background: #fff;
  box-shadow: 0 0 0 3px rgba(14, 165, 233, 0.1) !important;
}

:deep(.modern-dialog .el-dialog) {
  border-radius: 24px;
}

:deep(.el-input-number) {
  width: 100%;
}

:deep(.el-input-number .el-input__wrapper) {
  border-radius: 12px !important;
}

.animate-spin {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
