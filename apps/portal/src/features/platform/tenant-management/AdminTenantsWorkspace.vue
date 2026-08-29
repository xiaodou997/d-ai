<!--
  管理端租户管理 — 1:1 搬运自 v1/platform/platform-admin/src/views/Tenant/TenantList.vue。
  适配：axios api → platformAdminApi（listTenants/createTenant/updateTenant/deleteTenant/
       updateTenantStatus + listGovClients）；data.records → data.items；
       navigateBilling → router.push('/...')；编辑时 isWildcard→clientIds=['ALL']。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       筛选/表格/分页同卡）;数据接入 useListPage,弹窗仍为 element-plus。
-->
<template>
  <div class="tenants-page">
    <PortalPagePanel
      :icon="Building2"
      :breadcrumbs="[{ label: '用户中心' }, { label: '业务管理' }, { label: '租户管理' }]"
      description="管理平台租户，支持创建、编辑、充值、状态调整与关联业务系统维护。"
    >
      <template #actions>
        <el-button type="primary" @click="handleCreate">
          <el-icon class="mr-1"><Plus /></el-icon>
          创建租户
        </el-button>
      </template>

      <template #filters>
        <DsFilterBar>
          <DsFilterField label="关键词">
            <el-input
              v-model="query.keyword"
              placeholder="搜索租户名称、联系人或邮箱"
              clearable
              class="tenants-search-input"
              @keyup.enter="search"
            >
              <template #prefix>
                <Search class="tenants-search-input__icon" />
              </template>
            </el-input>
          </DsFilterField>
          <DsFilterField label="状态">
            <el-select v-model="query.status" placeholder="全部状态" clearable class="tenants-status-select">
              <el-option label="全部" value="" />
              <el-option label="正常" :value="1" />
              <el-option label="停用" :value="2" />
              <el-option label="欠费封禁" :value="3" />
            </el-select>
          </DsFilterField>

          <template #actions>
            <el-button type="primary" @click="search">
              <Search class="tenants-button-icon" />
              筛选
            </el-button>
            <el-button @click="resetQuery">
              <RefreshRight class="tenants-button-icon" />
              重置
            </el-button>
          </template>
        </DsFilterBar>
      </template>

      <DsTable
        :frame="false"
        :columns="columns"
        :rows="rows"
        row-key="tenantId"
        :loading="loading"
        empty-title="暂无租户"
      >
        <template #cell-tenantName="{ row }">
          <button type="button" class="tenants-name-link" @click="goTenantDetail(row.tenantId)">
            {{ row.tenantName }}
          </button>
        </template>
        <template #cell-status="{ row }">
          <el-tooltip
            :content="row.status === 3 ? '欠费封禁状态不可手动切换' : row.status === 1 ? '点击停用租户' : '点击启用租户'"
            placement="top"
          >
            <el-switch
              :model-value="row.status === 1"
              :loading="isStatusUpdating(row.tenantId)"
              :disabled="row.status === 3 || isStatusUpdating(row.tenantId)"
              :aria-label="`${row.tenantName}状态`"
              inline-prompt
              active-text="启"
              :inactive-text="row.status === 3 ? '封' : '停'"
              @change="handleStatusChange(row, Boolean($event))"
            />
          </el-tooltip>
        </template>
        <template #cell-balance="{ row }">
          <span class="tenants-num tenants-credits" :class="{ 'tenants-credits--danger': row.status === 3 }">
            {{ formatDisplayUSD(row.balanceUsd) }}
          </span>
        </template>
        <template #cell-userCount="{ row }">
          <button
            type="button"
            class="tenants-count-link tenants-num"
            :aria-label="`查看${row.tenantName}的关联用户`"
            @click="goAssociatedUsers(row.tenantId)"
          >
            {{ row.userCount ?? 0 }}
          </button>
        </template>
        <template #cell-createdTime="{ row }">
          <span class="tenants-time">{{ formatTime(row.createdTime) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <el-button link type="primary" @click="openAccount(row)">账户</el-button>
          <el-button link type="primary" @click="goTenantPolicy(row.tenantId)">策略</el-button>
          <el-button link type="primary" @click="handleEdit(row)">编辑</el-button>
          <el-button link type="success" @click="handleRecharge(row)">充值</el-button>
          <el-popconfirm title="确定删除该租户吗？" @confirm="handleDelete(row.tenantId)">
            <template #reference>
              <el-button link type="danger">删除</el-button>
            </template>
          </el-popconfirm>
        </template>
      </DsTable>

      <template #pagination>
        <DsPagination
          :page="page"
          :page-size="pageSize"
          :total="total"
          @update:page="handlePageChange"
          @update:page-size="handlePageSizeChange"
        />
      </template>
    </PortalPagePanel>

    <AccountOverviewDrawer
      :open="accountDrawerVisible"
      :account-type="1"
      :account-id="accountTarget?.tenantId || ''"
      :account-name="accountTarget?.tenantName"
      @close="accountDrawerVisible = false"
    />

    <RechargeDialog
      v-model="rechargeDialogVisible"
      title="租户充值"
      target-type-label="租户"
      :target-name="rechargeTarget?.tenantName || ''"
      :target-identity="rechargeTarget ? `租户 ID ${rechargeTarget.tenantId}` : ''"
      :target-balance-usd="rechargeTarget?.balanceUsd ?? 0"
      :submitting="rechargeSubmitting"
      @submit="submitTenantRecharge"
    />

    <!-- 租户表单对话框 -->
    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑租户' : '创建租户'" width="560px" append-to-body>
      <el-form :model="form" :rules="rules" ref="formRef" label-position="top">
        <el-form-item label="租户名称" prop="tenantName">
          <el-input v-model="form.tenantName" placeholder="请输入租户名称" />
        </el-form-item>
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="联系人" prop="contactPerson">
            <el-input v-model="form.contactPerson" placeholder="姓名" />
          </el-form-item>
          <el-form-item label="联系邮箱" prop="contactEmail">
            <el-input v-model="form.contactEmail" placeholder="email@example.com" />
          </el-form-item>
        </div>
        <el-form-item label="状态" prop="status">
          <el-select v-model="form.status" class="w-full">
            <el-option label="正常启用" :value="1" />
            <el-option label="初始停用" :value="2" />
          </el-select>
        </el-form-item>

        <template v-if="!isEdit">
          <el-divider content-position="left" class="my-4!">
            <span class="text-xs text-slate-400">初始管理员账号（可选）</span>
          </el-divider>
          <div class="grid grid-cols-2 gap-4">
            <el-form-item label="用户名" prop="initUsername">
              <el-input v-model="form.initUsername" placeholder="留空则不创建" />
            </el-form-item>
            <el-form-item label="邮箱" prop="initEmail">
              <el-input v-model="form.initEmail" placeholder="管理员邮箱（可选）" />
            </el-form-item>
          </div>
          <p v-if="form.initUsername" class="tenants-form-note">创建后将生成初始管理员的一次性激活链接。</p>
        </template>
      </el-form>
      <template #footer>
        <div class="flex justify-end gap-2">
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" @click="handleSubmit" :loading="submitting">确定</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance } from 'element-plus'
import { Plus, RefreshRight, Search } from '@element-plus/icons-vue'
import { useRouter } from 'vue-router'
import { Building2 } from 'lucide-vue-next'
import { PortalPagePanel, useListPage } from '@/platform'
import AccountOverviewDrawer from '@/components/AccountOverviewDrawer.vue'
import RechargeDialog from '@/components/RechargeDialog.vue'
import type { RechargeFormPayload } from '@/components/recharge'
import {
  DsFilterBar,
  DsFilterField,
  DsPagination,
  DsTable,
  type DsTableColumn
} from '@/shared/ui'
import { platformAdminApi } from '@/api/platformAdmin'
import { showActivationCredential } from '@/platform/auth/activation'
import type { TenantListItem } from '@/api/types/admin'
import { formatDisplayUSD } from '@/shared/currency'
import type { OperationBody } from '@/api'

const router = useRouter()

const columns: DsTableColumn[] = [
  { key: 'tenantId', title: '租户 ID', width: 150, mono: true },
  { key: 'tenantName', title: '租户名称' },
  { key: 'contactPerson', title: '联系人' },
  { key: 'contactEmail', title: '联系邮箱' },
  { key: 'status', title: '状态', width: 100 },
  { key: 'balance', title: '余额', align: 'right' },
  { key: 'userCount', title: '用户数', align: 'center' },
  { key: 'createdTime', title: '入驻时间' },
  { key: 'actions', title: '操作', width: 260 }
]

const {
  rows,
  total,
  loading,
  page,
  pageSize,
  query,
  refresh,
  search,
  resetQuery,
  handlePageChange,
  handlePageSizeChange
} = useListPage<{ keyword: string; status: number | '' }, TenantListItem>({
  initialQuery: { keyword: '', status: '' },
  pageSize: 20,
  fetcher: async (params) => {
    try {
      const data = await platformAdminApi.listTenants({
        keyword: params.keyword || undefined,
        status: params.status === '' ? undefined : Number(params.status),
        page: params.page,
        size: params.pageSize
      })
      return { items: data.items || [], total: data.total }
    } catch (error) {
      ElMessage.error('获取列表失败')
      throw error
    }
  }
})

const dialogVisible = ref(false)
const isEdit = ref(false)
const submitting = ref(false)
const statusUpdatingIds = ref<Set<string>>(new Set())
const rechargeDialogVisible = ref(false)
const rechargeSubmitting = ref(false)
const rechargeTarget = ref<TenantListItem | null>(null)
const accountDrawerVisible = ref(false)
const accountTarget = ref<TenantListItem | null>(null)
const formRef = ref<FormInstance>()
const form = reactive({
  tenantId: '',
  tenantName: '',
  contactPerson: '',
  contactEmail: '',
  status: 1,
  initUsername: '',
  initEmail: ''
})

const rules = {
  tenantName: [{ required: true, message: '请输入租户名称', trigger: 'blur' }],
  contactEmail: [{ type: 'email', message: '请输入正确的邮箱地址', trigger: 'blur' }]
}

const formatTime = (ts?: number) => (ts ? new Date(ts).toLocaleString() : '')
const isStatusUpdating = (tenantId: string) => statusUpdatingIds.value.has(tenantId)

const setStatusUpdating = (tenantId: string, updating: boolean) => {
  const next = new Set(statusUpdatingIds.value)
  if (updating) next.add(tenantId)
  else next.delete(tenantId)
  statusUpdatingIds.value = next
}

const handleCreate = () => {
  isEdit.value = false
  Object.assign(form, { tenantId: '', tenantName: '', contactPerson: '', contactEmail: '', status: 1, initUsername: '', initEmail: '' })
  dialogVisible.value = true
}

const handleRecharge = (row: TenantListItem) => {
  rechargeTarget.value = row
  rechargeDialogVisible.value = true
}

const openAccount = (row: TenantListItem) => {
  accountTarget.value = row
  accountDrawerVisible.value = true
}

const submitTenantRecharge = async (payload: RechargeFormPayload) => {
  if (!rechargeTarget.value) return

  try {
    await ElMessageBox.confirm(
      `确认对租户「${rechargeTarget.value.tenantName}」执行${payload.expireTime ? '【限时】' : '【永久】'}入账操作？`,
      '财务安全确认',
      { confirmButtonText: '确定入账', cancelButtonText: '取消', roundButton: true, type: 'warning' }
    )
  } catch {
    return
  }

  rechargeSubmitting.value = true
  try {
    await platformAdminApi.createRecharge({
      packageType: 1,
      tenantId: rechargeTarget.value.tenantId,
      ...payload
    })
    ElMessage.success(`已成功为租户「${rechargeTarget.value.tenantName}」充值 $${(payload.amountMicroUsd / 1_000_000).toLocaleString()}`)
    rechargeDialogVisible.value = false
    void refresh()
  } catch (error: any) {
    ElMessage.error(error?.message || '充值失败')
  } finally {
    rechargeSubmitting.value = false
  }
}

const goTenantDetail = (tenantId: string) => {
  router.push(`/admin/organization/tenants/${tenantId}`)
}

const goAssociatedUsers = (tenantId: string) => {
  router.push({ path: `/admin/organization/tenants/${tenantId}`, query: { tab: 'users' } })
}

const goTenantPolicy = (tenantId: string) => {
  router.push(`/admin/organization/tenants/${tenantId}/policy`)
}

const handleEdit = (row: TenantListItem) => {
  isEdit.value = true
  Object.assign(form, {
    tenantId: row.tenantId,
    tenantName: row.tenantName,
    contactPerson: row.contactPerson || '',
    contactEmail: row.contactEmail || '',
    status: row.status,
    initUsername: '',
    initEmail: ''
  })
  dialogVisible.value = true
}

const handleStatusChange = async (row: TenantListItem, enabled: boolean) => {
  if (row.status === 3 || isStatusUpdating(row.tenantId) || enabled === (row.status === 1)) return

  const actionText = enabled ? '启用' : '停用'
  const tips = enabled
    ? '确定要启用该租户吗？启用后将恢复被级联停用的用户。'
    : '停用后将级联停用该租户下的所有组织用户和终端用户，确定继续吗？'

  setStatusUpdating(row.tenantId, true)
  try {
    await ElMessageBox.confirm(tips, '状态变更确认', {
      confirmButtonText: `立即${actionText}`,
      cancelButtonText: '取消',
      type: 'warning',
      roundButton: true
    })
    await platformAdminApi.updateTenantStatus(row.tenantId, enabled ? 'active' : 'disabled')
    row.status = enabled ? 1 : 2
    row.statusDisplay = enabled ? '正常' : '停用'
    ElMessage.success(`${actionText}成功`)
    void refresh()
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') ElMessage.error(`${actionText}失败`)
  } finally {
    setStatusUpdating(row.tenantId, false)
  }
}

const handleSubmit = async () => {
  if (!formRef.value) return
  await formRef.value.validate(async (valid: boolean) => {
    if (!valid) return
    submitting.value = true
    try {
      if (isEdit.value) {
        await platformAdminApi.updateTenant(form.tenantId, {
          tenantName: form.tenantName,
          contactPerson: form.contactPerson || undefined,
          contactEmail: form.contactEmail || undefined,
          status: form.status
        })
        ElMessage.success('更新成功')
      } else {
        const body: OperationBody<'admin-create-tenant'> = {
          tenantName: form.tenantName,
          status: form.status
        }
        if (form.contactPerson) body.contactPerson = form.contactPerson
        if (form.contactEmail) body.contactEmail = form.contactEmail
        if (form.initUsername) {
          body.initUsername = form.initUsername
          if (form.initEmail) body.initEmail = form.initEmail
        }
        const res = await platformAdminApi.createTenant(body)
        dialogVisible.value = false
        void refresh()
        if (res.initUsername && res.activationToken && res.activationExpiresIn) {
          await showActivationCredential(
            { activationToken: res.activationToken, activationExpiresIn: res.activationExpiresIn },
            `初始管理员「${res.initUsername}」`
          )
        } else ElMessage.success('创建成功')
        return
      }
      dialogVisible.value = false
      refresh()
    } catch (error: any) {
      ElMessage.error(error?.message || '操作失败')
    } finally {
      submitting.value = false
    }
  })
}

const handleDelete = async (id: string) => {
  try {
    await platformAdminApi.deleteTenant(id)
    ElMessage.success('删除成功')
    refresh()
  } catch {
    ElMessage.error('删除失败')
  }
}

</script>

<style scoped>
.tenants-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.tenants-form-note {
  margin: -8px 0 16px;
  color: var(--ds-muted);
  font-size: 12px;
}

.tenants-search-input {
  width: min(360px, 100%);
}

.tenants-status-select {
  width: 160px;
}

.tenants-search-input__icon {
  width: 16px;
  height: 16px;
  color: var(--ds-faint);
}

.tenants-button-icon {
  width: 16px;
  height: 16px;
  margin-right: 4px;
}

.tenants-name-link {
  padding: 0;
  border: none;
  background: transparent;
  font-size: inherit;
  font-weight: 700;
  color: var(--ds-accent);
  cursor: pointer;
}

.tenants-name-link:hover {
  color: var(--ds-accent-hover);
  text-decoration: underline;
}

.tenants-count-link {
  padding: 0;
  border: none;
  background: transparent;
  color: var(--ds-accent);
  cursor: pointer;
}

.tenants-count-link:hover {
  color: var(--ds-accent-hover);
  text-decoration: underline;
}

.tenants-num {
  font-variant-numeric: tabular-nums;
}

.tenants-credits {
  font-weight: 700;
  color: var(--ds-ink-soft);
}

.tenants-credits--danger {
  color: var(--ds-danger);
}

.tenants-time {
  font-size: 12px;
  color: var(--ds-faint);
}
</style>
