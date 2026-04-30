<script setup>
import { onMounted, reactive, shallowRef, computed } from 'vue'
import { Refresh, Plus, Edit, Delete, CopyDocument } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  listTenantAPIKeys,
  createTenantAPIKey,
  updateTenantAPIKey,
  updateTenantAPIKeyStatus,
  listTenantModelGrants,
  statusOptions,
  formatCredits
} from '@/api/aiGateway'

const loading = shallowRef(false)
const apiKeys = shallowRef([])
const models = shallowRef([]) // 已授权模型（用于选择）
const dialogVisible = shallowRef(false)
const editingKeyId = shallowRef('')
const saving = shallowRef(false)
const generatedKey = shallowRef('')
const showKeyDialog = shallowRef(false)

const keyForm = reactive({
  name: '',
  quota_limit: null,
  allowed_models: [],
  status: 'active'
})

const isEditing = computed(() => Boolean(editingKeyId.value))
const modelOptions = computed(() =>
  models.value.map((item) => ({
    label: `${item.model_code} · ${item.display_name}`,
    value: item.model_id
  }))
)

const fetchAPIKeys = async () => {
  loading.value = true
  try {
    const res = await listTenantAPIKeys()
    apiKeys.value = res.data || []
  } finally {
    loading.value = false
  }
}

const fetchModels = async () => {
  try {
    const res = await listTenantModelGrants()
    models.value = res.data || []
  } catch {}
}

const resetForm = () => {
  editingKeyId.value = ''
  Object.assign(keyForm, {
    name: '',
    quota_limit: null,
    allowed_models: [],
    status: 'active'
  })
}

const openCreateDialog = () => {
  resetForm()
  dialogVisible.value = true
}

const openEditDialog = (row) => {
  editingKeyId.value = row.id
  Object.assign(keyForm, {
    name: row.name,
    quota_limit: row.quota_limit,
    allowed_models: row.allowed_models || [],
    status: row.status
  })
  dialogVisible.value = true
}

const submitForm = async () => {
  if (!keyForm.name) {
    ElMessage.warning('请输入名称')
    return
  }
  saving.value = true
  try {
    if (isEditing.value) {
      await updateTenantAPIKey(editingKeyId.value, {
        name: keyForm.name,
        quota_limit: keyForm.quota_limit,
        allowed_models: keyForm.allowed_models,
        status: keyForm.status
      })
      ElMessage.success('已更新')
      dialogVisible.value = false
    } else {
      const res = await createTenantAPIKey({
        name: keyForm.name,
        quota_limit: keyForm.quota_limit,
        allowed_models: keyForm.allowed_models,
        status: keyForm.status
      })
      // 显示生成的完整 Key
      if (res.data && res.data.key && res.data.key.key) {
        generatedKey.value = res.data.key.key
        showKeyDialog.value = true
        dialogVisible.value = false
      }
      ElMessage.success('已创建')
    }
    await fetchAPIKeys()
  } finally {
    saving.value = false
  }
}

const toggleStatus = async (row) => {
  const newStatus = row.status === 'active' ? 'inactive' : 'active'
  try {
    await updateTenantAPIKeyStatus(row.id, newStatus)
    ElMessage.success(`已${newStatus === 'active' ? '启用' : '停用'}`)
    await fetchAPIKeys()
  } catch {}
}

