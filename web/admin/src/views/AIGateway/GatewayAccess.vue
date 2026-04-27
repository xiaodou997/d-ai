<script setup>
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Key, Plus, Refresh, Search } from '@element-plus/icons-vue'
import {
  createTenantAPIKey,
  createUserAPIKey,
  grantModelToTenant,
  grantModelToUser,
  listModels,
  listTenantAPIKeys,
  listTenantModelGrants,
  listUserAPIKeys,
  listUserModelGrants,
  updateTenantAPIKeyStatus,
  updateTenantModelGrantStatus,
  updateUserAPIKeyStatus,
  updateUserModelGrantStatus
} from '@/api/aiGateway'

const loading = shallowRef(false)
const keyDialogVisible = shallowRef(false)
const lastKeyDialogVisible = shallowRef(false)
const activeOwner = shallowRef('tenant')
const lastPlainKey = shallowRef('')
const models = shallowRef([])
const tenantGrants = shallowRef([])
const tenantKeys = shallowRef([])
const userGrants = shallowRef([])
const userKeys = shallowRef([])

const scope = reactive({
  tenantId: 'tenant-local',
  userId: 'user-local'
})

const grantForm = reactive({
  model_id: '',
  status: 'active',
  created_by: 'admin'
})

const keyForm = reactive({
  name: '',
  quota_limit: 100000000,
  allowed_models: [],
  status: 'active',
  created_by: 'admin'
})

const modelOptions = computed(() =>
  models.value.map((item) => ({
    label: `${item.model_code} · ${item.display_name}`,
    value: item.id,
    code: item.model_code
  }))
)

const modelCodeOptions = computed(() =>
  models.value.map((item) => ({
    label: `${item.model_code} · ${item.display_name}`,
    value: item.model_code
  }))
)

const statusTagType = (status) => {
  const map = { active: 'success', inactive: 'warning', disabled: 'danger' }
  return map[status] || 'info'
}

const fetchModels = async () => {
  models.value = await listModels()
}

const fetchAll = async () => {
  if (!scope.tenantId) {
    ElMessage.warning('请输入租户 ID')
    return
  }
  loading.value = true
  try {
    tenantGrants.value = await listTenantModelGrants(scope.tenantId)
    tenantKeys.value = await listTenantAPIKeys(scope.tenantId)
    if (scope.userId) {
      userGrants.value = await listUserModelGrants(scope.tenantId, scope.userId)
      userKeys.value = await listUserAPIKeys(scope.tenantId, scope.userId)
    } else {
      userGrants.value = []
      userKeys.value = []
    }
  } finally {
    loading.value = false
  }
}

const submitGrant = async (owner) => {
  if (!grantForm.model_id) {
    ElMessage.warning('请选择模型')
    return
  }
  if (owner === 'tenant') {
    await grantModelToTenant(scope.tenantId, grantForm)
  } else {
    if (!scope.userId) {
      ElMessage.warning('请输入用户 ID')
      return
    }
    await grantModelToUser(scope.tenantId, scope.userId, grantForm)
  }
  ElMessage.success('模型授权已保存')
  await fetchAll()
}

const openKeyDialog = (owner) => {
  activeOwner.value = owner
  Object.assign(keyForm, {
    name: owner === 'tenant' ? 'Tenant runtime key' : 'User runtime key',
    quota_limit: 100000000,
    allowed_models: [],
    status: 'active',
    created_by: 'admin'
  })
  keyDialogVisible.value = true
}

const submitKey = async () => {
  const payload = {
    ...keyForm,
    quota_limit: keyForm.quota_limit || undefined
  }
  const response =
    activeOwner.value === 'tenant'
      ? await createTenantAPIKey(scope.tenantId, payload)
      : await createUserAPIKey(scope.tenantId, scope.userId, payload)

  lastPlainKey.value = response.api_key
  keyDialogVisible.value = false
  lastKeyDialogVisible.value = true
  await fetchAll()
}

