<script setup>
import { computed, onMounted, reactive, ref, shallowRef, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Edit, Plus, Refresh, Search, Switch } from '@element-plus/icons-vue'
import {
  capabilityOptions,
  checkUpstreamDeploymentHealth,
  createProvider,
  createProviderEndpoint,
  createUpstreamDeployment,
  deleteProviderEndpoint,
  deleteUpstreamDeployment,
  endpointProtocolOptions,
  fetchEndpointUpstreamModels,
  importEndpointUpstreamModels,
  listProviderEndpoints,
  listProviders,
  listUpstreamDeployments,
  protocolOptions,
  statusLabel,
  statusOptions,
  updateProvider,
  updateProviderEndpoint,
  updateProviderEndpointStatus,
  updateProviderStatus,
  updateUpstreamDeployment,
  updateUpstreamDeploymentStatus
} from '@/api/aiGateway'
import TieredPricingEditor from './components/TieredPricingEditor.vue'
import ResolutionPricingEditor from './components/ResolutionPricingEditor.vue'
import KeyValueEditor from './components/KeyValueEditor.vue'
import { suggestPricingForModel } from './modelPricingCatalog'

const loading = shallowRef(false)
const endpointLoading = shallowRef(false)
const deploymentLoading = shallowRef(false)
const providerDialogVisible = shallowRef(false)
const endpointDialogVisible = shallowRef(false)
const deploymentDialogVisible = shallowRef(false)
const discoveryDialogVisible = shallowRef(false)
const discoveryLoading = shallowRef(false)
const discoveryImporting = shallowRef(false)
const discoveryEndpointId = shallowRef('')
const discoveryEndpointProviderId = shallowRef('')
const discoveredModels = ref([])
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
  status: 'active'
})

const endpointForm = reactive({
  name: '',
  base_url: '',
  api_key: '',
  weight: 100,
  timeout_ms: 60000,
  default_protocol: 'openai_compatible',
  status: 'active'
})

const emptyPricing = () => ({
  tiers: [{ up_to: null, input_per_1m: 0, output_per_1m: 0, cache_write_per_1m: 0, cache_read_per_1m: 0, reasoning_per_1m: 0 }],
  request_cost: 0,
  image_prices: [],
  video_prices: []
})

const deploymentForm = reactive({
  endpoint_id: '',
  upstream_model: '',
  capability_type: 'chat',
  upstream_protocol: 'openai_chat',
  request_path: '',
  upstream_parameters: {},
  pricing: emptyPricing(),
  status: 'active'
})

const pricingSuggestion = ref(null) // { label, vendor, input_per_1m, output_per_1m, cache_write_per_1m, cache_read_per_1m, reasoning_per_1m }

const isFirstTierAllZero = () => {
  const t = deploymentForm.pricing.tiers[0]
  if (!t) return true
  return !t.input_per_1m && !t.output_per_1m && !t.cache_write_per_1m && !t.cache_read_per_1m && !t.reasoning_per_1m
}

const applyPricingSuggestion = () => {
  if (!pricingSuggestion.value) return
  const s = pricingSuggestion.value
  const tiers = deploymentForm.pricing.tiers.length ? [...deploymentForm.pricing.tiers] : [{ up_to: null }]
  tiers[0] = {
    ...tiers[0],
    up_to: tiers[0].up_to ?? null,
    input_per_1m: s.input_per_1m,
    output_per_1m: s.output_per_1m,
    cache_write_per_1m: s.cache_write_per_1m,
    cache_read_per_1m: s.cache_read_per_1m,
    reasoning_per_1m: s.reasoning_per_1m
  }
  deploymentForm.pricing.tiers = tiers
  pricingSuggestion.value = null
  ElMessage.success(`已应用 ${s.label} 建议价`)
}

const onUpstreamModelBlur = () => {
  if (!deploymentForm.upstream_model) {
    pricingSuggestion.value = null
    return
  }
  if (!supportsTieredPricing(deploymentForm.capability_type)) {
    pricingSuggestion.value = null
    return
  }
  const suggestion = suggestPricingForModel(deploymentForm.upstream_model)
  if (!suggestion) {
    pricingSuggestion.value = null
    return
  }
  if (isFirstTierAllZero()) {
    pricingSuggestion.value = suggestion
    applyPricingSuggestion()
  } else {
    // 用户已填了价，仅提示
    pricingSuggestion.value = suggestion
  }
}

