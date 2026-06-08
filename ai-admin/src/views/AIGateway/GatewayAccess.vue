<script setup>
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Edit, Plus, Refresh, Search } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { queryTenants } from '@/api/tenant'
import {
  capabilityOptions,
  grantModelToTenant,
  listModels,
  listTenantModelGrants,
  updateTenantModelGrantStatus
} from '@/api/aiGateway'

const authStore = useAuthStore()
const loading = shallowRef(false)
const tenantLoading = shallowRef(false)
const grantDialogVisible = shallowRef(false)
const editingGrantId = shallowRef('')
const models = shallowRef([])
const tenantGrants = shallowRef([])

const tenantSearchResults = shallowRef([])

const selectedTenant = reactive({
  tenantId: '',
  tenantName: '',
  contactPerson: '',
  contactEmail: ''
})

const grantForm = reactive({
  model_ids: [],
  model_id: '',
  status: 'active',
  created_by: 'admin'
})

const isEditingGrant = computed(() => Boolean(editingGrantId.value))
const hasSelectedTenant = computed(() => Boolean(selectedTenant.tenantId))

const modelOptions = computed(() =>
  models.value.map((item) => ({
    label: item.model_code,
    value: item.id,
    code: item.model_code,
    capability: item.capability_type
  }))
)

const grantSummary = computed(() => ({
  totalGrants: tenantGrants.value.length,
  activeGrants: tenantGrants.value.filter((item) => item.status === 'active').length,
  chatModels: tenantGrants.value.filter((item) => item.capability_type === 'chat').length,
  imageModels: tenantGrants.value.filter((item) => item.capability_type === 'image').length
}))

const statusTagType = (status) => {
  const map = { active: 'success', inactive: 'warning', disabled: 'danger' }
  return map[status] || 'info'
}

const capabilityLabel = (value) =>
  capabilityOptions.find((o) => o.value === value)?.label || value || '-'

const resetGrantForm = () => {
  editingGrantId.value = ''
  Object.assign(grantForm, {
    model_ids: [],
    model_id: '',
    status: 'active',
    created_by: 'admin'
  })
}

const applyGrantForm = (row) => {
  editingGrantId.value = row.model_id
  Object.assign(grantForm, {
    model_ids: [],
    model_id: row.model_id,
    status: row.status,
    created_by: 'admin'
  })
}

const fetchModels = async () => {
  if (!authStore.isPlatformAdmin) {
    models.value = []
    return
  }
  models.value = await listModels()
}

const searchTenants = async (query) => {
  const keyword = (query || '').trim()
  if (!keyword) {
    tenantSearchResults.value = []
    return
  }
  tenantLoading.value = true
  try {
    const data = await queryTenants({ keyword, page: 1, size: 20 })
    tenantSearchResults.value = (data.records || []).map((item) => ({
      tenantId: item.tenantId,
      tenantName: item.tenantName,
      contactPerson: item.contactPerson,
      contactEmail: item.contactEmail,
      label: `${item.tenantName} · ${item.tenantId} · ${item.contactPerson || '未知联系人'}`
    }))
  } catch {
    tenantSearchResults.value = []
  } finally {
    tenantLoading.value = false
  }
}

const selectTenant = (tenantId) => {
  if (!tenantId) {
    selectedTenant.tenantId = ''
    selectedTenant.tenantName = ''
    selectedTenant.contactPerson = ''
    selectedTenant.contactEmail = ''
    tenantGrants.value = []
    return
  }
  const found = tenantSearchResults.value.find((item) => item.tenantId === tenantId)
  if (found) {
    Object.assign(selectedTenant, {
      tenantId: found.tenantId,
      tenantName: found.tenantName,
      contactPerson: found.contactPerson,
      contactEmail: found.contactEmail
    })
  }
  fetchGrants()
}

const fetchGrants = async () => {
  if (!selectedTenant.tenantId) {
    tenantGrants.value = []
    return
  }
  loading.value = true
  try {
    tenantGrants.value = await listTenantModelGrants(selectedTenant.tenantId)
  } finally {
    loading.value = false
  }
}

const openGrantDialog = () => {
  if (!selectedTenant.tenantId) {
    ElMessage.warning('请先选择租户')
    return
  }
  resetGrantForm()
  grantDialogVisible.value = true
}

const openGrantEditDialog = (row) => {
  applyGrantForm(row)
  grantDialogVisible.value = true
}

