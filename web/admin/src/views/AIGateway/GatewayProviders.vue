<script setup>
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { ElMessage } from 'element-plus'
import { Edit, Plus, Refresh, Search, Switch } from '@element-plus/icons-vue'
import {
  checkProviderEndpointHealth,
  createProviderModelPrice,
  createProvider,
  createProviderEndpoint,
  formatCredits,
  formatTimestamp,
  listProviderModelPrices,
  listProviderEndpoints,
  listProviders,
  nowTimestamp,
  providerTypeOptions,
  protocolOptions,
  statusOptions,
  updateProvider,
  updateProviderEndpoint,
  updateProviderEndpointStatus,
  updateProviderModelPrice,
  updateProviderModelPriceStatus,
  updateProviderStatus
} from '@/api/aiGateway'

const loading = shallowRef(false)
const endpointLoading = shallowRef(false)
const priceLoading = shallowRef(false)
const providerDialogVisible = shallowRef(false)
const endpointDialogVisible = shallowRef(false)
const priceDialogVisible = shallowRef(false)
const editingProviderId = shallowRef('')
const editingEndpointId = shallowRef('')
const editingPriceId = shallowRef('')
const providers = shallowRef([])
const endpoints = shallowRef([])
const providerPrices = shallowRef([])
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
  protocol_type: 'openai_chat_completions',
  api_key: '',
  custom_path: '',
  weight: 100,
  timeout_ms: 60000,
  status: 'active'
})

const priceForm = reactive({
  endpoint_id: '',
  upstream_model: '',
  capability_type: 'chat',
  currency: 'CNY_CREDITS',
  input_cost_per_1m: 0,
  output_cost_per_1m: 0,
  request_cost: 0,
  image_cost: 0,
  video_cost_per_second: 0,
  effective_from: '',
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
const isEditingPrice = computed(() => Boolean(editingPriceId.value))

const endpointOptions = computed(() =>
  endpoints.value.map((item) => ({
    label: item.name,
    value: item.id
  }))
)

const endpointNameById = computed(() =>
  endpoints.value.reduce((map, item) => {
    map[item.id] = item.name
    return map
  }, {})
)

const providerSummary = computed(() => ({
  endpointCount: endpoints.value.length,
  activeEndpointCount: endpoints.value.filter((item) => item.status === 'active').length,
  priceCount: providerPrices.value.length,
  activePriceCount: providerPrices.value.filter((item) => item.status === 'active').length
}))

const setupSteps = [
  '创建厂商分组，标记上游归属和厂商类型。',
  '添加接入点，配置 Base URL、Key、权重和超时。',
  '维护 Provider 成本价，平台成本和毛利统计以这里为准。',
  '到模型映射页面绑定公开模型和部署，再进入授权与 Key 分配调用权限。'
]

const statusTagType = (status) => {
  const map = { active: 'success', inactive: 'warning', disabled: 'danger' }
  return map[status] || 'info'
}

const providerTypeLabel = (type) =>
  providerTypeOptions.find((item) => item.value === type)?.label || type || '-'

const endpointLabel = (endpointId) => endpointNameById.value[endpointId] || endpointId || '全局'

const resetPriceForm = () => {
  editingPriceId.value = ''
  Object.assign(priceForm, {
    endpoint_id: '',
    upstream_model: '',
    capability_type: 'chat',
    currency: 'CNY_CREDITS',
    input_cost_per_1m: 0,
    output_cost_per_1m: 0,
    request_cost: 0,
    image_cost: 0,
    video_cost_per_second: 0,
    effective_from: String(nowTimestamp()),
    status: 'active'
  })
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
    protocol_type: 'openai_chat_completions',
    api_key: '',
    custom_path: '',
    weight: 100,
    timeout_ms: 60000,
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
    protocol_type: row.protocol_type,
    api_key: '',
    custom_path: row.custom_path || '',
    weight: row.weight,
    timeout_ms: row.timeout_ms,
    status: row.status
  })
}

const applyPriceForm = (row) => {
  editingPriceId.value = row.id
  Object.assign(priceForm, {
    endpoint_id: row.endpoint_id || '',
    upstream_model: row.upstream_model,
    capability_type: row.capability_type,
    currency: row.currency,
    input_cost_per_1m: row.input_cost_per_1m,
    output_cost_per_1m: row.output_cost_per_1m,
    request_cost: row.request_cost,
    image_cost: row.image_cost,
    video_cost_per_second: row.video_cost_per_second,
    effective_from: row.effective_from ? String(row.effective_from) : String(nowTimestamp()),
    status: row.status
  })
}

