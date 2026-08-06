<!--
  终端用户 — 1:1 搬运自 v1/platform/platform-admin/src/views/User/UserList.vue。
  保留搜索条件和账号启停，债务状态仅供查看。
  仅适配：axios api → v4 强类型 client（platformAdminApi，列表字段 items）；跳转租户详情走新路由。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       筛选/表格/分页同卡）;数据接入 useListPage,请求参数与筛选语义保持不变,抽屉仍为 element-plus。
-->
<template>
  <div class="endusers-page">
    <PortalPagePanel
      :icon="Users"
      :breadcrumbs="[{ label: '用户中心' }, { label: '业务管理' }, { label: '终端用户' }]"
      description="查看与管理各租户下的终端用户。"
    >
      <template #filters>
        <DsFilterBar>
          <DsFilterField label="租户名称">
            <el-input
              v-model="query.tenantName"
              placeholder="租户名称"
              clearable
              class="endusers-input"
              @keyup.enter="search"
            />
          </DsFilterField>
          <DsFilterField label="用户名">
            <el-input
              v-model="query.username"
              placeholder="用户名"
              clearable
              class="endusers-input"
              @keyup.enter="search"
            />
          </DsFilterField>
          <DsFilterField label="状态">
            <el-select v-model="query.status" placeholder="全部状态" clearable class="endusers-status-select">
              <el-option label="全部" value="" />
              <el-option label="正常" :value="1" />
              <el-option label="禁用" :value="2" />
              <el-option label="锁定" :value="3" />
            </el-select>
          </DsFilterField>

          <template #actions>
            <el-button type="primary" @click="search">
              <Search class="endusers-button-icon" />
              筛选
            </el-button>
            <el-button @click="resetQuery">
              <RefreshRight class="endusers-button-icon" />
              重置
            </el-button>
          </template>
        </DsFilterBar>
      </template>

      <DsTable
        :frame="false"
        :columns="columns"
        :rows="rows"
        row-key="userId"
        :loading="loading"
        empty-title="暂无用户"
      >
        <template #cell-tenantName="{ row }">
          <button type="button" class="endusers-tenant-link" @click="goTenantDetail(row.tenantId)">
            {{ row.tenantName || row.tenantId }}
          </button>
        </template>
        <template #cell-status="{ row }">
          <DsTag :tone="statusTone(row.status)">
            {{ row.statusDisplay }}
          </DsTag>
        </template>
        <template #cell-credits="{ row }">
          <span class="endusers-num endusers-credits">{{ row.credits?.toLocaleString() }} 积分</span>
        </template>
        <template #cell-lastLoginTime="{ row }">
          <span class="endusers-time">{{ formatTime(row.lastLoginTime) }}</span>
        </template>
        <template #cell-createdTime="{ row }">
          <span class="endusers-time">{{ formatTime(row.createdTime) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <el-button
            link
            :type="row.status === 1 ? 'warning' : 'success'"
            @click="handleToggleStatus(row)"
          >
            {{ row.status === 1 ? '禁用' : '启用' }}
          </el-button>
          <el-button link type="primary" @click="openDebt(row)">债务</el-button>
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

    <el-drawer
	  v-model="debtDrawerVisible"
	  :title="`未结债务 — ${debtTarget.username || debtTarget.userId}`"
      size="720px"
      :destroy-on-close="true"
    >
	  <DebtStatusPanel
		v-if="debtTarget.userId"
		owner-type="user"
		:account-id="debtTarget.userId"
      />
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { RefreshRight, Search } from '@element-plus/icons-vue'
import { Users } from 'lucide-vue-next'
import { PortalPagePanel, useListPage } from '@/platform'
import {
  DsFilterBar,
  DsFilterField,
  DsPagination,
  DsTable,
  DsTag,
  type DsTableColumn
} from '@/shared/ui'
import { platformAdminApi } from '@/api/platformAdmin'
import DebtStatusPanel from '@/components/DebtStatusPanel.vue'

const router = useRouter()

const columns: DsTableColumn[] = [
  { key: 'userId', title: '用户 ID', width: 150, mono: true },
  { key: 'username', title: '用户名' },
  { key: 'tenantName', title: '归属租户' },
  { key: 'email', title: '邮箱' },
  { key: 'status', title: '状态' },
  { key: 'credits', title: '账户积分', align: 'right' },
  { key: 'lastLoginTime', title: '最后登录' },
  { key: 'createdTime', title: '注册时间' },
  { key: 'actions', title: '操作', width: 160 }
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
} = useListPage<{ tenantName: string; username: string; status: number | '' }, any>({
  initialQuery: { tenantName: '', username: '', status: '' },
  pageSize: 20,
  fetcher: async (params) => {
    try {
      const data = await platformAdminApi.listEndUsers({
        tenantName: params.tenantName || undefined,
        username: params.username || undefined,
        status: params.status === '' ? undefined : Number(params.status),
        page: params.page,
        size: params.pageSize
      })
      return {
        items: (data.items || []).map((r: any) => ({
          ...r,
          statusDisplay: getStatusDisplay(r.status)
        })),
        total: data.total
      }
    } catch (error) {
      ElMessage.error('查询用户列表失败')
      throw error
    }
  }
})

const debtDrawerVisible = ref(false)
const debtTarget = ref<{ userId: string; username: string }>({ userId: '', username: '' })
const openDebt = (row: any) => {
  debtTarget.value = { userId: row.userId, username: row.username }
  debtDrawerVisible.value = true
}

const handleToggleStatus = async (row: any) => {
  const targetStatus = row.status === 1 ? 2 : 1
  const actionText = targetStatus === 1 ? '启用' : '禁用'

  try {
    await ElMessageBox.confirm(`确定要${actionText}该用户吗？`, '状态变更确认', {
      confirmButtonText: `立即${actionText}`,
      cancelButtonText: '取消',
      type: 'warning',
      roundButton: true
    })

    await platformAdminApi.updateEndUserStatus(row.userId, targetStatus === 2 ? 'disabled' : 'active')

    ElMessage.success(`${actionText}成功`)
    refresh()
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(`${actionText}失败`)
  }
}

const statusTone = (status: number): 'positive' | 'neutral' | 'danger' | 'warning' =>
  (({ 1: 'positive', 2: 'neutral', 3: 'danger', 4: 'warning' } as const)[status] ?? 'neutral')

const getStatusDisplay = (status: number) => {
  const map: Record<number, string> = { 1: '正常', 2: '禁用', 3: '锁定', 4: '级联停用' }
  return map[status] || '未知'
}

const formatTime = (ts?: number) => {
  if (!ts) return ''
  const date = new Date(ts)
  return date.toLocaleString()
}

const goTenantDetail = (tenantId: string) => {
  router.push(`/admin/organization/tenants/${tenantId}`)
}
</script>

<style scoped>
.endusers-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.endusers-input {
  width: min(200px, 100%);
}

.endusers-status-select {
  width: 160px;
}

.endusers-button-icon {
  width: 16px;
  height: 16px;
  margin-right: 4px;
}

.endusers-tenant-link {
  padding: 0;
  border: none;
  background: transparent;
  font-size: inherit;
  font-weight: 700;
  color: var(--ds-accent);
  cursor: pointer;
}

.endusers-tenant-link:hover {
  color: var(--ds-accent-hover);
  text-decoration: underline;
}

.endusers-num {
  font-variant-numeric: tabular-nums;
}

.endusers-credits {
  font-weight: 700;
  color: var(--ds-ink-soft);
}

.endusers-time {
  font-size: 12px;
  color: var(--ds-faint);
}
</style>
