<script setup>
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { ElMessage } from 'element-plus'
import { Edit, Plus, Refresh, Search, Switch } from '@element-plus/icons-vue'
import {
  capabilityOptions,
  checkUpstreamDeploymentHealth,
  createProvider,
  createProviderEndpoint,
  createUpstreamDeployment,
  getUpstreamDeployment,
  listProviderEndpoints,
  listProviders,
  listUpstreamDeployments,
  protocolOptions,
  providerTypeOptions,
  statusOptions,
  updateProvider,
  updateProviderEndpoint,
  updateProviderEndpointStatus,
  updateProviderStatus,
  updateUpstreamDeployment,
  updateUpstreamDeploymentStatus
} from '@/api/aiGateway'

const loading = shallowRef(false)
const endpointLoading = shallowRef(false)
const deploymentLoading = shallowRef(false)
const providerDialogVisible = shallowRef(false)
const endpointDialogVisible = shallowRef(false)
const deploymentDialogVisible = shallowRef(false)
const editingProviderId = shallowRef('')
const editingEndpointId = shallowRef('')
const editingDeploymentId = shallowRef('')
const providers = shallowRef([])
const endpoints = shallowRef([])
const deployments = shallowRef([])
const selectedProviderId = shallowRef('')
const providerKeyword = shallowRef('')

const providerForm = reactive({
  code: '',
  name: '',
  provider_type: 'custom',
  is_custom: true,
  status: 'active'
})

const endpointForm = reactive({
  name: '',
  base_url: '',
  api_key: '',
  weight: 100,
  timeout_ms: 60000,
  status: 'active'
})

const deploymentForm = reactive({
  endpoint_id: '',
  name: '',
  upstream_model: '',
  capability_type: 'chat',
  upstream_protocol: 'openai_chat_completions',
  request_path: '',
  upstream_parameters: '',
  tags: '',
  status: 'active'
})

const selectedProvider = computed(() =>
  providers.value.find((item) => item.id === selectedProviderId.value)
)

const filteredProviders = computed(() => {
  const keyword = providerKeyword.value.trim().toLowerCase()
  if (!keyword) return providers.value
  return providers.value.filter((item) => {
    const name = item.name?.toLowerCase() || ''
    const code = item.code?.toLowerCase() || ''
    const providerType = item.provider_type?.toLowerCase() || ''
    return name.includes(keyword) || code.includes(keyword) || providerType.includes(keyword)
  })
})

const isEditingProvider = computed(() => Boolean(editingProviderId.value))
const isEditingEndpoint = computed(() => Boolean(editingEndpointId.value))
const isEditingDeployment = computed(() => Boolean(editingDeploymentId.value))

const providerSummary = computed(() => ({
  endpointCount: endpoints.value.length,
  activeEndpointCount: endpoints.value.filter((item) => item.status === 'active').length,
  deploymentCount: deployments.value.length,
  activeDeploymentCount: deployments.value.filter((item) => item.status === 'active').length
}))

const setupSteps = [
  '创建厂商分组，标记上游归属和厂商类型。',
  '添加连接配置，配置 Base URL、Key、额外 Headers、权重和超时。',
  '到模型映射页面维护调用配置、协议、请求路径和上游成本价。',
  '在授权与 Key 页面分配对外模型调用权限。'
]

const statusTagType = (status) => {
  const map = { active: 'success', inactive: 'warning', disabled: 'danger' }
  return map[status] || 'info'
}

const providerTypeLabel = (type) =>
  providerTypeOptions.find((item) => item.value === type)?.label || type || '-'

const protocolLabel = (value) =>
  protocolOptions.find((item) => item.value === value)?.label || value || '-'

const capabilityLabel = (value) =>
  capabilityOptions.find((item) => item.value === value)?.label || value || '-'

const getEndpointName = (endpointId) => {
  const endpoint = endpoints.value.find((item) => item.id === endpointId)
  return endpoint?.name || endpointId || '-'
}

const resetProviderForm = () => {
  editingProviderId.value = ''
  Object.assign(providerForm, {
    code: '',
    name: '',
    provider_type: 'custom',
    is_custom: true,
    status: 'active'
  })
}

const resetEndpointForm = () => {
  editingEndpointId.value = ''
  Object.assign(endpointForm, {
    name: '',
    base_url: '',
    api_key: '',
    weight: 100,
    timeout_ms: 60000,
    status: 'active'
  })
}

