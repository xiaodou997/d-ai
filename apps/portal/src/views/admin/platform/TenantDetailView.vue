<!--
  租户详情 — 1:1 搬运自 v1/platform/platform-admin/src/views/Tenant/TenantDetail.vue。
  2 Tab：组织用户 / 关联用户。
  适配：axios → platformAdminApi；头部用 getTenant + getAccountBalance；
  重构：页面骨架迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       详情页无筛选/分页槽,标签页与各 Tab 内容置于同卡 body 内 24px 容器排布），
       组织用户列表接入 useListPage；请求参数与筛选语义保持不变，弹窗仍为 element-plus。
       关联用户固定拉取前 100 条（接口限制），不渲染分页器。
-->
<template>
  <div class="tenant-detail-page">
    <PortalPagePanel
      :icon="Building2"
      :breadcrumbs="[
        { label: '用户中心' },
        { label: '业务管理' },
        { label: '租户管理', to: '/admin/organization/tenants' },
        { label: '租户详情' }
      ]"
      :description="headerDescription"
    >
      <template #actions>
        <DsTag v-if="tenantInfo" :tone="tenantInfo.status === 1 ? 'positive' : 'danger'">
          {{ tenantInfo.statusDisplay }}
        </DsTag>
        <el-button @click="goTenantPolicy">
          <SlidersHorizontal class="td-button-icon" />平台策略
        </el-button>
        <el-button @click="accountDrawerVisible = true">账户详情</el-button>
        <el-button type="primary" @click="handleRecharge">账户充值</el-button>
      </template>

      <!-- 详情主体:body 无内边距,用 24px 容器承载标签页与各 Tab 内容 -->
      <div class="td-body">
      <!-- 标签页 -->
      <div class="td-tabs-header">
        <DsTabs v-model="activeTab" :tabs="tabs" />
      </div>

      <!-- 组织用户 -->
      <div v-show="activeTab === 'orgUsers'" class="td-pane">
        <div class="td-toolbar">
          <div class="td-toolbar__search">
            <el-input
              v-model="orgUserQuery.keyword"
              placeholder="搜索用户名"
              clearable
              class="td-search-input"
              @keyup.enter="searchOrgUsers"
            />
            <el-button type="primary" @click="searchOrgUsers">搜索</el-button>
          </div>
          <el-button type="primary" @click="handleCreateOrgUser">
            <el-icon class="td-button-icon"><Plus /></el-icon>创建用户
          </el-button>
        </div>
        <DsTable
          :frame="false"
          :columns="orgUserColumns"
          :rows="orgUsers"
          row-key="userId"
          :loading="orgUsersLoading"
          empty-title="暂无组织用户"
        >
          <template #cell-email="{ row }">
            <span v-if="!row.email" class="td-muted">—</span>
            <span v-else>{{ row.email }}</span>
          </template>
          <template #cell-status="{ row }">
            <DsTag :tone="row.status === 1 ? 'positive' : 'neutral'">
              {{ row.statusText }}
            </DsTag>
          </template>
          <template #cell-createdTime="{ row }">
            <span class="td-time">{{ formatTime(row.createdTime) }}</span>
          </template>
          <template #cell-actions="{ row }">
            <el-button v-if="row.status === 1" link type="warning" @click="handleToggleOrgUser(row, 'disabled')">停用</el-button>
            <el-button v-else link type="success" @click="handleToggleOrgUser(row, 'active')">启用</el-button>
            <el-popconfirm title="确定重置该用户密码为 123456 吗？" @confirm="handleResetOrgUserPwd(row)">
              <template #reference>
                <el-button link type="primary">重置密码</el-button>
              </template>
            </el-popconfirm>
          </template>
        </DsTable>
        <div class="td-pager">
          <DsPagination
            :page="orgUserPage"
            :page-size="orgUserPageSize"
            :total="orgUserTotal"
            @update:page="handleOrgUserPage"
            @update:page-size="handleOrgUserSize"
          />
        </div>
      </div>

      <!-- 关联用户 -->
      <div v-show="activeTab === 'users'" class="td-pane">
        <p class="td-note">该租户下的所有终端用户（由业务系统同步，最多展示前 100 条）</p>
        <DsTable
          :frame="false"
          :columns="userColumns"
          :rows="users"
          row-key="userId"
          :loading="usersLoading"
          empty-title="暂无关联用户"
        >
          <template #cell-nickname="{ row }">
            <span>{{ row.nickname || '—' }}</span>
          </template>
          <template #cell-status="{ row }">
            <DsTag :tone="row.status === 1 ? 'positive' : 'neutral'">
              {{ endUserStatus(row.status) }}
            </DsTag>
          </template>
          <template #cell-createdTime="{ row }">
            <span class="td-time">{{ formatTime(row.createdTime) }}</span>
          </template>
        </DsTable>
      </div>

      </div>
    </PortalPagePanel>

    <AccountOverviewDrawer
      :open="accountDrawerVisible"
      :account-type="1"
      :account-id="tenantId"
      :account-name="tenantInfo?.tenantName"
      @close="accountDrawerVisible = false"
    />

    <RechargeDialog
      v-model="rechargeDialogVisible"
      title="租户充值"
      target-type-label="租户"
      :target-name="tenantInfo?.tenantName || ''"
      :target-identity="`租户 ID ${tenantId}`"
      :target-balance-usd="credits"
      :submitting="rechargeSubmitting"
      @submit="submitTenantRecharge"
    />

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