const fetchProviders = async () => {
  loading.value = true
  try {
    providers.value = await listProviders()
    if (providers.value.length === 0) {
      selectedProviderId.value = ''
      endpoints.value = []
      providerPrices.value = []
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
  await Promise.all([fetchEndpoints(), fetchProviderPrices()])
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

const fetchProviderPrices = async () => {
  if (!selectedProviderId.value) {
    providerPrices.value = []
    return
  }
  priceLoading.value = true
  try {
    providerPrices.value = await listProviderModelPrices(selectedProviderId.value)
  } finally {
    priceLoading.value = false
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

const openPriceDialog = () => {
  if (!selectedProviderId.value) {
    ElMessage.warning('请先选择厂商')
    return
  }
  resetPriceForm()
  priceDialogVisible.value = true
}

const openPriceEditDialog = (row) => {
  if (!selectedProviderId.value) {
    ElMessage.warning('请先选择厂商')
    return
  }
  applyPriceForm(row)
  priceDialogVisible.value = true
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
  const payload = {
    ...endpointForm,
    custom_path: endpointForm.custom_path || undefined
  }
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

const submitProviderPrice = async () => {
  const payload = {
    ...priceForm,
    endpoint_id: priceForm.endpoint_id || '',
    effective_from: priceForm.effective_from ? Number(priceForm.effective_from) : nowTimestamp()
  }
  if (editingPriceId.value) {
    await updateProviderModelPrice(selectedProviderId.value, editingPriceId.value, payload)
    ElMessage.success('成本价已保存')
  } else {
    await createProviderModelPrice(selectedProviderId.value, payload)
    ElMessage.success('成本价已创建')
  }
  priceDialogVisible.value = false
  await fetchProviderPrices()
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

const toggleProviderPrice = async (row) => {
  const nextStatus = row.status === 'active' ? 'disabled' : 'active'
  await updateProviderModelPriceStatus(selectedProviderId.value, row.id, nextStatus)
  ElMessage.success('成本价状态已更新')
  await fetchProviderPrices()
}

const checkEndpoint = async (row) => {
  await checkProviderEndpointHealth(selectedProviderId.value, row.id)
  ElMessage.success('健康检查完成')
  await fetchEndpoints()
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
            <span>成本价</span>
            <strong>{{ providerSummary.priceCount }}</strong>
            <small>{{ providerSummary.activePriceCount }} active</small>
          </div>
          <div class="metric-cell">
            <span>厂商类型</span>
            <strong>{{ providerTypeLabel(selectedProvider.provider_type) }}</strong>
            <small>协议由接入点决定</small>
          </div>
          <div class="metric-cell">
            <span>计费单位</span>
            <strong>积分</strong>
            <small>整数积分，按能力区分</small>
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
            <p>配置当前厂商的上游地址、密钥、权重、超时和健康检查</p>
          </div>
          <el-button type="primary" :icon="Plus" @click="openEndpointDialog">新增接入点</el-button>
        </div>

        <el-table v-loading="endpointLoading" :data="endpoints" border stripe class="w-full">
          <el-table-column prop="name" label="名称" min-width="160" />
          <el-table-column prop="base_url" label="Base URL" min-width="280" show-overflow-tooltip />
          <el-table-column prop="protocol_type" label="协议" min-width="190" show-overflow-tooltip />
          <el-table-column prop="weight" label="权重" width="80" align="right" />
          <el-table-column prop="timeout_ms" label="超时(ms)" width="110" align="right" />
          <el-table-column label="健康" width="95">
            <template #default="{ row }">
              <el-tag size="small" type="info">{{ row.health_status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="95">
            <template #default="{ row }">
              <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="190" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" :icon="Edit" @click="openEndpointEditDialog(row)">编辑</el-button>
              <el-button link type="primary" @click="checkEndpoint(row)">检查</el-button>
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
            <h3>Provider 成本价管理</h3>
            <p>维护上游模型成本价，平台成本、毛利和用量报表使用这里的积分价格</p>
          </div>
          <div class="flex gap-2">
            <el-button :icon="Refresh" circle @click="fetchProviderPrices" />
            <el-button type="primary" :icon="Plus" @click="openPriceDialog">新增成本价</el-button>
          </div>
        </div>

        <el-table v-loading="priceLoading" :data="providerPrices" border stripe class="w-full">
          <el-table-column prop="upstream_model" label="上游模型" min-width="170" />
          <el-table-column prop="capability_type" label="能力" width="90" />
          <el-table-column label="接入点" min-width="180" show-overflow-tooltip>
            <template #default="{ row }">{{ endpointLabel(row.endpoint_id) }}</template>
          </el-table-column>
          <el-table-column label="输入/1M(积分)" width="150" align="right">
            <template #default="{ row }">{{ formatCredits(row.input_cost_per_1m) }}</template>
          </el-table-column>
          <el-table-column label="输出/1M(积分)" width="150" align="right">
            <template #default="{ row }">{{ formatCredits(row.output_cost_per_1m) }}</template>
          </el-table-column>
          <el-table-column label="请求(积分)" width="120" align="right">
            <template #default="{ row }">{{ formatCredits(row.request_cost) }}</template>
          </el-table-column>
          <el-table-column label="单图(积分)" width="120" align="right">
            <template #default="{ row }">{{ formatCredits(row.image_cost) }}</template>
          </el-table-column>
          <el-table-column label="视频/秒(积分)" width="140" align="right">
            <template #default="{ row }">{{ formatCredits(row.video_cost_per_second) }}</template>
          </el-table-column>
          <el-table-column label="生效时间" min-width="170" show-overflow-tooltip>
            <template #default="{ row }">{{ formatTimestamp(row.effective_from) }}</template>
          </el-table-column>
          <el-table-column label="状态" width="95">
            <template #default="{ row }">
              <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="130" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" :icon="Edit" @click="openPriceEditDialog(row)">编辑</el-button>
              <el-button link :type="row.status === 'active' ? 'warning' : 'success'" @click="toggleProviderPrice(row)">
                {{ row.status === 'active' ? '禁用' : '启用' }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </section>
    </main>

    <main v-else class="empty-workspace">
      <p class="eyebrow">No Provider</p>
      <h2>先创建一个服务商</h2>
      <p>服务商是接入点、成本价和后续模型映射的上游归属。创建后即可在右侧完成完整配置。</p>
      <el-button type="primary" :icon="Plus" @click="openProviderDialog">新增服务商</el-button>
    </main>

    <el-dialog v-model="providerDialogVisible" :title="isEditingProvider ? '编辑服务商' : '新增服务商'" width="560px">
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

    <el-dialog v-model="endpointDialogVisible" :title="isEditingEndpoint ? '编辑接入点' : '新增接入点'" width="640px">
      <el-form :model="endpointForm" label-position="top">
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="接入点名称" required>
            <el-input v-model="endpointForm.name" placeholder="OpenAI Compatible Endpoint" />
          </el-form-item>
          <el-form-item label="请求协议">
            <el-select v-model="endpointForm.protocol_type" class="w-full">
              <el-option v-for="item in protocolOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item label="Base URL" required>
          <el-input v-model="endpointForm.base_url" placeholder="https://example.com/v1" />
        </el-form-item>
        <el-form-item :label="isEditingEndpoint ? '上游 API Key（留空不修改）' : '上游 API Key'" :required="!isEditingEndpoint">
          <el-input v-model="endpointForm.api_key" type="password" show-password placeholder="后端加密保存" />
        </el-form-item>
        <div class="grid grid-cols-3 gap-4">
          <el-form-item label="Custom Path">
            <el-input v-model="endpointForm.custom_path" placeholder="/chat/completions" />
          </el-form-item>
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

    <el-dialog v-model="priceDialogVisible" :title="isEditingPrice ? '编辑 Provider 成本价' : '新增 Provider 成本价'" width="680px">
      <el-form :model="priceForm" label-position="top">
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="接入点">
            <el-select v-model="priceForm.endpoint_id" class="w-full" clearable>
              <el-option v-for="item in endpointOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="上游模型" required>
            <el-input v-model="priceForm.upstream_model" placeholder="provider-model-name" />
          </el-form-item>
        </div>
        <div class="grid grid-cols-3 gap-4">
          <el-form-item label="输入成本/1M(积分)">
            <el-input-number v-model="priceForm.input_cost_per_1m" :min="0" :precision="0" class="w-full" />
          </el-form-item>
          <el-form-item label="输出成本/1M(积分)">
            <el-input-number v-model="priceForm.output_cost_per_1m" :min="0" :precision="0" class="w-full" />
          </el-form-item>
          <el-form-item label="请求成本(积分)">
            <el-input-number v-model="priceForm.request_cost" :min="0" :precision="0" class="w-full" />
          </el-form-item>
          <el-form-item label="单图成本(积分)">
            <el-input-number v-model="priceForm.image_cost" :min="0" :precision="0" class="w-full" />
          </el-form-item>
          <el-form-item label="视频每秒成本(积分)">
            <el-input-number v-model="priceForm.video_cost_per_second" :min="0" :precision="0" class="w-full" />
          </el-form-item>
        </div>
        <el-form-item label="生效时间">
          <el-date-picker
            v-model="priceForm.effective_from"
            type="datetime"
            value-format="x"
            clearable
            class="w-full"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="priceDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitProviderPrice">{{ isEditingPrice ? '保存' : '创建' }}</el-button>
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