const toggleGrant = async (owner, row) => {
  const nextStatus = row.status === 'active' ? 'disabled' : 'active'
  if (owner === 'tenant') {
    await updateTenantModelGrantStatus(scope.tenantId, row.model_id, nextStatus)
  } else {
    await updateUserModelGrantStatus(scope.tenantId, scope.userId, row.model_id, nextStatus)
  }
  ElMessage.success('授权状态已更新')
  await fetchAll()
}

const toggleKey = async (owner, row) => {
  const nextStatus = row.status === 'active' ? 'disabled' : 'active'
  if (owner === 'tenant') {
    await updateTenantAPIKeyStatus(scope.tenantId, row.id, nextStatus)
  } else {
    await updateUserAPIKeyStatus(scope.tenantId, scope.userId, row.id, nextStatus)
  }
  ElMessage.success('API Key 状态已更新')
  await fetchAll()
}

const copyPlainKey = async () => {
  await navigator.clipboard.writeText(lastPlainKey.value)
  ElMessage.success('已复制')
}

const closePlainKey = async () => {
  try {
    await ElMessageBox.confirm('关闭后将无法再次查看明文 API Key，确认关闭？', '确认', {
      confirmButtonText: '关闭',
      cancelButtonText: '返回',
      type: 'warning'
    })
    lastPlainKey.value = ''
    lastKeyDialogVisible.value = false
  } catch {}
}

onMounted(async () => {
  await fetchModels()
  await fetchAll()
})
</script>