const submitGrant = async () => {
  if (isEditingGrant.value) {
    if (!grantForm.model_id) {
      ElMessage.warning('请选择模型')
      return
    }
    await grantModelToTenant(selectedTenant.tenantId, {
      model_id: grantForm.model_id,
      status: grantForm.status,
      created_by: grantForm.created_by
    })
    ElMessage.success('模型授权已更新')
  } else {
    if (!grantForm.model_ids.length) {
      ElMessage.warning('请选择模型')
      return
    }
    const results = []
    for (const modelId of grantForm.model_ids) {
      try {
        await grantModelToTenant(selectedTenant.tenantId, {
          model_id: modelId,
          status: grantForm.status,
          created_by: grantForm.created_by
        })
        results.push({ success: true, modelId })
      } catch (e) {
        results.push({ success: false, modelId })
      }
    }
    const successCount = results.filter((r) => r.success).length
    const failCount = results.filter((r) => !r.success).length
    if (failCount === 0) {
      ElMessage.success(`已授权 ${successCount} 个模型`)
    } else {
      ElMessage.warning(`授权完成：${successCount} 成功，${failCount} 失败`)
    }
  }
  grantDialogVisible.value = false
  await fetchGrants()
}

const toggleGrant = async (row) => {
  const nextStatus = row.status === 'active' ? 'disabled' : 'active'
  await updateTenantModelGrantStatus(selectedTenant.tenantId, row.model_id, nextStatus)
  ElMessage.success('授权状态已更新')
  await fetchGrants()
}

onMounted(async () => {
  await fetchModels()
})
</script>

