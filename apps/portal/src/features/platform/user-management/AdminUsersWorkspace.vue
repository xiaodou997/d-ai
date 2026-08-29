<!--
  Platform administrator account workspace.
  平台管理员 — 1:1 搬运自 v1/platform/platform-admin/src/views/System/AdminUserList.vue。
  适配：system axios → platformAdminApi（listSystemAdmins/createSystemAdmin/updateSystemAdmin/
       deleteSystemAdmin）；data.records → data.items；id 即 userId。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       筛选/表格/分页同卡）,空态带行动按钮;数据接入 useListPage,弹窗仍为 element-plus。
-->
<template>
  <div class="admin-users-page">
    <PortalPagePanel
      :icon="ShieldCheck"
      :breadcrumbs="[{ label: '用户中心' }, { label: '系统审计' }, { label: '平台管理员' }]"
      description="管理能够登录控制台的平台级管理员账号"
    >
      <template #actions>
        <el-button type="primary" @click="handleCreate">
          <el-icon class="mr-1"><Plus /></el-icon>
          添加管理员
        </el-button>
      </template>

      <template #filters>
        <DsFilterBar>
          <DsFilterField label="关键词">
            <el-input
              v-model="query.keyword"
              placeholder="搜索用户名或邮箱"
              clearable
              class="admin-users-search-input"
              @keyup.enter="search"
            >
              <template #prefix>
                <Search class="admin-users-search-input__icon" />
              </template>
            </el-input>
          </DsFilterField>

          <template #actions>
            <el-button type="primary" @click="search">
              <Search class="admin-users-button-icon" />
              筛选
            </el-button>
            <el-button @click="resetQuery">
              <RefreshRight class="admin-users-button-icon" />
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
      >
        <template #empty>
          <DsEmpty title="暂无管理员" description="还没有平台管理员账号,先添加一个吧">
            <template #action>
              <el-button type="primary" @click="handleCreate">
                <el-icon class="mr-1"><Plus /></el-icon>
                添加管理员
              </el-button>
            </template>
          </DsEmpty>
        </template>
        <template #cell-status="{ row }">
          <DsTag :tone="row.credentialState === 'pending_activation' ? 'neutral' : statusTone(row.status)">
            {{ row.statusText }}
          </DsTag>
        </template>
        <template #cell-createdTime="{ row }">
          <span class="admin-users-time">{{ formatTime(row.createdTime) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <el-button link type="primary" @click="handleEdit(row)">编辑</el-button>
          <el-button link type="warning" @click="handleResetPassword(row)">重置密码</el-button>
          <el-popconfirm title="确定删除该管理员吗？" @confirm="handleDelete(row.userId)">
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

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑管理员' : '创建管理员'" width="440px" append-to-body>
      <el-form :model="form" :rules="rules" ref="formRef" label-position="top">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" placeholder="登录用户名" :disabled="isEdit" />
        </el-form-item>
        <el-form-item label="邮箱" prop="email">
          <el-input v-model="form.email" placeholder="邮箱地址（可选）" />
        </el-form-item>
        <el-form-item v-if="isEdit" label="状态" prop="status">
          <el-select v-model="form.status" class="w-full">
            <el-option label="正常" :value="1" />
            <el-option label="禁用" :value="2" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance } from 'element-plus'
import { Plus, RefreshRight, Search } from '@element-plus/icons-vue'
import { ShieldCheck } from 'lucide-vue-next'
import { PortalPagePanel, useListPage } from '@/platform'
import {
  DsEmpty,
  DsFilterBar,
  DsFilterField,
  DsPagination,
  DsTable,
  DsTag,
  type DsTableColumn
} from '@/shared/ui'
import { platformAdminApi } from '@/api/platformAdmin'
import type { AdminUserItem } from '@/api/types/admin'
import { showActivationCredential } from '@/platform/auth/activation'

const columns: DsTableColumn[] = [
  { key: 'userId', title: '用户 ID', width: 160, mono: true },
  { key: 'username', title: '用户名' },
  { key: 'email', title: '邮箱' },
  { key: 'status', title: '状态', width: 100 },
  { key: 'createdTime', title: '创建时间', width: 170 },
  { key: 'actions', title: '操作', width: 200 }
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
} = useListPage<{ keyword: string }, AdminUserItem>({
  initialQuery: { keyword: '' },
  pageSize: 20,
  fetcher: async (params) => {
    try {
      const data = await platformAdminApi.listSystemAdmins({
        keyword: params.keyword || undefined,
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
const formRef = ref<FormInstance>()
const form = reactive({ userId: '', username: '', email: '', status: 1 })

const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }]
}

const statusTone = (status: number): 'positive' | 'neutral' => (status === 1 ? 'positive' : 'neutral')

const formatTime = (ts?: number) => (ts ? new Date(ts).toLocaleString() : '')

const handleCreate = () => {
  isEdit.value = false
  Object.assign(form, { userId: '', username: '', email: '', status: 1 })
  dialogVisible.value = true
}

const handleEdit = (row: AdminUserItem) => {
  isEdit.value = true
  Object.assign(form, { userId: row.userId, username: row.username, email: row.email, status: row.status })
  dialogVisible.value = true
}

const handleSubmit = async () => {
  if (!formRef.value) return
  await formRef.value.validate(async (valid: boolean) => {
    if (!valid) return
    submitting.value = true
    try {
      if (isEdit.value) {
        await platformAdminApi.updateSystemAdmin(form.userId, { email: form.email, status: form.status })
        ElMessage.success('更新成功')
      } else {
        const credential = await platformAdminApi.createSystemAdmin({
          username: form.username,
          email: form.email
        })
        dialogVisible.value = false
        void refresh()
        await showActivationCredential(credential, `管理员「${form.username}」`)
        return
      }
      dialogVisible.value = false
      refresh()
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : '操作失败'
      ElMessage.error(message)
    } finally {
      submitting.value = false
    }
  })
}

const handleResetPassword = async (row: AdminUserItem) => {
  try {
    await ElMessageBox.confirm(`重置后「${row.username}」的现有会话将全部失效，账号需通过新链接重新激活。`, '重置密码', {
      confirmButtonText: '确定重置',
      cancelButtonText: '取消',
      type: 'warning'
    })
    const credential = await platformAdminApi.resetSystemAdminPassword(row.userId)
    await showActivationCredential(credential, `管理员「${row.username}」`)
  } catch (err) {
    if (err !== 'cancel') ElMessage.error('重置失败')
  }
}

const handleDelete = async (id: string) => {
  try {
    await platformAdminApi.deleteSystemAdmin(id)
    ElMessage.success('已删除')
    refresh()
  } catch {
    ElMessage.error('删除失败')
  }
}

</script>

<style scoped>
.admin-users-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.admin-users-search-input {
  width: min(300px, 100%);
}

.admin-users-search-input__icon {
  width: 16px;
  height: 16px;
  color: var(--ds-faint);
}

.admin-users-button-icon {
  width: 16px;
  height: 16px;
  margin-right: 4px;
}

.admin-users-time {
  font-size: 12px;
  color: var(--ds-faint);
}
</style>