<script setup lang="ts">
import { computed, reactive, ref, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { Building2, SlidersHorizontal } from 'lucide-vue-next'
import { PortalPagePanel, useListPage } from '@/platform'
import AccountOverviewDrawer from '@/components/AccountOverviewDrawer.vue'
import RechargeDialog from '@/components/RechargeDialog.vue'
import type { RechargeFormPayload } from '@/components/recharge'
import {
  DsPagination,
  DsTable,
  DsTabs,
  DsTag,
  type DsTableColumn
} from '@/shared/ui'
import { platformAdminApi } from '@/api/platformAdmin'
import type { TenantDetailOutput } from '@/api/types/admin'
import { formatDisplayUSD } from '@/shared/currency'

const route = useRoute()
const router = useRouter()
const tenantId = String(route.params.id || '')
const activeTab = ref(route.query.tab === 'users' ? 'users' : 'orgUsers')

const tabs = [
  { key: 'orgUsers', label: '组织用户' },
  { key: 'users', label: '关联用户' }
]

const tenantInfo = ref<TenantDetailOutput | null>(null)
const credits = ref(0)
const accountDrawerVisible = ref(false)
const rechargeDialogVisible = ref(false)
const rechargeSubmitting = ref(false)

// 页头副标题集中展示关键信息
const headerDescription = computed(
  () => `租户 ID：${tenantId} · 余额：${formatDisplayUSD(credits.value)}`
)

const orgUserColumns: DsTableColumn[] = [
  { key: 'userId', title: '用户 ID', mono: true },
  { key: 'username', title: '用户名' },
  { key: 'email', title: '邮箱' },
  { key: 'status', title: '状态', width: 100 },
  { key: 'createdTime', title: '创建时间', width: 170 },
  { key: 'actions', title: '操作', width: 190 }
]

const userColumns: DsTableColumn[] = [
  { key: 'userId', title: '用户 ID', mono: true },
  { key: 'username', title: '用户名' },
  { key: 'nickname', title: '昵称' },
  { key: 'status', title: '状态', width: 100 },
  { key: 'createdTime', title: '注册时间', width: 170 }
]

// 组织用户：关键词筛选 + 分页，接入 useListPage
const {
  rows: orgUsers,
  total: orgUserTotal,
  loading: orgUsersLoading,
  page: orgUserPage,
  pageSize: orgUserPageSize,
  query: orgUserQuery,
  refresh: refreshOrgUsers,
  search: searchOrgUsers,
  handlePageChange: handleOrgUserPage,
  handlePageSizeChange: handleOrgUserSize
} = useListPage<{ keyword: string }, any>({
  initialQuery: { keyword: '' },
  pageSize: 20,
  fetcher: async (params) => {
    const data = await platformAdminApi.listTenantUsers({
      tenantId,
      page: params.page,
      size: params.pageSize,
      keyword: params.keyword.trim() || undefined
    })
    return { items: data.items || [], total: data.total || 0 }
  }
})

const createOrgUserVisible = ref(false)
const createOrgUserSubmitting = ref(false)
const createOrgUserFormRef = ref<any>(null)
const createOrgUserForm = reactive({ username: '', email: '' })
const createOrgUserRules = { username: [{ required: true, message: '请输入用户名', trigger: 'blur' }] }

// 关联用户：接口无分页参数，固定拉取前 100 条，故不渲染分页器
const users = ref<any[]>([])
const usersLoading = ref(false)
const usersLoaded = ref(false)

const formatTime = (ts?: number) => (ts ? new Date(ts).toLocaleString('zh-CN', { hour12: false }) : '—')
const endUserStatus = (s: number) => (({ 1: '正常', 2: '禁用', 3: '锁定', 4: '级联停用' } as any)[s] || '未知')

const handleRecharge = () => {
  rechargeDialogVisible.value = true
}

const submitTenantRecharge = async (payload: RechargeFormPayload) => {
  if (!tenantInfo.value) return

  try {
    await ElMessageBox.confirm(
      `确认对租户「${tenantInfo.value.tenantName}」执行${payload.expireTime ? '【限时】' : '【永久】'}入账操作？`,
      '财务安全确认',
      { confirmButtonText: '确定入账', cancelButtonText: '取消', roundButton: true, type: 'warning' }
    )
  } catch {
    return
  }

  rechargeSubmitting.value = true
  try {
    await platformAdminApi.createRecharge({ packageType: 1, tenantId, ...payload })
    ElMessage.success(`已成功充值 $${(payload.amountMicroUsd / 1_000_000).toLocaleString()}`)
    rechargeDialogVisible.value = false
    void fetchBalance()
  } catch (error: any) {
    ElMessage.error(error?.message || '充值失败')
  } finally {
    rechargeSubmitting.value = false
  }
}

const goTenantPolicy = () => {
  router.push(`/admin/organization/tenants/${tenantId}/policy`)
}

const fetchTenantInfo = async () => {
  try {
    tenantInfo.value = await platformAdminApi.getTenant(tenantId)
  } catch {
    ElMessage.error('加载租户信息失败')
  }
}

const fetchBalance = async () => {
  try {
    const res = await platformAdminApi.getAccountBalance({ accountType: 1, accountId: tenantId })
    credits.value = res.availableUsd
  } catch {
    credits.value = 0
  }
}

const fetchUsers = async () => {
  if (usersLoaded.value) return
  usersLoading.value = true
  try {
    const data = await platformAdminApi.listEndUsers({ tenantId, page: 1, size: 100 })
    users.value = data.items || []
    usersLoaded.value = true
  } finally {
    usersLoading.value = false
  }
}

const handleTabChange = (name: string) => {
  if (name === 'users') fetchUsers()
  const { tab: _tab, ...query } = route.query
  void router.replace({ query: name === 'users' ? { ...query, tab: 'users' } : query })
}

watch(activeTab, handleTabChange)

const handleCreateOrgUser = () => {
  createOrgUserForm.username = ''
  createOrgUserForm.email = ''
  createOrgUserVisible.value = true
}

const submitCreateOrgUser = async () => {
  if (!createOrgUserFormRef.value) return
  await createOrgUserFormRef.value.validate(async (valid: boolean) => {
    if (!valid) return
    createOrgUserSubmitting.value = true
    try {
      await platformAdminApi.createTenantUser({ tenantId, username: createOrgUserForm.username, email: createOrgUserForm.email || undefined })
      ElMessage.success(`用户「${createOrgUserForm.username}」已创建，默认密码：123456`)
      createOrgUserVisible.value = false
      refreshOrgUsers()
    } catch (err: any) {
      ElMessage.error(err?.message || '创建失败')
    } finally {
      createOrgUserSubmitting.value = false
    }
  })
}

const handleToggleOrgUser = async (row: any, status: 'active' | 'disabled') => {
  const action = status === 'active' ? '启用' : '停用'
  try {
    await ElMessageBox.confirm(`确定${action}用户「${row.username}」吗？`, `${action}确认`, { type: 'warning' })
    await platformAdminApi.updateTenantUserStatus(row.userId, status)
    ElMessage.success(`已${action}`)
    refreshOrgUsers()
  } catch (err) {
    if (err !== 'cancel') ElMessage.error('操作失败')
  }
}

const handleResetOrgUserPwd = async (row: any) => {
  try {
    await platformAdminApi.resetTenantUserPassword(row.userId)
    ElMessage.success('密码已重置为 123456')
  } catch {
    ElMessage.error('重置失败')
  }
}

onMounted(() => {
  fetchTenantInfo()
  fetchBalance()
  if (activeTab.value === 'users') fetchUsers()
})
</script>

<style scoped>
.tenant-detail-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 详情主体:24px 容器承载标签页与各 Tab 内容(面板 body 本身无内边距) */
.td-body {
  padding: 24px;
}

/* tabs 头部与面板内容对齐(24px 容器内无需左右留白) */
.td-tabs-header {
  padding: 0 0 6px;
}

/* tab 面板内边距(24px 容器内仅保留顶部间距) */
.td-pane {
  padding: 16px 0 0;
}

/* tab 内工具条 */
.td-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}

.td-toolbar__search {
  display: flex;
  gap: 8px;
}

.td-search-input {
  width: 224px;
}

.td-button-icon {
  width: 16px;
  height: 16px;
  margin-right: 4px;
}

/* 组织用户分页器右对齐 */
.td-pager {
  display: flex;
  justify-content: flex-end;
  padding-top: 16px;
}

.td-note {
  margin: 0 0 16px;
  font-size: 13px;
  color: var(--ds-muted);
}

.td-muted {
  font-size: 12px;
  color: var(--ds-faint);
}

.td-time {
  font-size: 12px;
  color: var(--ds-faint);
}

/* USD 金额：等宽数字 + 加粗 */
.td-num {
  font-variant-numeric: tabular-nums;
  font-weight: 700;
  color: var(--ds-ink-soft);
}

.td-loading {
  padding: 48px 0;
  text-align: center;
  color: var(--ds-faint);
}

.td-wildcard {
  margin-bottom: 16px;
}
</style>