<template>
  <div class="access-page">
    <!-- 搜索栏 -->
    <section class="search-panel">
      <div class="search-row">
        <el-select
          v-model="selectedTenant.tenantId"
          filterable
          remote
          :remote-method="searchTenants"
          :loading="tenantLoading"
          placeholder="输入租户名称、联系人或邮箱搜索"
          class="tenant-select"
          clearable
          @change="selectTenant"
        >
          <el-option
            v-for="item in tenantSearchResults"
            :key="item.tenantId"
            :label="item.label"
            :value="item.tenantId"
          />
        </el-select>
        <el-button type="primary" :icon="Search" :loading="loading" :disabled="!hasSelectedTenant" @click="fetchGrants">查询授权</el-button>
        <el-button :icon="Refresh" circle :disabled="!hasSelectedTenant" @click="fetchGrants" />
      </div>
      <p v-if="hasSelectedTenant" class="tenant-info">
        {{ selectedTenant.tenantName }} · {{ selectedTenant.contactPerson || '无联系人' }} · {{ selectedTenant.contactEmail || '无邮箱' }}
      </p>
      <p v-else class="tenant-hint">请先搜索并选择一个租户，再进行授权操作</p>
    </section>

    <!-- 统计卡片 -->
    <section class="metric-grid">
      <div class="metric-cell">
        <span>已授权模型</span>
        <strong>{{ grantSummary.totalGrants }}</strong>
        <small>{{ grantSummary.activeGrants }} active</small>
      </div>
      <div class="metric-cell">
        <span>对话模型</span>
        <strong>{{ grantSummary.chatModels }}</strong>
        <small>Chat capability</small>
      </div>
      <div class="metric-cell">
        <span>图像模型</span>
        <strong>{{ grantSummary.imageModels }}</strong>
        <small>Image capability</small>
      </div>
      <div class="metric-cell">
        <span>计费单位</span>
        <strong>积分</strong>
        <small>按租户价格结算</small>
      </div>
    </section>

    <!-- 配置流程 -->
    <section class="guide-panel">
      <div>
        <p class="eyebrow">Access Flow</p>
        <h3>配置流程</h3>
      </div>
      <ol>
        <li>在模型页面创建公开模型和配置路由。</li>
        <li>在此页面将模型授权给租户，使其可调用。</li>
        <li>租户在自有后台设置销售价格和创建 API Key。</li>
        <li>终端用户在用户后台创建自己的 API Key。</li>
      </ol>
      <p class="guide-note">平台管理员只负责模型授权，API Key 由租户和用户自行管理。</p>
    </section>

    <!-- 授权列表 -->
    <section class="panel">
      <div class="section-head">
        <div>
          <h3>已授权模型</h3>
          <p>{{ selectedTenant.tenantId ? `${selectedTenant.tenantName} (${selectedTenant.tenantId}) - 可调用的模型列表` : '请先选择租户' }}</p>
        </div>
        <el-button type="primary" :icon="Plus" :disabled="!hasSelectedTenant" @click="openGrantDialog">新增授权</el-button>
      </div>

      <el-table v-loading="loading" :data="tenantGrants" border stripe class="w-full">
        <el-table-column label="模型" min-width="200">
          <template #default="{ row }">
            <div class="model-cell">
              <strong>{{ row.model_code }}</strong>
              <small>{{ capabilityLabel(row.capability_type) }}</small>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="能力类型" width="120">
          <template #default="{ row }">
            <el-tag size="small">{{ capabilityLabel(row.capability_type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" :icon="Edit" @click="openGrantEditDialog(row)">编辑</el-button>
            <el-button link :type="row.status === 'active' ? 'warning' : 'success'" @click="toggleGrant(row)">
              {{ row.status === 'active' ? '禁用' : '启用' }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <!-- 授权对话框 -->
    <el-dialog v-model="grantDialogVisible" :title="isEditingGrant ? '编辑模型授权' : '新增模型授权'" width="480px" append-to-body>
      <el-form :model="grantForm" label-position="top">
        <el-form-item :label="isEditingGrant ? '模型' : '选择模型（可多选）'" required>
          <!-- 新增：多选 -->
          <el-select
            v-if="!isEditingGrant"
            v-model="grantForm.model_ids"
            multiple
            class="w-full"
            filterable
            placeholder="可选择多个模型批量授权"
          >
            <el-option v-for="item in modelOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
          <!-- 编辑：单选 -->
          <el-select
            v-else
            v-model="grantForm.model_id"
            class="w-full"
            filterable
            placeholder="选择要授权的模型"
          >
            <el-option v-for="item in modelOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="grantForm.status" class="w-full">
            <el-option label="启用" value="active" />
            <el-option label="停用" value="inactive" />
            <el-option label="禁用" value="disabled" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="grantDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitGrant">{{ isEditingGrant ? '保存' : '授权' }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.access-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.search-panel,
.panel,
.guide-panel {
  min-width: 0;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #ffffff;
}

.search-panel {
  padding: 16px;
}

.search-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.tenant-select {
  width: 360px;
}

.tenant-info {
  margin: 12px 0 0;
  color: #475569;
  font-size: 12px;
  font-weight: 700;
}

.tenant-hint {
  margin: 12px 0 0;
  color: #94a3b8;
  font-size: 12px;
  font-weight: 600;
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}

.metric-cell {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 4px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #f8fafc;
  padding: 12px;
}

.metric-cell span,
.metric-cell small {
  color: #64748b;
  font-size: 11px;
  font-weight: 800;
}

.metric-cell strong {
  overflow: hidden;
  color: #0f172a;
  font-size: 20px;
  font-weight: 900;
  line-height: 1.15;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.guide-panel {
  display: grid;
  grid-template-columns: 190px minmax(0, 1fr);
  gap: 16px;
  padding: 16px;
  border-color: #bae6fd;
  background: #f0f9ff;
}

.eyebrow {
  margin: 0 0 4px;
  color: #64748b;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0;
  text-transform: uppercase;
}

.guide-panel h3,
.section-head h3 {
  margin: 0;
  color: #0f172a;
  font-size: 16px;
  font-weight: 900;
}

.guide-panel ol {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 18px;
  margin: 0;
  padding-left: 18px;
}

.guide-panel li,
.guide-note,
.section-head p {
  color: #475569;
  font-size: 12px;
  font-weight: 700;
  line-height: 1.65;
}

.guide-note {
  grid-column: 2;
  margin: 2px 0 0;
  color: #0369a1;
}

.panel {
  padding: 16px;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.section-head p {
  margin: 4px 0 0;
  color: #64748b;
}

.model-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.model-cell strong {
  color: #334155;
  font-weight: 700;
}

.model-cell small {
  color: #94a3b8;
  font-size: 11px;
}

.price-tag-public {
  color: #94a3b8;
  font-size: 11px;
  font-weight: 700;
}

.public-price-hint {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-radius: 6px;
  background: #f0f9ff;
  border: 1px solid #bae6fd;
  margin-bottom: 16px;
  font-size: 12px;
  font-weight: 700;
  color: #0369a1;
}

.hint-label {
  color: #64748b;
  white-space: nowrap;
}

.size-price-editor {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}

.size-price-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.size-input { width: 140px; }
.price-input { flex: 1; }

@media (max-width: 900px) {
  .metric-grid,
  .guide-panel,
  .guide-panel ol {
    grid-template-columns: 1fr;
  }

  .guide-note {
    grid-column: auto;
  }

  .search-row {
    flex-wrap: wrap;
  }

  .tenant-select {
    width: 100%;
  }
}
</style>
