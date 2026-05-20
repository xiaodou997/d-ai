<script setup>
import { onMounted, reactive, shallowRef, computed } from 'vue'
import { Refresh, Plus, Edit, Delete as DeleteIcon, CopyDocument } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  listTenantAPIKeys,
  createTenantAPIKey,
  updateTenantAPIKey,
  updateTenantAPIKeyStatus,
  rotateTenantAPIKey,
  deleteTenantAPIKey,
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
    label: `${item.model_code} · ${item.capability_type}`,
    value: item.model_code
  }))
)

const fetchAPIKeys = async () => {
  loading.value = true
  try {
    const res = await listTenantAPIKeys()
    apiKeys.value = res || []
  } finally {
    loading.value = false
  }
}

const fetchModels = async () => {
  try {
    const res = await listTenantModelGrants()
    models.value = res || []
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
      if (res?.key) {
        generatedKey.value = res.plaintext_key
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
  const newStatus = row.status === 'active' ? 'disabled' : 'active'
  try {
    await updateTenantAPIKeyStatus(row.id, newStatus)
    ElMessage.success(newStatus === 'active' ? '已启用' : '已停用')
    await fetchAPIKeys()
  } catch {}
}

const rotateKey = async (row) => {
  try {
    await ElMessageBox.confirm('轮换后旧 Key 立即失效，新 Key 仅显示一次，确定继续？', '轮换 API Key', {
      confirmButtonText: '确定轮换',
      cancelButtonText: '取消',
      type: 'warning'
    })
    const res = await rotateTenantAPIKey(row.id)
    generatedKey.value = res.plaintext_key
    showKeyDialog.value = true
    await fetchAPIKeys()
  } catch {}
}

const deleteKey = async (row) => {
  try {
    await ElMessageBox.confirm(`确定删除 API Key「${row.name}」？删除后无法恢复，且立即失效。`, '删除 API Key', {
      confirmButtonText: '确定删除',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger'
    })
    await deleteTenantAPIKey(row.id)
    ElMessage.success('已删除')
    await fetchAPIKeys()
  } catch {}
}

const copyKey = async (text) => {
  try {
    await navigator.clipboard.writeText(text ?? generatedKey.value)
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
        <p>用于租户自用或匿名用户场景</p>
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
          <el-table-column label="API Key" min-width="200">
            <template #default="{ row }">
              <code class="key-prefix">····{{ row.last_four || '????' }}</code>
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
          <el-table-column label="操作" width="240" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" :icon="Edit" @click="openEditDialog(row)">编辑</el-button>
              <el-button link type="info" @click="rotateKey(row)">轮换</el-button>
              <el-button
                link
                :type="row.status === 'active' ? 'warning' : 'success'"
                @click="toggleStatus(row)"
              >
                {{ row.status === 'active' ? '停用' : '启用' }}
              </el-button>
              <el-button link type="danger" :icon="DeleteIcon" @click="deleteKey(row)">删除</el-button>
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
      <div class="key-display">
        <code class="full-key">{{ generatedKey }}</code>
        <el-button :icon="CopyDocument" type="primary" @click="copyKey()">复制 Key</el-button>
      </div>
      <template #footer>
        <el-button type="primary" @click="showKeyDialog = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page-container {
  display: flex;
  flex-direction: column;
  gap: 24px;
  padding: 24px;
}

.page-header {
  background: #fff;
  border: 1px solid #f1f5f9;
  border-radius: 16px;
  padding: 24px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  box-shadow: 0 4px 16px rgba(15, 23, 42, 0.04);
}

.page-title { display: flex; flex-direction: column; }
.eyebrow {
  margin: 0 0 4px;
  color: #64748b;
  font-size: 11px;
  font-weight: 900;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.page-title h1 { margin: 0; color: #0f172a; font-size: 22px; font-weight: 900; }
.page-title p  { margin: 4px 0 0; color: #64748b; font-size: 13px; }

.header-actions { display: flex; gap: 12px; }

.page-main { /* no padding, container handles it */ }

.list-panel {
  background: #fff;
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  box-shadow: 0 4px 16px rgba(15, 23, 42, 0.04);
  overflow: hidden;
}

.key-cell {
  display: flex;
  align-items: center;
  gap: 4px;
}

.key-prefix {
  font-family: 'JetBrains Mono', 'Fira Code', 'Monaco', monospace;
  font-size: 12.5px;
  color: #475569;
  background: #f1f5f9;
  padding: 2px 8px;
  border-radius: 6px;
  word-break: break-all;
}

.key-display { display: flex; flex-direction: column; gap: 16px; }

.full-key {
  font-family: 'JetBrains Mono', 'Fira Code', 'Monaco', monospace;
  font-size: 13px;
  word-break: break-all;
  background: #1e293b;
  color: #e2e8f0;
  padding: 16px;
  border-radius: 12px;
  line-height: 1.6;
}

:deep(.el-table__header th) {
  background: #f8fafc !important;
  color: #64748b;
  font-size: 11px;
  font-weight: 900;
  text-transform: uppercase;
}
</style>