const resetDeploymentForm = () => {
  editingDeploymentId.value = ''
  Object.assign(deploymentForm, {
    endpoint_id: '',
    name: '',
    upstream_model: '',
    capability_type: 'chat',
    upstream_protocol: 'openai_chat_completions',
    request_path: '',
    upstream_parameters: '',
    tags: '',
    status: 'active'
  })
}

const applyProviderForm = (row) => {
  editingProviderId.value = row.id
  Object.assign(providerForm, {
    code: row.code,
    name: row.name,
    provider_type: row.provider_type,
    is_custom: row.is_custom,
    status: row.status
  })
}

const applyEndpointForm = (row) => {
  editingEndpointId.value = row.id
  Object.assign(endpointForm, {
    name: row.name,
    base_url: row.base_url,
    api_key: '',
    weight: row.weight,
    timeout_ms: row.timeout_ms,
    status: row.status
  })
}

const applyDeploymentForm = (row) => {
  editingDeploymentId.value = row.id
  Object.assign(deploymentForm, {
    endpoint_id: row.endpoint_id || '',
    name: row.name || '',
    upstream_model: row.upstream_model || '',
    capability_type: row.capability_type || 'chat',
    upstream_protocol: row.upstream_protocol || 'openai_chat_completions',
    request_path: row.request_path || '',
    upstream_parameters: typeof row.upstream_parameters === 'object' ? JSON.stringify(row.upstream_parameters, null, 2) : (row.upstream_parameters || ''),
    tags: typeof row.tags === 'object' ? JSON.stringify(row.tags, null, 2) : (row.tags || ''),
    status: row.status || 'active'
  })
}

const fetchProviders = async () => {
  loading.value = true
  try {
    providers.value = await listProviders()
    if (providers.value.length === 0) {
      selectedProviderId.value = ''
      endpoints.value = []
      deployments.value = []
      return
    }
    if (!providers.value.some((item) => item.id === selectedProviderId.value)) {
      selectedProviderId.value = providers.value[0].id
    }
    await fetchSelectedProviderDetail()
  } finally {
    loading.value = false
  }
}

const fetchSelectedProviderDetail = async () => {
  await Promise.all([fetchEndpoints(), fetchDeployments()])
}

const fetchEndpoints = async () => {
  if (!selectedProviderId.value) {
    endpoints.value = []
    return
  }
  endpointLoading.value = true
  try {
    endpoints.value = await listProviderEndpoints(selectedProviderId.value)
  } finally {
    endpointLoading.value = false
  }
}

const fetchDeployments = async () => {
  if (!selectedProviderId.value) {
    deployments.value = []
    return
  }
  deploymentLoading.value = true
  try {
    deployments.value = await listUpstreamDeployments({ provider_id: selectedProviderId.value })
  } finally {
    deploymentLoading.value = false
  }
}

const selectProvider = async (row) => {
  selectedProviderId.value = row?.id || ''
  await fetchSelectedProviderDetail()
}

const openProviderDialog = () => {
  resetProviderForm()
  providerDialogVisible.value = true
}

const openProviderEditDialog = async (row) => {
  if (row.id !== selectedProviderId.value) {
    selectedProviderId.value = row.id
    await fetchSelectedProviderDetail()
  }
  applyProviderForm(row)
  providerDialogVisible.value = true
}

const openEndpointDialog = () => {
  if (!selectedProviderId.value) {
    ElMessage.warning('请先选择厂商')
    return
  }
  resetEndpointForm()
  endpointDialogVisible.value = true
}

const openEndpointEditDialog = (row) => {
  if (!selectedProviderId.value) {
    ElMessage.warning('请先选择厂商')
    return
  }
  applyEndpointForm(row)
  endpointDialogVisible.value = true
}

const openDeploymentDialog = () => {
  if (!selectedProviderId.value) {
    ElMessage.warning('请先选择厂商')
    return
  }
  if (endpoints.value.length === 0) {
    ElMessage.warning('请先创建接入点')
    return
  }
  resetDeploymentForm()
  deploymentDialogVisible.value = true
}

const openDeploymentEditDialog = async (row) => {
  if (!selectedProviderId.value) {
    ElMessage.warning('请先选择厂商')
    return
  }
  applyDeploymentForm(row)
  deploymentDialogVisible.value = true
}