const imagePresets = [
  { label: '1024×1024', resolutions: ['1024x1024'] },
  { label: '1024×1792', resolutions: ['1024x1792'] },
  { label: '1792×1024', resolutions: ['1792x1024'] }
]

const videoPresets = [
  { label: '通用', resolutions: ['480p', '720p', '1080p', '4k'] },
  { label: 'Sora', resolutions: ['720x1280', '1024x1792'] },
  { label: 'Veo', resolutions: ['720p', '1080p', '4k'] }
]

const supportsTieredPricing = (cap) => cap !== 'image' && cap !== 'video'

const selectedProvider = computed(() =>
  providers.value.find((item) => item.id === selectedProviderId.value)
)

const filteredProviders = computed(() => {
  const keyword = providerKeyword.value.trim().toLowerCase()
  if (!keyword) return providers.value
  return providers.value.filter((item) => {
    const name = item.name?.toLowerCase() || ''
    const code = item.code?.toLowerCase() || ''
    return name.includes(keyword) || code.includes(keyword)
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
  '添加 API 账户，填入 Base URL、API Key、权重和超时。',
  '在模型配置中维护可调用的模型名、协议和上游成本价（含阶梯/分辨率档）。',
  '在授权与 Key 页面分配对外模型调用权限。'
]

const statusTagType = (status) => {
  const map = { active: 'success', inactive: 'warning', disabled: 'danger' }
  return map[status] || 'info'
}

const protocolLabel = (value) =>
  protocolOptions.find((item) => item.value === value)?.label || value || '-'

const capabilityLabel = (value) =>
  capabilityOptions.find((item) => item.value === value)?.label || value || '-'

const getEndpointName = (endpointId) => {
  const endpoint = endpoints.value.find((item) => item.id === endpointId)
  return endpoint?.name || endpointId || '-'
}

const parsePricing = (raw) => {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return emptyPricing()
  const tiers = Array.isArray(raw.tiers) && raw.tiers.length > 0
    ? raw.tiers.map(normalizeTier)
    : emptyPricing().tiers
  return {
    tiers,
    request_cost: Number(raw.request_cost) || 0,
    image_prices: Array.isArray(raw.image_prices) ? raw.image_prices.map(normalizeResPrice) : [],
    video_prices: Array.isArray(raw.video_prices) ? raw.video_prices.map(normalizeResPrice) : []
  }
}

const normalizeTier = (t) => ({
  up_to: t.up_to == null ? null : Number(t.up_to),
  input_per_1m: Number(t.input_per_1m) || 0,
  output_per_1m: Number(t.output_per_1m) || 0,
  cache_write_per_1m: Number(t.cache_write_per_1m) || 0,
  cache_read_per_1m: Number(t.cache_read_per_1m) || 0,
  reasoning_per_1m: Number(t.reasoning_per_1m) || 0
})

const normalizeResPrice = (r) => ({
  resolution: String(r.resolution || ''),
  price: Number(r.price) || 0
})

// 价格列展示
const firstTier = (row) => parsePricing(row.pricing).tiers[0]
const tierCount = (row) => parsePricing(row.pricing).tiers.length
const minResolutionPrice = (list) => {
  if (!list || list.length === 0) return null
  return list.reduce((acc, cur) => (cur.price < acc ? cur.price : acc), list[0].price)
}

const fmtMoney = (v) => {
  if (v == null) return '0'
  const n = Number(v)
  if (!Number.isFinite(n)) return '0'
  return Number.isInteger(n) ? String(n) : n.toFixed(2).replace(/\.?0+$/, '')
}

const pricingCellChat = (row) => {
  const t = firstTier(row)
  return { input: fmtMoney(t.input_per_1m), output: fmtMoney(t.output_per_1m) }
}

const pricingCellImage = (row) => {
  const pricing = parsePricing(row.pricing)
  const min = minResolutionPrice(pricing.image_prices)
  return min == null ? null : fmtMoney(min)
}

const pricingCellVideo = (row) => {
  const pricing = parsePricing(row.pricing)
  const min = minResolutionPrice(pricing.video_prices)
  return min == null ? null : fmtMoney(min)
}

const resetProviderForm = () => {
  editingProviderId.value = ''
  Object.assign(providerForm, {
    code: '',
    name: '',
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
    default_protocol: 'openai_compatible',
    status: 'active'
  })
}

const resetDeploymentForm = () => {
  editingDeploymentId.value = ''
  Object.assign(deploymentForm, {
    endpoint_id: '',
    upstream_model: '',
    capability_type: 'chat',
    upstream_protocol: 'openai_chat',
    request_path: '',
    upstream_parameters: {},
    pricing: emptyPricing(),
    status: 'active'
  })
}

const applyProviderForm = (row) => {
  editingProviderId.value = row.id
  Object.assign(providerForm, {
    code: row.code,
    name: row.name,
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
    default_protocol: row.default_protocol || 'openai_compatible',
    status: row.status
  })
}

const applyDeploymentForm = (row) => {
  editingDeploymentId.value = row.id
  const params = row.upstream_parameters && typeof row.upstream_parameters === 'object'
    ? row.upstream_parameters
    : {}
  Object.assign(deploymentForm, {
    endpoint_id: row.endpoint_id || '',
    upstream_model: row.upstream_model || '',
    capability_type: row.capability_type || 'chat',
    upstream_protocol: row.upstream_protocol || 'openai_chat',
    request_path: row.request_path || '',
    upstream_parameters: { ...params },
    pricing: parsePricing(row.pricing),
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
    ElMessage.warning('请先创建 API 账户')
    return
  }
  resetDeploymentForm()
  pricingSuggestion.value = null
  deploymentDialogVisible.value = true
}

const openDeploymentEditDialog = (row) => {
  if (!selectedProviderId.value) {
    ElMessage.warning('请先选择厂商')
    return
  }
  applyDeploymentForm(row)
  pricingSuggestion.value = null
  deploymentDialogVisible.value = true
}

// 切换能力类型时，清理不再适用的价格区块（避免误存）
watch(
  () => deploymentForm.capability_type,
  (cap) => {
    if (cap !== 'image') deploymentForm.pricing.image_prices = []
    if (cap !== 'video') deploymentForm.pricing.video_prices = []
    if (!supportsTieredPricing(cap) && deploymentForm.pricing.tiers.length === 0) {
      deploymentForm.pricing.tiers = emptyPricing().tiers
    }
  }
)

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
  const payload = { ...endpointForm, base_url: endpointForm.base_url.trim() }
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
  const cap = deploymentForm.capability_type
  const pricing = {
    tiers: supportsTieredPricing(cap) ? deploymentForm.pricing.tiers : [],
    request_cost: Number(deploymentForm.pricing.request_cost) || 0,
    image_prices: cap === 'image' ? deploymentForm.pricing.image_prices : [],
    video_prices: cap === 'video' ? deploymentForm.pricing.video_prices : []
  }
  const payload = {
    endpoint_id: deploymentForm.endpoint_id,
    upstream_model: deploymentForm.upstream_model,
    capability_type: cap,
    upstream_protocol: deploymentForm.upstream_protocol,
    request_path: deploymentForm.request_path || null,
    upstream_parameters: deploymentForm.upstream_parameters || {},
    pricing,
    status: deploymentForm.status
  }
  if (editingDeploymentId.value) {
    await updateUpstreamDeployment(editingDeploymentId.value, payload)
    ElMessage.success('模型配置已保存')
  } else {
    await createUpstreamDeployment(payload)
    ElMessage.success('模型配置已创建')
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

const deleteEndpoint = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除接入点「${row.name}」吗？关联的上游部署也将一并删除，此操作不可恢复。`,
      '删除接入点',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消', confirmButtonClass: 'el-button--danger' }
    )
  } catch {
    return
  }
  await deleteProviderEndpoint(selectedProviderId.value, row.id)
  ElMessage.success('接入点已删除')
  await fetchSelectedProviderDetail()
}

const toggleDeployment = async (row) => {
  const nextStatus = row.status === 'active' ? 'disabled' : 'active'
  await updateUpstreamDeploymentStatus(row.id, nextStatus)
  ElMessage.success('模型配置状态已更新')
  await fetchDeployments()
}

const deleteDeployment = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除模型配置「${row.upstream_model}」吗？关联的路由规则也将一并删除，此操作不可恢复。`,
      '删除模型配置',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消', confirmButtonClass: 'el-button--danger' }
    )
  } catch {
    return
  }
  await deleteUpstreamDeployment(row.id)
  ElMessage.success('模型配置已删除')
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

const openDiscoveryDialog = async (endpointRow) => {
  if (!selectedProviderId.value) return
  discoveryEndpointId.value = endpointRow.id
  discoveryEndpointProviderId.value = selectedProviderId.value
  discoveredModels.value = []
  discoveryDialogVisible.value = true
  discoveryLoading.value = true
  try {
    const models = await fetchEndpointUpstreamModels(selectedProviderId.value, endpointRow.id)
    discoveredModels.value = models.map((m) => ({ ...m, selected: true }))
  } catch {
    // Error already handled by interceptor
  } finally {
    discoveryLoading.value = false
  }
}

const discoverySelectAll = (val) => {
  discoveredModels.value = discoveredModels.value.map((m) => ({ ...m, selected: val }))
}

const discoveryAllSelected = computed(() =>
  discoveredModels.value.length > 0 && discoveredModels.value.every((m) => m.selected)
)

const submitDiscovery = async () => {
  const toImport = discoveredModels.value.filter((m) => m.selected)
  if (toImport.length === 0) {
    ElMessage.warning('请至少选择一个模型')
    return
  }
  discoveryImporting.value = true
  try {
    const result = await importEndpointUpstreamModels(
      discoveryEndpointProviderId.value,
      discoveryEndpointId.value,
      {
        models: toImport.map((m) => {
          const suggestion = suggestPricingForModel(m.id)
          const item = {
            upstream_model: m.id,
            capability_type: m.capability_type,
            upstream_protocol: m.upstream_protocol
          }
          if (suggestion && m.capability_type !== 'image' && m.capability_type !== 'video') {
            item.pricing = {
              tiers: [{
                up_to: null,
                input_per_1m: suggestion.input_per_1m,
                output_per_1m: suggestion.output_per_1m,
                cache_write_per_1m: suggestion.cache_write_per_1m,
                cache_read_per_1m: suggestion.cache_read_per_1m,
                reasoning_per_1m: suggestion.reasoning_per_1m
              }]
            }
          }
          return item
        })
      }
    )
    const createdCount = result.created?.length ?? 0
    const skippedCount = result.skipped?.length ?? 0
    ElMessage.success(`已创建 ${createdCount} 个部署，跳过 ${skippedCount} 个（已存在）`)
    discoveryDialogVisible.value = false
    await fetchDeployments()
  } catch {
    // Error already handled by interceptor
  } finally {
    discoveryImporting.value = false
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
              <el-tag :type="statusTagType(provider.status)" size="small">{{ statusLabel(provider.status) }}</el-tag>
            </div>
            <span>{{ provider.code }}</span>
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
            <el-tag :type="statusTagType(selectedProvider.status)" effect="dark">{{ statusLabel(selectedProvider.status) }}</el-tag>
          </div>
          <p>{{ selectedProvider.code }}</p>
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
            <span>API 账户</span>
            <strong>{{ providerSummary.endpointCount }}</strong>
            <small>{{ providerSummary.activeEndpointCount }} 启用</small>
          </div>
          <div class="metric-cell">
            <span>模型配置</span>
            <strong>{{ providerSummary.deploymentCount }}</strong>
            <small>{{ providerSummary.activeDeploymentCount }} 启用</small>
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
        <p class="guide-note">文本类按 token 计成本（支持阶梯定价），图片按分辨率/张，视频按分辨率/秒。所有金额单位均为整数积分。</p>
      </section>

      <section class="panel">
        <div class="section-head">
          <div>
            <h3>API 账户管理</h3>
            <p>配置连接到上游供应商的账户：Base URL、API Key、权重和超时</p>
          </div>
          <el-button type="primary" :icon="Plus" @click="openEndpointDialog">新增账户</el-button>
        </div>

        <el-table v-loading="endpointLoading" :data="endpoints" border stripe class="w-full">
          <el-table-column prop="name" label="名称" min-width="160" />
          <el-table-column prop="base_url" label="Base URL" min-width="220" show-overflow-tooltip />
          <el-table-column prop="weight" label="权重" width="75" align="right" />
          <el-table-column prop="timeout_ms" label="超时(ms)" width="100" align="right" />
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="statusTagType(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="310" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" :icon="Edit" @click="openEndpointEditDialog(row)">编辑</el-button>
              <el-button link type="success" @click="openDiscoveryDialog(row)">发现模型</el-button>
              <el-button link :type="row.status === 'active' ? 'warning' : 'success'" @click="toggleEndpoint(row)">
                <el-icon><Switch /></el-icon>
                {{ row.status === 'active' ? '禁用' : '启用' }}
              </el-button>
              <el-button link type="danger" @click="deleteEndpoint(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </section>

      <section class="panel">
        <div class="section-head">
          <div>
            <h3>模型配置管理</h3>
            <p>配置账户下可调用的具体模型：上游模型名、协议类型和上游成本价</p>
          </div>
          <el-button type="primary" :icon="Plus" @click="openDeploymentDialog">新增模型</el-button>
        </div>

        <el-table v-loading="deploymentLoading" :data="deployments" border stripe class="w-full">
          <el-table-column prop="upstream_model" label="模型" min-width="200" show-overflow-tooltip />
          <el-table-column label="所属账户" min-width="140">
            <template #default="{ row }">
              {{ getEndpointName(row.endpoint_id) }}
            </template>
          </el-table-column>
          <el-table-column label="能力" width="100">
            <template #default="{ row }">
              <el-tag size="small">{{ capabilityLabel(row.capability_type) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="协议" width="180">
            <template #default="{ row }">
              <span class="protocol-text">{{ protocolLabel(row.upstream_protocol) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="价格" min-width="220">
            <template #default="{ row }">
              <template v-if="row.capability_type === 'image'">
                <span v-if="pricingCellImage(row) == null" class="price-empty">—</span>
                <span v-else class="price-pill price-pill-image">
                  <span class="price-pill-label">图</span>
                  <span class="price-pill-value">¥{{ pricingCellImage(row) }}/张 起</span>
                </span>
              </template>
              <template v-else-if="row.capability_type === 'video'">
                <span v-if="pricingCellVideo(row) == null" class="price-empty">—</span>
                <span v-else class="price-pill price-pill-video">
                  <span class="price-pill-label">视</span>
                  <span class="price-pill-value">¥{{ pricingCellVideo(row) }}/s 起</span>
                </span>
              </template>
              <template v-else>
                <div class="price-pair">
                  <span class="price-pill price-pill-input">
                    <span class="price-pill-label">In</span>
                    <span class="price-pill-value">¥{{ pricingCellChat(row).input }}</span>
                  </span>
                  <span class="price-pill price-pill-output">
                    <span class="price-pill-label">Out</span>
                    <span class="price-pill-value">¥{{ pricingCellChat(row).output }}</span>
                  </span>
                  <el-tag v-if="tierCount(row) > 1" size="small" effect="plain" class="tier-badge">阶梯</el-tag>
                </div>
              </template>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="95">
            <template #default="{ row }">
              <el-tag :type="statusTagType(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="270" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" :icon="Edit" @click="openDeploymentEditDialog(row)">编辑</el-button>
              <el-button link :type="row.status === 'active' ? 'warning' : 'success'" @click="toggleDeployment(row)">
                <el-icon><Switch /></el-icon>
                {{ row.status === 'active' ? '禁用' : '启用' }}
              </el-button>
              <el-button link type="info" @click="handleHealthCheck(row)">健康检查</el-button>
              <el-button link type="danger" @click="deleteDeployment(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </section>
    </main>

    <main v-else class="empty-workspace">
      <p class="eyebrow">No Provider</p>
      <h2>先创建一个服务商</h2>
      <p>服务商是 API 账户和模型配置的归属分组。创建后即可在右侧添加 API 账户并配置可调用的模型。</p>
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
        <el-form-item label="状态">
          <el-select v-model="providerForm.status" class="w-full">
            <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="providerDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitProvider">{{ isEditingProvider ? '保存' : '创建' }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="endpointDialogVisible" :title="isEditingEndpoint ? '编辑 API 账户' : '新增 API 账户'" width="640px" append-to-body>
      <el-form :model="endpointForm" label-position="top">
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="账户名称" required>
            <el-input v-model="endpointForm.name" placeholder="OpenAI Compatible" />
          </el-form-item>
          <el-form-item label="厂商 API 风格" required>
            <el-select v-model="endpointForm.default_protocol" class="w-full">
              <el-option v-for="item in endpointProtocolOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item label="Base URL" required>
          <el-input v-model="endpointForm.base_url" placeholder="https://example.com" />
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
        <el-form-item label="状态">
          <el-select v-model="endpointForm.status" class="w-full">
            <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="endpointDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitEndpoint">{{ isEditingEndpoint ? '保存账户' : '创建账户' }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="discoveryDialogVisible" title="发现模型" width="800px" append-to-body>
      <div v-if="discoveryLoading" class="discovery-loading">
        <el-text type="info">正在从上游拉取模型列表…</el-text>
      </div>
      <template v-else-if="discoveredModels.length > 0">
        <div class="discovery-header">
          <el-checkbox :model-value="discoveryAllSelected" @change="discoverySelectAll">全选</el-checkbox>
          <el-text type="info" size="small">共 {{ discoveredModels.length }} 个模型，勾选后批量创建模型配置</el-text>
        </div>
        <el-table :data="discoveredModels" max-height="420" class="w-full">
          <el-table-column width="48">
            <template #default="{ row }">
              <el-checkbox v-model="row.selected" />
            </template>
          </el-table-column>
          <el-table-column prop="id" label="模型 ID" min-width="180" show-overflow-tooltip />
          <el-table-column label="能力类型" width="140">
            <template #default="{ row }">
              <el-select v-model="row.capability_type" size="small" class="w-full">
                <el-option v-for="item in capabilityOptions" :key="item.value" :label="item.label" :value="item.value" />
              </el-select>
            </template>
          </el-table-column>
          <el-table-column label="协议类型" min-width="180">
            <template #default="{ row }">
              <el-select v-model="row.upstream_protocol" size="small" class="w-full">
                <el-option v-for="item in protocolOptions" :key="item.value" :label="item.label" :value="item.value" />
              </el-select>
            </template>
          </el-table-column>
        </el-table>
      </template>
      <div v-else class="discovery-empty">
        <el-text type="warning">未能获取到模型列表，请检查接入点配置和 API Key</el-text>
      </div>
      <template #footer>
        <el-button @click="discoveryDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="discoveryImporting" :disabled="discoveryLoading || discoveredModels.length === 0" @click="submitDiscovery">
          导入选中
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="deploymentDialogVisible" :title="isEditingDeployment ? '编辑模型配置' : '新增模型配置'" width="860px" append-to-body>
      <el-form :model="deploymentForm" label-position="top">
        <div class="section-card">
          <p class="section-title">基础信息</p>
          <div class="grid grid-cols-2 gap-4">
            <el-form-item label="所属 API 账户" required>
              <el-select v-model="deploymentForm.endpoint_id" placeholder="选择账户" class="w-full">
                <el-option v-for="item in endpoints" :key="item.id" :label="item.name" :value="item.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="上游模型名" required>
              <el-input
                v-model="deploymentForm.upstream_model"
                placeholder="gpt-4o"
                @blur="onUpstreamModelBlur"
              />
            </el-form-item>
          </div>
          <div class="grid grid-cols-3 gap-4">
            <el-form-item label="能力类型">
              <el-select v-model="deploymentForm.capability_type" class="w-full">
                <el-option v-for="item in capabilityOptions" :key="item.value" :label="item.label" :value="item.value" />
              </el-select>
            </el-form-item>
            <el-form-item label="协议类型">
              <el-select v-model="deploymentForm.upstream_protocol" class="w-full">
                <el-option v-for="item in protocolOptions" :key="item.value" :label="item.label" :value="item.value" />
              </el-select>
              <div class="protocol-hint">
                严格 1:1：客户端访问的协议必须与此处完全一致才会命中。例如 /v1/chat/completions 只会路由到 openai_chat 的 deployment，/v1/responses 只会路由到 openai_responses。同一上游模型支持多协议时，请分别建立两条 deployment。
              </div>
            </el-form-item>
            <el-form-item label="状态">
              <el-select v-model="deploymentForm.status" class="w-full">
                <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
              </el-select>
            </el-form-item>
          </div>
          <el-form-item label="请求路径（可选）">
            <el-input v-model="deploymentForm.request_path" placeholder="/v1/chat/completions" />
          </el-form-item>
        </div>

        <div class="section-card">
          <p class="section-title">上游参数（可选）</p>
          <p class="section-sub">用于覆盖默认请求参数，例如 temperature、top_p。值会按 JSON 类型推断（数字/布尔/字符串）。</p>
          <KeyValueEditor v-model="deploymentForm.upstream_parameters" />
        </div>

        <div class="section-card">
          <p class="section-title">价格配置</p>
          <p class="section-sub">单位：人民币（¥/百万 token）；图片/视频按张/秒。所有字段填 0 表示该项不计成本。</p>

          <el-alert
            v-if="pricingSuggestion"
            class="suggestion-banner"
            type="info"
            show-icon
            :closable="false"
          >
            <template #title>
              <span>检测到 <strong>{{ pricingSuggestion.label }}</strong>，建议价：输入 ¥{{ pricingSuggestion.input_per_1m }}/M、输出 ¥{{ pricingSuggestion.output_per_1m }}/M</span>
              <el-button type="primary" size="small" link @click="applyPricingSuggestion">应用建议价</el-button>
              <el-button size="small" link @click="pricingSuggestion = null">忽略</el-button>
            </template>
          </el-alert>

          <template v-if="supportsTieredPricing(deploymentForm.capability_type)">
            <p class="sub-label">阶梯定价（按输入 token 用量分档）</p>
            <TieredPricingEditor v-model="deploymentForm.pricing.tiers" />
          </template>

          <template v-if="deploymentForm.capability_type === 'image'">
            <p class="sub-label">图片计费（按分辨率）</p>
            <ResolutionPricingEditor
              v-model="deploymentForm.pricing.image_prices"
              mode="image"
              :presets="imagePresets"
            />
          </template>

          <template v-if="deploymentForm.capability_type === 'video'">
            <p class="sub-label">视频计费（按分辨率 × 秒）</p>
            <ResolutionPricingEditor
              v-model="deploymentForm.pricing.video_prices"
              mode="video"
              :presets="videoPresets"
            />
          </template>

          <div class="request-cost-row">
            <span class="sub-label">按次计费（每次请求叠加，¥）</span>
            <el-input-number v-model="deploymentForm.pricing.request_cost" :min="0" :precision="4" :step="0.01" controls-position="right" />
          </div>
        </div>
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

.protocol-hint {
  margin-top: 4px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

.price-pair {
  display: inline-flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
}

.price-pill {
  display: inline-flex;
  align-items: stretch;
  overflow: hidden;
  border-radius: 4px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11px;
  font-weight: 700;
  line-height: 1;
  border: 1px solid transparent;
}

.price-pill-label {
  display: inline-flex;
  align-items: center;
  padding: 3px 6px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.price-pill-value {
  display: inline-flex;
  align-items: center;
  padding: 3px 7px;
  background: #ffffff;
  color: #0f172a;
}

.price-pill-input {
  border-color: #bfdbfe;
}

.price-pill-input .price-pill-label {
  background: #dbeafe;
  color: #1d4ed8;
}

.price-pill-output {
  border-color: #fed7aa;
}

.price-pill-output .price-pill-label {
  background: #ffedd5;
  color: #c2410c;
}

.price-pill-image {
  border-color: #c7d2fe;
}

.price-pill-image .price-pill-label {
  background: #e0e7ff;
  color: #4338ca;
}

.price-pill-video {
  border-color: #fbcfe8;
}

.price-pill-video .price-pill-label {
  background: #fce7f3;
  color: #be185d;
}

.price-empty {
  color: #94a3b8;
  font-size: 12px;
}

.tier-badge {
  margin-left: 2px;
}

.section-card {
  margin-bottom: 16px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 14px 16px;
  background: #fafbfc;
}

.section-title {
  margin: 0 0 4px;
  color: #0f172a;
  font-size: 14px;
  font-weight: 800;
}

.section-sub {
  margin: 0 0 12px;
  color: #64748b;
  font-size: 12px;
}

.sub-label {
  margin: 12px 0 8px;
  color: #475569;
  font-size: 12px;
  font-weight: 700;
}

.request-cost-row {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px dashed #e5e7eb;
}

.request-cost-row .sub-label {
  margin: 0;
}

.suggestion-banner {
  margin-bottom: 14px;
}

.suggestion-banner :deep(.el-alert__title) {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.discovery-loading,
.discovery-empty {
  display: flex;
  min-height: 120px;
  align-items: center;
  justify-content: center;
}

.discovery-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
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
