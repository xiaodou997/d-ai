<template>
  <div class="page-container space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <!-- 头部信息 -->
    <div class="bg-white p-8 rounded-2xl border border-slate-50 shadow-soft flex justify-between items-start">
      <div class="flex items-center">
        <div class="w-16 h-16 bg-indigo-50 rounded-2xl flex items-center justify-center text-indigo-600 mr-6">
          <el-icon :size="32"><OfficeBuilding /></el-icon>
        </div>
        <div>
          <div class="flex items-center gap-3">
            <h1 class="text-2xl font-black text-slate-800">{{ tenantInfo.tenantName || '加载中...' }}</h1>
            <el-tag :type="tenantInfo.status === 1 ? 'success' : 'danger'" class="rounded-2xl font-bold">
              {{ tenantInfo.statusDisplay }}
            </el-tag>
          </div>
          <p class="text-slate-400 font-medium mt-1">租户 ID: {{ tenantId }}</p>
        </div>
      </div>
      <div class="flex gap-4">
        <div class="text-right">
          <p class="text-[10px] font-black text-slate-400 uppercase">平台积分</p>
          <p class="text-2xl font-black text-slate-800">{{ (tenantInfo.credits || 0).toLocaleString() }} 积分</p>
        </div>
      </div>
    </div>

    <!-- 标签页内容 -->
    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft p-2">
      <el-tabs v-model="activeTab" class="modern-tabs">
        <!-- 组织用户 Tab -->
        <el-tab-pane label="组织用户" name="orgUsers">
          <div class="p-4">
            <div class="flex justify-between items-center mb-4">
              <div class="flex gap-2">
                <el-input v-model="orgUserKeyword" placeholder="搜索用户名" clearable class="modern-input w-56" @keyup.enter="fetchOrgUsers" />
                <el-button type="primary" class="!rounded-2xl" @click="fetchOrgUsers">搜索</el-button>
              </div>
              <el-button type="primary" class="!rounded-2xl font-bold" @click="handleCreateOrgUser">
                <el-icon class="mr-1"><Plus /></el-icon>创建用户
              </el-button>
            </div>
            <el-table :data="orgUsers" v-loading="orgUsersLoading" border stripe class="modern-table">
              <el-table-column prop="userId" label="用户 ID" min-width="180" show-overflow-tooltip />
              <el-table-column prop="username" label="用户名" min-width="130" />
              <el-table-column prop="email" label="邮箱" min-width="200" show-overflow-tooltip>
                <template #default="{ row }">
                  <span class="text-slate-400 text-xs" v-if="!row.email">—</span>
                  <span v-else>{{ row.email }}</span>
                </template>
              </el-table-column>
              <el-table-column label="状态" width="100">
                <template #default="{ row }">
                  <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small" class="rounded-2xl font-bold">
                    {{ row.statusText }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="创建时间" width="170">
                <template #default="{ row }">
                  <span class="text-xs text-slate-400">{{ formatTime(row.createdTime) }}</span>
                </template>
              </el-table-column>
              <el-table-column label="操作" fixed="right" width="190">
                <template #default="{ row }">
                  <el-button link type="warning" @click="handleDisableOrgUser(row)" v-if="row.status === 1">停用</el-button>
                  <el-button link type="success" @click="handleEnableOrgUser(row)" v-else>启用</el-button>
                  <el-popconfirm title="确定重置该用户密码为 123456 吗？" @confirm="handleResetOrgUserPwd(row)">
                    <template #reference>
                      <el-button link type="primary">重置密码</el-button>
                    </template>
                  </el-popconfirm>
                </template>
              </el-table-column>
            </el-table>
            <div class="pt-4 flex justify-end">
              <el-pagination
                v-model:current-page="orgUserPagination.page"
                v-model:page-size="orgUserPagination.size"
                :total="orgUserPagination.total"
                layout="total, prev, pager, next"
                @current-change="fetchOrgUsers"
                class="modern-pagination"
              />
            </div>
          </div>
        </el-tab-pane>

        <!-- 关联用户 Tab -->
        <el-tab-pane label="关联用户" name="users">
          <div class="p-4">
            <p class="text-slate-500 text-sm mb-4">该租户下的所有终端用户（由业务系统同步）</p>
            <el-table :data="users" border stripe class="modern-table">
              <el-table-column prop="userId" label="用户 ID" />
              <el-table-column prop="username" label="用户名" />
              <el-table-column prop="nickname" label="昵称" />
              <el-table-column prop="statusDisplay" label="状态" />
              <el-table-column prop="createdTime" label="注册时间" />
            </el-table>
          </div>
        </el-tab-pane>

      </el-tabs>
    </div>

    <!-- 创建组织用户弹窗 -->
    <el-dialog v-model="createOrgUserVisible" title="创建组织用户" width="440px" append-to-body>
      <el-form :model="createOrgUserForm" :rules="createOrgUserRules" ref="createOrgUserFormRef" label-position="top">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="createOrgUserForm.username" placeholder="登录用户名" />
        </el-form-item>
        <el-form-item label="邮箱" prop="email">
          <el-input v-model="createOrgUserForm.email" placeholder="邮箱（可选）" />
        </el-form-item>
        <p class="text-xs text-amber-500">默认密码：123456，请提醒用户及时修改</p>
      </el-form>
      <template #footer>
        <el-button @click="createOrgUserVisible = false">取消</el-button>
        <el-button type="primary" :loading="createOrgUserSubmitting" @click="submitCreateOrgUser">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { OfficeBuilding, Plus } from '@element-plus/icons-vue'
import {
  queryUsers, queryTenants,
  listTenantUsers, createTenantUser, disableTenantUser, enableTenantUser, resetTenantUserPassword
} from '@/api/tenant'

const route = useRoute()
const tenantId = route.params.id
const activeTab = ref('orgUsers')

const tenantInfo = ref({})
const users = ref([])

// 组织用户
const orgUsers = ref([])
const orgUsersLoading = ref(false)
const orgUserKeyword = ref('')
const orgUserPagination = reactive({ page: 1, size: 20, total: 0 })
const createOrgUserVisible = ref(false)
const createOrgUserSubmitting = ref(false)
const createOrgUserFormRef = ref(null)
const createOrgUserForm = reactive({ username: '', email: '' })
const createOrgUserRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }]
}