<template>
  <div class="space-y-4">
    <section class="panel">
      <div class="grid grid-cols-1 md:grid-cols-[1fr_1fr_auto] gap-4 items-end">
        <el-form-item label="租户 ID" class="!mb-0">
          <el-input v-model="scope.tenantId" placeholder="tenant-local" />
        </el-form-item>
        <el-form-item label="用户 ID" class="!mb-0">
          <el-input v-model="scope.userId" placeholder="user-local" />
        </el-form-item>
        <el-button type="primary" :icon="Search" :loading="loading" @click="fetchAll">查询</el-button>
      </div>
    </section>

    <section class="panel">
      <div class="section-head">
        <div>
          <h3>模型授权</h3>
          <p>租户授权控制可售模型，用户授权控制可创建 Key 的模型范围</p>
        </div>
        <el-button :icon="Refresh" circle @click="fetchAll" />
      </div>
      <div class="grid grid-cols-1 md:grid-cols-[minmax(0,1fr)_160px_160px] gap-3 items-end mb-4">
        <el-form-item label="模型" class="!mb-0">
          <el-select v-model="grantForm.model_id" class="w-full" filterable>
            <el-option v-for="item in modelOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-button type="primary" :icon="Plus" @click="submitGrant('tenant')">授权给租户</el-button>
        <el-button type="success" :icon="Plus" @click="submitGrant('user')">授权给用户</el-button>
      </div>

      <div class="grid grid-cols-1 xl:grid-cols-2 gap-4">
        <div>
          <h4 class="table-title">租户模型</h4>
          <el-table :data="tenantGrants" border stripe class="w-full">
            <el-table-column prop="model_code" label="模型" min-width="150" />
            <el-table-column prop="capability_type" label="能力" width="90" />
            <el-table-column label="状态" width="95">
              <template #default="{ row }">
                <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="90">
              <template #default="{ row }">
                <el-button link @click="toggleGrant('tenant', row)">
                  {{ row.status === 'active' ? '禁用' : '启用' }}
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
        <div>
          <h4 class="table-title">用户模型</h4>
          <el-table :data="userGrants" border stripe class="w-full">
            <el-table-column prop="model_code" label="模型" min-width="150" />
            <el-table-column prop="capability_type" label="能力" width="90" />
            <el-table-column label="状态" width="95">
              <template #default="{ row }">
                <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="90">
              <template #default="{ row }">
                <el-button link @click="toggleGrant('user', row)">
                  {{ row.status === 'active' ? '禁用' : '启用' }}
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </div>
    </section>

    <section class="panel">
      <div class="section-head">
        <div>
          <h3>API Key</h3>
          <p>租户 Key 适合匿名售卖，用户 Key 绑定终端用户账本</p>
        </div>
        <div class="flex gap-2">
          <el-button type="primary" :icon="Key" @click="openKeyDialog('tenant')">租户 Key</el-button>
          <el-button type="success" :icon="Key" @click="openKeyDialog('user')">用户 Key</el-button>
        </div>
      </div>

      <div class="grid grid-cols-1 xl:grid-cols-2 gap-4">
        <div>
          <h4 class="table-title">租户 Key</h4>
          <el-table :data="tenantKeys" border stripe class="w-full">
            <el-table-column prop="key_prefix" label="前缀" width="130" />
            <el-table-column prop="name" label="名称" min-width="160" />
            <el-table-column label="模型" min-width="180" show-overflow-tooltip>
              <template #default="{ row }">{{ row.allowed_models?.join(', ') || '全部' }}</template>
            </el-table-column>
            <el-table-column label="额度" width="130" align="right">
              <template #default="{ row }">{{ row.quota_used }} / {{ row.quota_limit || '不限' }}</template>
            </el-table-column>
            <el-table-column label="状态" width="95">
              <template #default="{ row }">
                <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="90">
              <template #default="{ row }">
                <el-button link @click="toggleKey('tenant', row)">
                  {{ row.status === 'active' ? '禁用' : '启用' }}
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
        <div>
          <h4 class="table-title">用户 Key</h4>
          <el-table :data="userKeys" border stripe class="w-full">
            <el-table-column prop="key_prefix" label="前缀" width="130" />
            <el-table-column prop="name" label="名称" min-width="160" />
            <el-table-column label="模型" min-width="180" show-overflow-tooltip>
              <template #default="{ row }">{{ row.allowed_models?.join(', ') || '全部' }}</template>
            </el-table-column>
            <el-table-column label="额度" width="130" align="right">
              <template #default="{ row }">{{ row.quota_used }} / {{ row.quota_limit || '不限' }}</template>
            </el-table-column>
            <el-table-column label="状态" width="95">
              <template #default="{ row }">
                <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="90">
              <template #default="{ row }">
                <el-button link @click="toggleKey('user', row)">
                  {{ row.status === 'active' ? '禁用' : '启用' }}
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </div>
    </section>

    <el-dialog v-model="keyDialogVisible" :title="activeOwner === 'tenant' ? '创建租户 Key' : '创建用户 Key'" width="640px">
      <el-form :model="keyForm" label-position="top">
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="名称" required>
            <el-input v-model="keyForm.name" />
          </el-form-item>
          <el-form-item label="总额度">
            <el-input-number v-model="keyForm.quota_limit" :min="0" class="w-full" />
          </el-form-item>
        </div>
        <el-form-item label="允许模型">
          <el-select v-model="keyForm.allowed_models" class="w-full" multiple filterable>
            <el-option v-for="item in modelCodeOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="keyDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitKey">创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="lastKeyDialogVisible" title="API Key 明文" width="680px" :close-on-click-modal="false" :show-close="false">
      <el-alert type="warning" :closable="false" show-icon title="明文只显示一次，关闭后无法再次查看。" />
      <el-input :model-value="lastPlainKey" class="mt-4" readonly>
        <template #append>
          <el-button @click="copyPlainKey">复制</el-button>
        </template>
      </el-input>
      <template #footer>
        <el-button type="primary" @click="closePlainKey">我已保存，关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.panel {
  border: 1px solid #f1f5f9;
  border-radius: 14px;
  padding: 16px;
  min-width: 0;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.section-head h3 {
  margin: 0;
  color: #0f172a;
  font-size: 16px;
  font-weight: 800;
}

.section-head p,
.table-title {
  margin: 4px 0 12px;
  color: #64748b;
  font-size: 12px;
}

.table-title {
  font-weight: 800;
}
</style>