const submitProvider = async () => {
  if (editingProviderId.value) {
    await updateProvider(editingProviderId.value, providerForm)
    ElMessage.success('厂商已保存')
  } else {
    await createProvider(providerForm)
    ElMessage.success('厂商已创建')
  }
  providerDialogVisible.value = false
  await fetchProviders()
}

const submitEndpoint = async () => {
  const payload = { ...endpointForm }
  if (editingEndpointId.value) {
    await updateProviderEndpoint(selectedProviderId.value, editingEndpointId.value, payload)
    ElMessage.success('接入点已保存')
  } else {
    await createProviderEndpoint(selectedProviderId.value, payload)
    ElMessage.success('接入点已创建')
  }
  endpointDialogVisible.value = false
  await fetchEndpoints()
}

const submitDeployment = async () => {
  const payload = {
    endpoint_id: deploymentForm.endpoint_id,
    name: deploymentForm.name,
    upstream_model: deploymentForm.upstream_model,
    capability_type: deploymentForm.capability_type,
    upstream_protocol: deploymentForm.upstream_protocol,
    request_path: deploymentForm.request_path || null,
    upstream_parameters: deploymentForm.upstream_parameters ? JSON.parse(deploymentForm.upstream_parameters) : null,
    tags: deploymentForm.tags ? JSON.parse(deploymentForm.tags) : null,
    status: deploymentForm.status
  }
  if (editingDeploymentId.value) {
    await updateUpstreamDeployment(editingDeploymentId.value, payload)
    ElMessage.success('上游部署已保存')
  } else {
    await createUpstreamDeployment(payload)
    ElMessage.success('上游部署已创建')
  }
  deploymentDialogVisible.value = false
  await fetchDeployments()
}

const toggleProvider = async (row) => {
  const nextStatus = row.status === 'active' ? 'disabled' : 'active'
  await updateProviderStatus(row.id, nextStatus)
  ElMessage.success('厂商状态已更新')
  await fetchProviders()
}

const toggleEndpoint = async (row) => {
  const nextStatus = row.status === 'active' ? 'disabled' : 'active'
  await updateProviderEndpointStatus(selectedProviderId.value, row.id, nextStatus)
  ElMessage.success('接入点状态已更新')
  await fetchEndpoints()
}

const toggleDeployment = async (row) => {
  const nextStatus = row.status === 'active' ? 'disabled' : 'active'
  await updateUpstreamDeploymentStatus(row.id, nextStatus)
  ElMessage.success('上游部署状态已更新')
  await fetchDeployments()
}

const handleHealthCheck = async (row) => {
  try {
    const result = await checkUpstreamDeploymentHealth(row.id)
    if (result.healthy) {
      ElMessage.success(`健康检查通过: ${result.message || '连接正常'}`)
    } else {
      ElMessage.warning(`健康检查失败: ${result.message || '连接异常'}`)
    }
  } catch {
    // Error already handled by interceptor
  }
}

onMounted(fetchProviders)
</script>