const fetchTenantInfo = async () => {
  const data = await queryTenants({ tenantId })
  if (data.records && data.records.length > 0) {
    tenantInfo.value = data.records[0]
  }
}

const fetchUsers = async () => {
  const data = await queryUsers({ tenantId, page: 1, size: 100 })
  users.value = data.records
}

const fetchOrgUsers = async () => {
  orgUsersLoading.value = true
  try {
    const data = await listTenantUsers({ tenantId, page: orgUserPagination.page, size: orgUserPagination.size })
    orgUsers.value = data.records || []
    orgUserPagination.total = data.total || 0
  } finally {
    orgUsersLoading.value = false
  }
}

const handleCreateOrgUser = () => {
  createOrgUserForm.username = ''
  createOrgUserForm.email = ''
  createOrgUserVisible.value = true
}

const submitCreateOrgUser = async () => {
  if (!createOrgUserFormRef.value) return
  await createOrgUserFormRef.value.validate(async (valid) => {
    if (!valid) return
    createOrgUserSubmitting.value = true
    try {
      await createTenantUser({ tenantId, username: createOrgUserForm.username, email: createOrgUserForm.email })
      ElMessage.success(`用户「${createOrgUserForm.username}」已创建，默认密码：123456`)
      createOrgUserVisible.value = false
      fetchOrgUsers()
    } catch (err) {
      ElMessage.error(err.message || '创建失败')
    } finally {
      createOrgUserSubmitting.value = false
    }
  })
}

const handleDisableOrgUser = async (row) => {
  try {
    await ElMessageBox.confirm(`确定停用用户「${row.username}」吗？`, '停用确认', { type: 'warning' })
    await disableTenantUser(row.userId)
    ElMessage.success('已停用')
    fetchOrgUsers()
  } catch (err) {
    if (err !== 'cancel') ElMessage.error('操作失败')
  }
}

const handleEnableOrgUser = async (row) => {
  try {
    await ElMessageBox.confirm(`确定启用用户「${row.username}」吗？`, '启用确认', { type: 'warning' })
    await enableTenantUser(row.userId)
    ElMessage.success('已启用')
    fetchOrgUsers()
  } catch (err) {
    if (err !== 'cancel') ElMessage.error('操作失败')
  }
}

const handleResetOrgUserPwd = async (row) => {
  try {
    await resetTenantUserPassword(row.userId)
    ElMessage.success('密码已重置为 123456')
  } catch {
    ElMessage.error('重置失败')
  }
}

const formatTime = (ts) => {
  if (!ts) return '—'
  return new Date(ts).toLocaleString('zh-CN', { hour12: false })
}

onMounted(() => {
  fetchTenantInfo()
  fetchUsers()
  fetchOrgUsers()
})
</script>

<style scoped lang="scss">
:deep(.el-tabs__header) {
  margin-bottom: 0;
  border-bottom: 1px solid #f8fafc;
  padding: 0 20px;
}
:deep(.el-tabs__nav-wrap::after) {
  display: none;
}
:deep(.el-tabs__item) {
  height: 60px;
  font-weight: 700;
  color: #64748b;
  &.is-active {
    color: #6366f1;
  }
}
</style>
