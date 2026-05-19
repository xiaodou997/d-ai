<template>
  <div class="page-container space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
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
        <el-button type="primary" class="!rounded-2xl px-6 h-12 font-bold ml-4" @click="handleRecharge">账户充值</el-button>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft p-2">
      <el-tabs v-model="activeTab" class="modern-tabs" @tab-change="handleTabChange">
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

        <el-tab-pane label="接入应用" name="apps">
          <div class="p-4">
            <div v-if="tenantAppsLoading" class="text-center py-12 text-slate-400">加载中...</div>
            <template v-else>
              <div v-if="tenantAppsIsAll" class="mb-4">
                <el-tag type="success" class="!rounded-2xl font-bold">已授权全部应用</el-tag>
              </div>
              <el-empty v-if="tenantApps.length === 0" description="暂无接入的应用系统" />
              <el-table v-else :data="tenantApps" border stripe class="modern-table">
                <el-table-column prop="appKey" label="AppKey" min-width="180" show-overflow-tooltip />
                <el-table-column prop="appName" label="应用名称" min-width="160" />
                <el-table-column prop="description" label="描述" min-width="200" show-overflow-tooltip>
                  <template #default="{ row }">
                    <span class="text-slate-400 text-xs" v-if="!row.description">—</span>
                    <span v-else>{{ row.description }}</span>
                  </template>
                </el-table-column>
                <el-table-column label="状态" width="90">
                  <template #default="{ row }">
                    <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small" class="rounded-2xl font-bold">
                      {{ row.status === 1 ? '启用' : '禁用' }}
                    </el-tag>
                  </template>
                </el-table-column>
              </el-table>
            </template>
          </div>
        </el-tab-pane>

        <el-tab-pane label="API Keys" name="apiKeys">
          <div class="p-4">
            <div class="flex justify-between items-center mb-4">
              <p class="text-slate-500 text-sm">该租户的 API Keys（不含明文，仅展示前缀）</p>
              <el-button type="primary" class="!rounded-2xl font-bold" @click="openCreateAPIKey">
                <el-icon class="mr-1"><Plus /></el-icon>创建 API Key
              </el-button>
            </div>
            <el-table :data="apiKeys" v-loading="apiKeysLoading" border stripe class="modern-table">
              <el-table-column prop="name" label="名称" min-width="160" />
              <el-table-column prop="key_prefix" label="Key 前缀" width="140">
                <template #default="{ row }">
                  <span class="font-mono text-slate-600">{{ row.key_prefix }}...</span>
                </template>
              </el-table-column>
              <el-table-column label="配额" width="200" align="right">
                <template #default="{ row }">
                  <span v-if="!row.quota_limit" class="text-slate-400">无限制</span>
                  <span v-else class="text-xs">
                    {{ (row.quota_used || 0).toLocaleString() }} / {{ (row.quota_limit || 0).toLocaleString() }}
                  </span>
                </template>
              </el-table-column>
              <el-table-column prop="status" label="状态" width="130">
                <template #default="{ row }">
                  <el-select :model-value="row.status" size="small" @change="changeAPIKeyStatus(row, $event)">
                    <el-option label="启用" value="active" />
                    <el-option label="停用" value="inactive" />
                    <el-option label="禁用" value="disabled" />
                  </el-select>
                </template>
              </el-table-column>
              <el-table-column label="创建时间" width="170">
                <template #default="{ row }">
                  <span class="text-xs text-slate-400">{{ formatTime(row.created_at) }}</span>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </el-tab-pane>

        <el-tab-pane label="用户售价" name="userPrices">
          <div class="p-4">
            <div class="flex justify-between items-center mb-4">
              <p class="text-slate-500 text-sm">该租户对其用户的模型定价（积分/1M tokens）</p>
              <el-button type="primary" class="!rounded-2xl font-bold" :loading="userPricesLoading" @click="openUpsertUserPrice(null)">
                <el-icon class="mr-1"><Plus /></el-icon>设置售价
              </el-button>
            </div>
            <el-table :data="userPrices" v-loading="userPricesLoading" border stripe class="modern-table">
              <el-table-column prop="model_code" label="模型" min-width="220" show-overflow-tooltip />
              <el-table-column prop="capability_type" label="类型" width="110" />
              <el-table-column label="输入价格" width="130" align="right">
                <template #default="{ row }">{{ row.input_price_per_1m ?? '—' }}</template>
              </el-table-column>
              <el-table-column label="输出价格" width="130" align="right">
                <template #default="{ row }">{{ row.output_price_per_1m ?? '—' }}</template>
              </el-table-column>
              <el-table-column label="操作" width="140" fixed="right">
                <template #default="{ row }">
                  <el-button link type="primary" size="small" @click="openUpsertUserPrice(row)">编辑</el-button>
                  <el-popconfirm title="确定删除该售价吗？" @confirm="deleteUserPrice(row)">
                    <template #reference>
                      <el-button link type="danger" size="small">删除</el-button>
                    </template>
                  </el-popconfirm>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </el-tab-pane>

        <el-tab-pane label="财务流水" name="transactions">
          <div class="p-4">
            <el-table :data="transactions" border stripe class="modern-table">
              <el-table-column prop="transactionId" label="交易流水" />
              <el-table-column label="积分" align="right">
                <template #default="{ row }">
                  <span class="font-black text-slate-700">{{ ((row.tenantCredits || 0) + (row.userCredits || 0)).toLocaleString() }} 积分</span>
                </template>
              </el-table-column>
              <el-table-column prop="statusDisplay" label="状态" />
              <el-table-column prop="createdTime" label="时间" />
            </el-table>
          </div>
        </el-tab-pane>
      </el-tabs>
    </div>

    <el-dialog v-model="upsertUserPriceVisible" :title="editingUserPrice ? '编辑用户售价' : '设置用户售价'" width="520px" append-to-body>
      <el-form :model="userPriceForm" label-width="120px">
        <el-form-item label="模型">
          <el-select v-model="userPriceForm.model_id" :disabled="!!editingUserPrice" filterable placeholder="选择模型">
            <el-option
              v-for="g in tenantGrants"
              :key="g.model_id"
              :label="g.model_code"
              :value="g.model_id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="输入价格/1M">
          <el-input-number v-model="userPriceForm.input_price_per_1m" :min="0" :step="10" controls-position="right" />
        </el-form-item>
        <el-form-item label="输出价格/1M">
          <el-input-number v-model="userPriceForm.output_price_per_1m" :min="0" :step="10" controls-position="right" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="upsertUserPriceVisible = false">取消</el-button>
        <el-button type="primary" :loading="userPriceSubmitting" @click="submitUserPrice">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="createAPIKeyVisible" title="创建租户 API Key" width="480px" append-to-body>
      <el-form :model="apiKeyForm" label-width="100px">
        <el-form-item label="名称">
          <el-input v-model="apiKeyForm.name" placeholder="API Key 名称" />
        </el-form-item>
        <el-form-item label="配额上限">
          <el-input-number v-model="apiKeyForm.quota_limit" :min="0" placeholder="0 = 不限制" controls-position="right" />
          <span class="text-xs text-slate-400 ml-2">0 = 不限制</span>
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="apiKeyForm.status">
            <el-option label="启用" value="active" />
            <el-option label="停用" value="inactive" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createAPIKeyVisible = false">取消</el-button>
        <el-button type="primary" :loading="apiKeySubmitting" @click="submitCreateAPIKey">创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showAPIKeyDialog" title="API Key 创建成功" width="520px" append-to-body>
      <el-alert type="warning" :closable="false" class="mb-4">
        请立即复制 API Key，此后将无法再次查看完整密钥。
      </el-alert>
      <div class="flex items-center gap-2 bg-slate-50 p-3 rounded-xl font-mono text-sm break-all">
        <span class="flex-1 select-all">{{ generatedAPIKey }}</span>
        <el-button size="small" @click="copyAPIKey">复制</el-button>
      </div>
      <template #footer>
        <el-button type="primary" @click="showAPIKeyDialog = false">已保存，关闭</el-button>
      </template>
    </el-dialog>

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
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { OfficeBuilding, Plus } from '@element-plus/icons-vue'
import {
  queryUsers, queryTransactions, queryTenants,
  listTenantUsers, createTenantUser, updateTenantUserStatus, resetTenantUserPassword,
  listTenantApps
} from '@/api/tenant'
import {
  listTenantAPIKeys,
  createTenantAPIKey,
  updateTenantAPIKeyStatus,
  listTenantModelGrants,
  listTenantUserPrices,
  upsertTenantUserPrice,
  deleteTenantUserPrice
} from '@/api/aiGateway'