const deleteKey = async (row) => {
  try {
    await ElMessageBox.confirm('确定删除此 API Key？删除后无法恢复', '提示', {
      confirmButtonText: '确定删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
    // TODO: 后端暂无删除 API Key 的接口，暂时只支持停用
    ElMessage.warning('暂不支持删除，请使用停用功能')
  } catch {}
}

const copyKey = async () => {
  try {
    await navigator.clipboard.writeText(generatedKey.value)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.warning('复制失败，请手动复制')
  }
}

const statusTagType = (status) => {
  const map = { active: 'success', inactive: 'warning', disabled: 'danger' }
  return map[status] || 'info'
}

const formatDate = (value) => {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

const parseAllowedModels = (raw) => {
  if (!raw) return []
  try {
    return typeof raw === 'string' ? JSON.parse(raw) : raw
  } catch {
    return []
  }
}

const getModelsDisplay = (row) => {
  const ids = parseAllowedModels(row.allowed_models)
  if (ids.length === 0) return '全部授权模型'
  return ids.length + ' 个模型'
}

onMounted(() => {
  fetchAPIKeys()
  fetchModels()
})
</script>

<template>
  <div class="page-container">
    <!-- Header -->
    <header class="page-header">
      <div class="page-title">
        <p class="eyebrow">Tenant API Keys</p>
        <h1>租户 API Key</h1>
        <p>用于租户自用或匿名用户场景，Key 创建后仅显示一次请立即保存</p>
      </div>
      <div class="header-actions">
        <el-button :icon="Refresh" @click="fetchAPIKeys" :loading="loading">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openCreateDialog">创建 Key</el-button>
      </div>
    </header>

    <!-- Content -->
    <main class="page-main">
      <section class="list-panel">
        <el-table :data="apiKeys" v-loading="loading" stripe>
          <el-table-column prop="name" label="名称" min-width="140" />
          <el-table-column prop="key_prefix" label="Key 前缀" min-width="120">
            <template #default="{ row }">
              <code class="key-prefix">{{ row.key_prefix }}...</code>
            </template>
          </el-table-column>
          <el-table-column label="配额限制" min-width="120">
            <template #default="{ row }">
              {{ row.quota_limit ? formatCredits(row.quota_limit) + ' 积分' : '无限制' }}
            </template>
          </el-table-column>
          <el-table-column label="已使用" min-width="120">
            <template #default="{ row }">
              {{ formatCredits(row.quota_used) }} 积分
            </template>
          </el-table-column>
          <el-table-column label="允许模型" min-width="100">
            <template #default="{ row }">
              <span class="text-sm">{{ getModelsDisplay(row) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" min-width="80">
            <template #default="{ row }">
              <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="创建时间" min-width="160">
            <template #default="{ row }">
              {{ formatDate(row.created_at) }}
            </template>
          </el-table-column>
          <el-table-column label="操作" width="140" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" :icon="Edit" @click="openEditDialog(row)">编辑</el-button>
              <el-button
                link
                :type="row.status === 'active' ? 'warning' : 'success'"
                @click="toggleStatus(row)"
              >
                {{ row.status === 'active' ? '停用' : '启用' }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </section>
    </main>

    <!-- Create/Edit Dialog -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEditing ? '编辑 API Key' : '创建 API Key'"
      width="560px"
      append-to-body
    >
      <el-form :model="keyForm" label-position="top">
        <el-form-item label="名称" required>
          <el-input v-model="keyForm.name" placeholder="给 API Key 命名" />
        </el-form-item>
        <el-form-item label="配额限制">
          <el-input-number
            v-model="keyForm.quota_limit"
            :min="0"
            :precision="0"
            placeholder="不填则无限制"
            class="w-full"
          />
          <p class="text-xs text-slate-400 mt-1">积分上限，超过后将无法使用</p>
        </el-form-item>
        <el-form-item label="允许模型">
          <el-select v-model="keyForm.allowed_models" multiple filterable placeholder="选择允许的模型" class="w-full">
            <el-option v-for="item in modelOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
          <p class="text-xs text-slate-400 mt-1">不选则继承租户所有授权模型</p>
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="keyForm.status" class="w-full">
            <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitForm" :loading="saving">{{ isEditing ? '保存' : '创建' }}</el-button>
      </template>
    </el-dialog>

    <!-- Show Generated Key Dialog -->
    <el-dialog v-model="showKeyDialog" title="API Key 已生成" width="480px" append-to-body>
      <el-alert type="warning" :closable="false" show-icon class="mb-4">
        <template #title>
          <strong>请立即复制保存！</strong>
        </template>
        Key 仅显示一次，关闭后将无法再次查看完整 Key。
      </el-alert>
      <div class="key-display">
        <code class="full-key">{{ generatedKey }}</code>
        <el-button :icon="CopyDocument" type="primary" @click="copyKey">复制 Key</el-button>
      </div>
      <template #footer>
        <el-button type="primary" @click="showKeyDialog = false">我已保存，关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: #f5f7fa;
}

.page-header {
  padding: 16px 24px;
  background: #ffffff;
  border-bottom: 1px solid #e4e7ed;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.page-title {
  display: flex;
  flex-direction: column;
}

.eyebrow {
  margin: 0 0 4px;
  color: #64748b;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0;
  text-transform: uppercase;
}

.page-title h1 {
  margin: 0;
  color: #111827;
  font-size: 22px;
  font-weight: 900;
}

.page-title p {
  margin: 4px 0 0;
  color: #64748b;
  font-size: 13px;
}

.header-actions {
  display: flex;
  gap: 12px;
}

.page-main {
  padding: 24px;
  flex: 1;
  min-height: 0;
}

.list-panel {
  background: #ffffff;
  border-radius: 8px;
  padding: 16px;
}

.key-prefix {
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 13px;
  color: #606266;
  background: #f5f7fa;
  padding: 2px 6px;
  border-radius: 4px;
}

.key-display {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.full-key {
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 14px;
  word-break: break-all;
  background: #f5f7fa;
  padding: 12px;
  border-radius: 8px;
  border: 1px solid #e4e7ed;
}
</style>