<template>
  <div class="provider-workbench">
    <aside class="provider-rail">
      <div class="rail-head">
        <div>
          <p class="eyebrow">Provider Directory</p>
          <h2>厂商接入</h2>
        </div>
        <el-button type="primary" :icon="Plus" @click="openProviderDialog">新增</el-button>
      </div>

      <el-input
        v-model="providerKeyword"
        :prefix-icon="Search"
        clearable
        placeholder="搜索厂商、编码、类型"
        class="provider-search"
      />

      <div v-loading="loading" class="provider-list">
        <div v-if="filteredProviders.length === 0" class="empty-list">
          暂无匹配厂商
        </div>
        <div
          v-for="provider in filteredProviders"
          :key="provider.id"
          class="provider-item"
          :class="{ active: provider.id === selectedProviderId }"
          @click="selectProvider(provider)"
        >
          <div class="provider-item-main">
            <div class="provider-item-title">
              <strong>{{ provider.name }}</strong>
              <el-tag :type="statusTagType(provider.status)" size="small">{{ provider.status }}</el-tag>
            </div>
            <span>{{ provider.code }}</span>
            <small>{{ providerTypeLabel(provider.provider_type) }}</small>
          </div>
          <div class="provider-item-actions">
            <el-button link type="primary" :icon="Edit" @click.stop="openProviderEditDialog(provider)">编辑</el-button>
            <el-button link :type="provider.status === 'active' ? 'warning' : 'success'" @click.stop="toggleProvider(provider)">
              {{ provider.status === 'active' ? '禁用' : '启用' }}
            </el-button>
          </div>
        </div>
      </div>
    </aside>

    <main v-if="selectedProvider" class="provider-workspace">
      <section class="workspace-hero">
        <div class="hero-main">
          <p class="eyebrow">Current Provider</p>
          <div class="hero-title-row">
            <h2>{{ selectedProvider.name }}</h2>
            <el-tag :type="statusTagType(selectedProvider.status)" effect="dark">{{ selectedProvider.status }}</el-tag>
          </div>
          <p>{{ selectedProvider.code }} · {{ providerTypeLabel(selectedProvider.provider_type) }}</p>
        </div>
        <div class="hero-actions">
          <el-button :icon="Refresh" circle @click="fetchSelectedProviderDetail" />
          <el-button :icon="Edit" @click="openProviderEditDialog(selectedProvider)">编辑厂商</el-button>
          <el-button :type="selectedProvider.status === 'active' ? 'warning' : 'success'" @click="toggleProvider(selectedProvider)">
            {{ selectedProvider.status === 'active' ? '禁用厂商' : '启用厂商' }}
          </el-button>
        </div>

        <div class="metric-grid">
          <div class="metric-cell">
            <span>接入点</span>
            <strong>{{ providerSummary.endpointCount }}</strong>
            <small>{{ providerSummary.activeEndpointCount }} active</small>
          </div>
          <div class="metric-cell">
            <span>上游部署</span>
            <strong>{{ providerSummary.deploymentCount }}</strong>
            <small>{{ providerSummary.activeDeploymentCount }} active</small>
          </div>
          <div class="metric-cell">
            <span>厂商类型</span>
            <strong>{{ providerTypeLabel(selectedProvider.provider_type) }}</strong>
            <small>Provider 只表示归属</small>
          </div>
          <div class="metric-cell">
            <span>计费单位</span>
            <strong>积分</strong>
            <small>成本价在部署维护</small>
          </div>
        </div>
      </section>

      <section class="guide-panel">
        <div>
          <p class="eyebrow">Setup Flow</p>
          <h3>配置流程</h3>
        </div>
        <ol>
          <li v-for="step in setupSteps" :key="step">{{ step }}</li>
        </ol>
        <p class="guide-note">文本类接口按 token 计成本，图片类接口按 image_count 计成本；所有成本字段统一使用整数积分。</p>
      </section>

      <section class="panel">
        <div class="section-head">
          <div>
            <h3>接入点管理</h3>
            <p>配置当前厂商的连接配置：Base URL、密钥、额外 Headers、权重和超时</p>
          </div>
          <el-button type="primary" :icon="Plus" @click="openEndpointDialog">新增接入点</el-button>
        </div>

        <el-table v-loading="endpointLoading" :data="endpoints" border stripe class="w-full">
          <el-table-column prop="name" label="名称" min-width="160" />
          <el-table-column prop="base_url" label="Base URL" min-width="280" show-overflow-tooltip />
          <el-table-column prop="weight" label="权重" width="80" align="right" />
          <el-table-column prop="timeout_ms" label="超时(ms)" width="110" align="right" />
          <el-table-column label="状态" width="95">
            <template #default="{ row }">
              <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="190" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" :icon="Edit" @click="openEndpointEditDialog(row)">编辑</el-button>
              <el-button link :type="row.status === 'active' ? 'warning' : 'success'" @click="toggleEndpoint(row)">
                <el-icon><Switch /></el-icon>
                {{ row.status === 'active' ? '禁用' : '启用' }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </section>

      <section class="panel">
        <div class="section-head">
          <div>
            <h3>上游部署管理</h3>
            <p>配置当前厂商的上游模型映射：协议类型、请求路径和上游模型名</p>
          </div>
          <el-button type="primary" :icon="Plus" @click="openDeploymentDialog">新增部署</el-button>
        </div>

        <el-table v-loading="deploymentLoading" :data="deployments" border stripe class="w-full">
          <el-table-column prop="name" label="名称" min-width="140" />
          <el-table-column label="关联接入点" min-width="140">
            <template #default="{ row }">
              {{ getEndpointName(row.endpoint_id) }}
            </template>
          </el-table-column>
          <el-table-column prop="upstream_model" label="上游模型" min-width="160" show-overflow-tooltip />
          <el-table-column label="能力类型" width="110">
            <template #default="{ row }">
              <el-tag size="small">{{ capabilityLabel(row.capability_type) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="协议类型" width="180">
            <template #default="{ row }">
              <span class="protocol-text">{{ protocolLabel(row.upstream_protocol) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="95">
            <template #default="{ row }">
              <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="240" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" :icon="Edit" @click="openDeploymentEditDialog(row)">编辑</el-button>
              <el-button link :type="row.status === 'active' ? 'warning' : 'success'" @click="toggleDeployment(row)">
                <el-icon><Switch /></el-icon>
                {{ row.status === 'active' ? '禁用' : '启用' }}
              </el-button>
              <el-button link type="info" @click="handleHealthCheck(row)">健康检查</el-button>
            </template>
          </el-table-column>
        </el-table>
      </section>
    </main>

    <main v-else class="empty-workspace">
      <p class="eyebrow">No Provider</p>
      <h2>先创建一个服务商</h2>
      <p>服务商是连接配置和后续模型映射的上游归属。创建后即可在右侧完成接入点配置。</p>
      <el-button type="primary" :icon="Plus" @click="openProviderDialog">新增服务商</el-button>
    </main>

    <el-dialog v-model="providerDialogVisible" :title="isEditingProvider ? '编辑服务商' : '新增服务商'" width="560px" append-to-body>
      <el-form :model="providerForm" label-position="top">
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="厂商编码" required>
            <el-input v-model="providerForm.code" placeholder="custom_vendor" />
          </el-form-item>
          <el-form-item label="厂商名称" required>
            <el-input v-model="providerForm.name" placeholder="Custom Vendor" />
          </el-form-item>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="厂商类型">
            <el-select v-model="providerForm.provider_type" class="w-full">
              <el-option v-for="item in providerTypeOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="状态">
            <el-select v-model="providerForm.status" class="w-full">
              <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="providerDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitProvider">{{ isEditingProvider ? '保存' : '创建' }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="endpointDialogVisible" :title="isEditingEndpoint ? '编辑接入点' : '新增接入点'" width="640px" append-to-body>
      <el-form :model="endpointForm" label-position="top">
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="接入点名称" required>
            <el-input v-model="endpointForm.name" placeholder="OpenAI Compatible Endpoint" />
          </el-form-item>
          <el-form-item label="状态">
            <el-select v-model="endpointForm.status" class="w-full">
              <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item label="Base URL" required>
          <el-input v-model="endpointForm.base_url" placeholder="https://example.com/v1" />
        </el-form-item>
        <el-form-item :label="isEditingEndpoint ? '上游 API Key（留空不修改）' : '上游 API Key'" :required="!isEditingEndpoint">
          <el-input v-model="endpointForm.api_key" type="password" show-password placeholder="后端加密保存" />
        </el-form-item>
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="权重">
            <el-input-number v-model="endpointForm.weight" :min="0" :precision="0" class="w-full" />
          </el-form-item>
          <el-form-item label="超时">
            <el-input-number v-model="endpointForm.timeout_ms" :min="1000" class="w-full" />
          </el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="endpointDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitEndpoint">{{ isEditingEndpoint ? '保存' : '创建' }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="deploymentDialogVisible" :title="isEditingDeployment ? '编辑上游部署' : '新增上游部署'" width="640px" append-to-body>
      <el-form :model="deploymentForm" label-position="top">
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="关联接入点" required>
            <el-select v-model="deploymentForm.endpoint_id" placeholder="选择接入点" class="w-full">
              <el-option v-for="item in endpoints" :key="item.id" :label="item.name" :value="item.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="部署名称" required>
            <el-input v-model="deploymentForm.name" placeholder="GPT-4o Deployment" />
          </el-form-item>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="上游模型名" required>
            <el-input v-model="deploymentForm.upstream_model" placeholder="gpt-4o" />
          </el-form-item>
          <el-form-item label="状态">
            <el-select v-model="deploymentForm.status" class="w-full">
              <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="能力类型">
            <el-select v-model="deploymentForm.capability_type" class="w-full">
              <el-option v-for="item in capabilityOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="协议类型">
            <el-select v-model="deploymentForm.upstream_protocol" class="w-full">
              <el-option v-for="item in protocolOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item label="请求路径（可选）">
          <el-input v-model="deploymentForm.request_path" placeholder="/v1/chat/completions" />
        </el-form-item>
        <el-form-item label="上游参数 JSON（可选）">
          <el-input v-model="deploymentForm.upstream_parameters" type="textarea" :rows="3" placeholder='{"temperature": 0.7}' />
        </el-form-item>
        <el-form-item label="标签 JSON（可选）">
          <el-input v-model="deploymentForm.tags" type="textarea" :rows="2" placeholder='{"env": "prod", "tier": "premium"}' />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="deploymentDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitDeployment">{{ isEditingDeployment ? '保存' : '创建' }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.provider-workbench {
  display: grid;
  grid-template-columns: minmax(320px, 360px) minmax(0, 1fr);
  gap: 18px;
  align-items: start;
}

.provider-rail,
.provider-workspace,
.empty-workspace,
.panel,
.guide-panel,
.workspace-hero {
  min-width: 0;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #ffffff;
}

.provider-rail {
  position: sticky;
  top: 16px;
  padding: 16px;
}

.rail-head,
.section-head,
.hero-title-row,
.hero-actions {
  display: flex;
  align-items: center;
}

.rail-head,
.section-head {
  justify-content: space-between;
  gap: 16px;
}

.eyebrow {
  margin: 0 0 4px;
  color: #64748b;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0;
  text-transform: uppercase;
}

.rail-head h2,
.workspace-hero h2,
.empty-workspace h2 {
  margin: 0;
  color: #111827;
  font-size: 22px;
  font-weight: 900;
}

.provider-search {
  margin: 16px 0;
}

.provider-list {
  display: flex;
  min-height: 260px;
  max-height: calc(100vh - 260px);
  flex-direction: column;
  gap: 10px;
  overflow-y: auto;
  padding-right: 2px;
}

.provider-item {
  display: flex;
  cursor: pointer;
  flex-direction: column;
  gap: 12px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #f9fafb;
  padding: 12px;
  transition: border-color 0.16s ease, background-color 0.16s ease, box-shadow 0.16s ease;
}

.provider-item:hover {
  border-color: #cbd5e1;
  background: #ffffff;
}

.provider-item.active {
  border-color: #2563eb;
  background: #eff6ff;
  box-shadow: 0 10px 24px rgba(37, 99, 235, 0.12);
}

.provider-item-main {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 4px;
}

.provider-item-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.provider-item-title strong {
  overflow: hidden;
  color: #111827;
  font-size: 14px;
  font-weight: 900;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.provider-item span,
.provider-item small {
  overflow: hidden;
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.provider-item small {
  color: #94a3b8;
  font-size: 11px;
}

.provider-item-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  border-top: 1px solid rgba(148, 163, 184, 0.25);
  padding-top: 8px;
}

.empty-list {
  display: flex;
  min-height: 180px;
  align-items: center;
  justify-content: center;
  border: 1px dashed #cbd5e1;
  border-radius: 8px;
  color: #94a3b8;
  font-size: 13px;
  font-weight: 700;
}

.provider-workspace {
  display: flex;
  flex-direction: column;
  gap: 16px;
  border: 0;
  background: transparent;
}

.workspace-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 18px;
  padding: 20px;
}

.hero-main {
  min-width: 0;
}

.hero-title-row {
  flex-wrap: wrap;
  gap: 10px;
}

.workspace-hero p:not(.eyebrow) {
  margin: 8px 0 0;
  color: #64748b;
  font-size: 13px;
  font-weight: 700;
}

.hero-actions {
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.metric-grid {
  grid-column: 1 / -1;
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
  margin-bottom: 14px;
}

.section-head p {
  margin: 4px 0 0;
  color: #64748b;
}

.protocol-text {
  overflow: hidden;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.empty-workspace {
  display: flex;
  min-height: 420px;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  padding: 40px;
}

.empty-workspace p:not(.eyebrow) {
  max-width: 520px;
  margin: 12px 0 22px;
  color: #64748b;
  font-size: 14px;
  font-weight: 700;
  line-height: 1.7;
}

@media (max-width: 1280px) {
  .provider-workbench {
    grid-template-columns: 1fr;
  }

  .provider-rail {
    position: static;
  }

  .provider-list {
    max-height: none;
  }
}

@media (max-width: 900px) {
  .workspace-hero,
  .guide-panel,
  .metric-grid,
  .guide-panel ol {
    grid-template-columns: 1fr;
  }

  .guide-note {
    grid-column: auto;
  }

  .hero-actions {
    justify-content: flex-start;
  }
}
</style>