const route = useRoute()
const router = useRouter()
const tenantId = route.params.id
const activeTab = ref('orgUsers')

const tenantInfo = ref({})
const users = ref([])
const transactions = ref([])

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

const tenantApps = ref([])
const tenantAppsIsAll = ref(false)
const tenantAppsLoading = ref(false)
const tenantAppsLoaded = ref(false)

const apiKeys = ref([])
const apiKeysLoading = ref(false)
const apiKeysLoaded = ref(false)
const createAPIKeyVisible = ref(false)
const apiKeySubmitting = ref(false)
const showAPIKeyDialog = ref(false)
const generatedAPIKey = ref('')
const apiKeyForm = reactive({ name: '', quota_limit: 0, status: 'active' })

const userPrices = ref([])
const userPricesLoading = ref(false)
const userPricesLoaded = ref(false)
const tenantGrants = ref([])
const upsertUserPriceVisible = ref(false)
const userPriceSubmitting = ref(false)
const editingUserPrice = ref(null)
const userPriceForm = reactive({ model_id: '', input_price_per_1m: 0, output_price_per_1m: 0 })

const handleRecharge = () => {
  router.push(`/finance/recharge?tenantId=${tenantId}&tenantName=${encodeURIComponent(tenantInfo.value.tenantName || '')}`)
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

const fetchTransactions = async () => {
  const data = await queryTransactions({ accountId: tenantId, page: 1, size: 20 })
  transactions.value = data.records
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

const fetchTenantApps = async () => {
  if (tenantAppsLoaded.value) return
  tenantAppsLoading.value = true
  try {
    const data = await listTenantApps(tenantId)
    tenantApps.value = data.clientServices || []
    tenantAppsIsAll.value = data.isWildcard || false
    tenantAppsLoaded.value = true
  } finally {
    tenantAppsLoading.value = false
  }
}

const fetchAPIKeys = async () => {
  if (apiKeysLoaded.value) return
  apiKeysLoading.value = true
  try {
    apiKeys.value = await listTenantAPIKeys(tenantId) || []
    apiKeysLoaded.value = true
  } finally {
    apiKeysLoading.value = false
  }
}

const openCreateAPIKey = () => {
  Object.assign(apiKeyForm, { name: '', quota_limit: 0, status: 'active' })
  createAPIKeyVisible.value = true
}

const submitCreateAPIKey = async () => {
  if (!apiKeyForm.name) { ElMessage.warning('请输入名称'); return }
  apiKeySubmitting.value = true
  try {
    const res = await createTenantAPIKey(tenantId, {
      name: apiKeyForm.name,
      quota_limit: apiKeyForm.quota_limit || undefined,
      status: apiKeyForm.status
    })
    if (res?.api_key) {
      generatedAPIKey.value = res.api_key
      showAPIKeyDialog.value = true
    }
    createAPIKeyVisible.value = false
    apiKeysLoaded.value = false
    await fetchAPIKeys()
    ElMessage.success('API Key 已创建')
  } finally {
    apiKeySubmitting.value = false
  }
}

const changeAPIKeyStatus = async (row, status) => {
  try {
    await updateTenantAPIKeyStatus(tenantId, row.id, status)
    row.status = status
  } catch {
    ElMessage.error('状态更新失败')
  }
}

const copyAPIKey = async () => {
  try {
    await navigator.clipboard.writeText(generatedAPIKey.value)
    ElMessage.success('已复制')
  } catch { ElMessage.warning('请手动复制') }
}

const fetchUserPrices = async () => {
  if (userPricesLoaded.value) return
  userPricesLoading.value = true
  try {
    const [prices, grants] = await Promise.all([
      listTenantUserPrices(tenantId),
      tenantGrants.value.length ? Promise.resolve(tenantGrants.value) : listTenantModelGrants(tenantId)
    ])
    userPrices.value = prices || []
    if (Array.isArray(grants)) tenantGrants.value = grants
    userPricesLoaded.value = true
  } finally {
    userPricesLoading.value = false
  }
}

const openUpsertUserPrice = (row) => {
  editingUserPrice.value = row
  if (row) {
    Object.assign(userPriceForm, {
      model_id: row.model_id,
      input_price_per_1m: row.input_price_per_1m ?? 0,
      output_price_per_1m: row.output_price_per_1m ?? 0
    })
  } else {
    Object.assign(userPriceForm, { model_id: '', input_price_per_1m: 0, output_price_per_1m: 0 })
  }
  upsertUserPriceVisible.value = true
}

const submitUserPrice = async () => {
  if (!userPriceForm.model_id) { ElMessage.warning('请选择模型'); return }
  userPriceSubmitting.value = true
  try {
    await upsertTenantUserPrice(tenantId, userPriceForm.model_id, {
      input_price_per_1m: userPriceForm.input_price_per_1m,
      output_price_per_1m: userPriceForm.output_price_per_1m
    })
    ElMessage.success('已保存')
    upsertUserPriceVisible.value = false
    userPricesLoaded.value = false
    await fetchUserPrices()
  } finally {
    userPriceSubmitting.value = false
  }
}

const deleteUserPrice = async (row) => {
  try {
    await deleteTenantUserPrice(tenantId, row.model_id)
    ElMessage.success('已删除')
    userPricesLoaded.value = false
    await fetchUserPrices()
  } catch { ElMessage.error('删除失败') }
}

const handleTabChange = (tab) => {
  if (tab === 'apps') fetchTenantApps()
  if (tab === 'apiKeys') fetchAPIKeys()
  if (tab === 'userPrices') fetchUserPrices()
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
    await updateTenantUserStatus(row.userId, 'disabled')
    ElMessage.success('已停用')
    fetchOrgUsers()
  } catch (err) {
    if (err !== 'cancel') ElMessage.error('操作失败')
  }
}

const handleEnableOrgUser = async (row) => {
  try {
    await ElMessageBox.confirm(`确定启用用户「${row.username}」吗？`, '启用确认', { type: 'warning' })
    await updateTenantUserStatus(row.userId, 'active')
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
  fetchTransactions()
  fetchOrgUsers()
})
</script>

<style scoped>
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
}
:deep(.el-tabs__item.is-active) {
  color: #6366f1;
}
</style>
