<script setup>
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Edit, Plus, Refresh, Search } from '@element-plus/icons-vue'
import {
  capabilityOptions,
  createModel,
  createModelRoute,
  deleteModelRoute,
  formatWholeCredits,
  formatTimestamp,
  getModelPrice,
  listModelRoutes,
  listModels,
  listUpstreamDeployments,
  statusOptions,
  updateModel,
  updateModelRoute,
  updateModelRouteStatus,
  updateModelStatus,
  upsertModelPrice
} from '@/api/aiGateway'
import ResolutionPricingEditor from './components/ResolutionPricingEditor.vue'
import { suggestModelConfig } from './modelPricingCatalog'

// ── Loading states ──────────────────────────────────────────────────────────
const loading = shallowRef(false)
const priceLoading = shallowRef(false)
const routeLoading = shallowRef(false)
const submittingRoutes = shallowRef(false)

// ── Dialog visibility ───────────────────────────────────────────────────────
const modelDialogVisible = shallowRef(false)
const priceDialogVisible = shallowRef(false)
const routeDialogVisible = shallowRef(false)

// ── Editing state ───────────────────────────────────────────────────────────
const editingModelId = shallowRef('')
const editingRouteId = shallowRef('')
const modelDialogStep = shallowRef(0) // 0 基础信息 | 1 价格

// ── Data ────────────────────────────────────────────────────────────────────
const models = shallowRef([])
const modelPrice = shallowRef(null)
const modelRoutes = shallowRef([])
const allUpstreamDeployments = shallowRef([])

// ── Selection & filter ──────────────────────────────────────────────────────
const selectedModelId = shallowRef('')
const searchQuery = shallowRef('')
const capabilityFilter = shallowRef('all')

// ── Image / video price presets ─────────────────────────────────────────────
const imagePresets = [
  { label: '1024×1024', resolutions: ['1024x1024'] },
  { label: '1024×1792', resolutions: ['1024x1792'] },
  { label: '1792×1024', resolutions: ['1792x1024'] },
  { label: '512×512',   resolutions: ['512x512'] }
]

const videoPresets = [
  { label: '通用', resolutions: ['480p', '720p', '1080p', '4k'] },
  { label: 'Sora', resolutions: ['720x1280', '1024x1792'] },
  { label: 'Veo',  resolutions: ['720p', '1080p', '4k'] }
]

// ── Forms ───────────────────────────────────────────────────────────────────
const modelForm = reactive({
  model_code: '',
  capability_type: 'chat',
  context_window: null,
  default_max_output_tokens: 4096,
  max_output_tokens: null,
  status: 'active'
})

const priceForm = reactive({
  input_price_per_1m_credits: 0,
  output_price_per_1m_credits: 0,
  image_prices: [],   // [{resolution, price}]
  video_prices: [],   // [{resolution, price}] (price = credits per second)
  audio_tts_price_per_1m_chars_credits: 0,
  audio_stt_price_per_minute_credits: 0
})

const suggestionHint = shallowRef(null)

// ── Route picker ────────────────────────────────────────────────────────────
const pickerSourceFilter = shallowRef('')    // '' | 'endpoint' | 'pool'
const pickerProviderFilter = shallowRef('')
const pickerEndpointFilter = shallowRef('')
const pickerCapabilityFilter = shallowRef('')
const pickerSearch = shallowRef('')
const pickerSelectedIds = shallowRef([])     // Set-like array of deployment ids
const pickerOverrides = reactive({})         // { [deploymentId]: { priority, weight, supports_stream } }
const pickerDefaults = reactive({
  priority: 100,
  weight: 100,
  supports_stream: true,
  status: 'active'
})

// ── Computed ────────────────────────────────────────────────────────────────
const isEditingModel = computed(() => Boolean(editingModelId.value))
const isEditingRoute = computed(() => Boolean(editingRouteId.value))

const selectedModel = computed(() =>
  models.value.find((m) => m.id === selectedModelId.value)
)

const selectedCapabilityType = computed(() => selectedModel.value?.capability_type || '')

const filteredModels = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return models.value.filter((m) => {
    if (capabilityFilter.value !== 'all' && m.capability_type !== capabilityFilter.value) return false
    if (!q) return true
    return m.model_code.toLowerCase().includes(q)
  })
})

const capabilityFilterOptions = computed(() => [
  { label: '全部', value: 'all' },
  ...capabilityOptions
])

const isChatLike = (cap) => ['chat', 'embedding', 'rerank'].includes(cap)
const isImage = (cap) => cap === 'image'
const isVideo = (cap) => cap === 'video'
const isAudioTTS = (cap) => cap === 'audio_tts'
const isAudioSTT = (cap) => cap === 'audio_stt'

const showTokenPrice = computed(() => isChatLike(selectedCapabilityType.value))
const showOutputPrice = computed(() => selectedCapabilityType.value === 'chat')
const showImagePrice = computed(() => isImage(selectedCapabilityType.value))
const showVideoPrice = computed(() => isVideo(selectedCapabilityType.value))
const showAudioTTSPrice = computed(() => isAudioTTS(selectedCapabilityType.value))
const showAudioSTTPrice = computed(() => isAudioSTT(selectedCapabilityType.value))

const formCap = computed(() => modelForm.capability_type)
const showChatFieldsInForm = computed(() => formCap.value === 'chat')

const activeRouteCount = computed(() => modelRoutes.value.filter((r) => r.status === 'active').length)

// ── Utilities ───────────────────────────────────────────────────────────────
const statusTagType = (status) => ({ active: 'success', inactive: 'warning', disabled: 'danger' }[status] || 'info')

const capabilityLabel = (value) => capabilityOptions.find((o) => o.value === value)?.label || value || '-'

const capabilityDotClass = (value) => {
  const map = { chat: 'dot-chat', image: 'dot-image', video: 'dot-video', embedding: 'dot-embedding', audio_tts: 'dot-audio', audio_stt: 'dot-audio', rerank: 'dot-rerank' }
  return map[value] || 'dot-default'
}

// ── Reset forms ─────────────────────────────────────────────────────────────
const resetModelForm = () => {
  editingModelId.value = ''
  modelDialogStep.value = 0
  suggestionHint.value = null
  Object.assign(modelForm, {
    model_code: '',
    capability_type: 'chat',
    context_window: null,
    default_max_output_tokens: 4096,
    max_output_tokens: null,
    status: 'active'
  })
  resetPriceFormDefaults()
}

const resetPriceFormDefaults = () => {
  Object.assign(priceForm, {
    input_price_per_1m_credits: 0,
    output_price_per_1m_credits: 0,
    image_prices: [],
    video_prices: [],
    audio_tts_price_per_1m_chars_credits: 0,
    audio_stt_price_per_minute_credits: 0
  })
}

const seedPriceFormFromModel = () => {
  const p = modelPrice.value
  Object.assign(priceForm, {
    input_price_per_1m_credits: p?.input_price_per_1m_credits ?? 0,
    output_price_per_1m_credits: p?.output_price_per_1m_credits ?? 0,
    image_prices: Array.isArray(p?.image_prices) ? p.image_prices : [],
    video_prices: Array.isArray(p?.video_prices) ? p.video_prices : [],
    audio_tts_price_per_1m_chars_credits: p?.audio_tts_price_per_1m_chars_credits ?? 0,
    audio_stt_price_per_minute_credits: p?.audio_stt_price_per_minute_credits ?? 0
  })
}

const applyModelForm = (row) => {
  editingModelId.value = row.id
  modelDialogStep.value = 0
  suggestionHint.value = null
  Object.assign(modelForm, {
    model_code: row.model_code,
    capability_type: row.capability_type,
    context_window: row.context_window ?? null,
    default_max_output_tokens: row.default_max_output_tokens,
    max_output_tokens: row.max_output_tokens ?? null,
    status: row.status
  })
}

// 编辑时切换 capability，priceForm 不主动重置，避免误清。新增时切 capability 不影响 step1。
const onModelCapabilityChange = () => {
  // 切到非 chat 时清掉 context_window / max_output_tokens 字段值（保留 default_max_output_tokens 让后端有默认）
  if (modelForm.capability_type !== 'chat') {
    modelForm.context_window = null
    modelForm.max_output_tokens = null
  }
  suggestionHint.value = null
}

const onModelCodeBlur = () => {
  const code = (modelForm.model_code || '').trim()
  if (!code) return
  if (modelForm.capability_type !== 'chat') return
  const cfg = suggestModelConfig(code)
  if (!cfg) {
    suggestionHint.value = null
    return
  }
  // 仅在字段为空/默认值时填入，避免覆盖用户输入
  if (modelForm.context_window == null) modelForm.context_window = cfg.context_window
  if (modelForm.default_max_output_tokens == null || modelForm.default_max_output_tokens === 4096) {
    modelForm.default_max_output_tokens = cfg.default_max_output_tokens
  }
  if (modelForm.max_output_tokens == null) modelForm.max_output_tokens = cfg.max_output_tokens
  suggestionHint.value = cfg
}

// ── Fetch ───────────────────────────────────────────────────────────────────
const fetchModels = async () => {
  loading.value = true
  try {
    models.value = await listModels()
    if (!selectedModelId.value && models.value.length > 0) {
      selectedModelId.value = models.value[0].id
      await fetchSelectedModelDetail()
    } else if (selectedModelId.value && !models.value.some((m) => m.id === selectedModelId.value)) {
      selectedModelId.value = models.value[0]?.id || ''
      if (selectedModelId.value) await fetchSelectedModelDetail()
    }
  } finally {
    loading.value = false
  }
}

const fetchSelectedModelDetail = async () => {
  await Promise.all([fetchModelPrice(), fetchModelRoutes()])
}

const fetchModelPrice = async () => {
  if (!selectedModelId.value) { modelPrice.value = null; return }
  priceLoading.value = true
  try {
    modelPrice.value = await getModelPrice(selectedModelId.value)
  } catch {
    modelPrice.value = null
  } finally {
    priceLoading.value = false
  }
}

const fetchModelRoutes = async () => {
  if (!selectedModelId.value) { modelRoutes.value = []; return }
  routeLoading.value = true
  try {
    modelRoutes.value = await listModelRoutes(selectedModelId.value)
  } finally {
    routeLoading.value = false
  }
}

const fetchAllUpstreamDeployments = async () => {
  try { allUpstreamDeployments.value = await listUpstreamDeployments() }
  catch { allUpstreamDeployments.value = [] }
}

// ── Selection ───────────────────────────────────────────────────────────────
const selectModel = async (model) => {
  selectedModelId.value = model.id
  await fetchSelectedModelDetail()
}

// ── Model dialog handlers ───────────────────────────────────────────────────
const openModelDialog = () => { resetModelForm(); modelDialogVisible.value = true }
const openModelEditDialog = (row) => { applyModelForm(row); modelDialogVisible.value = true }

const modelStepNext = () => {
  if (!modelForm.model_code.trim()) {
    ElMessage.error('请填写模型名称')
    return
  }
  modelDialogStep.value = 1
}

const submitModel = async () => {
  const payload = {
    model_code: modelForm.model_code.trim(),
    capability_type: modelForm.capability_type,
    context_window: modelForm.capability_type === 'chat' ? (modelForm.context_window || undefined) : undefined,
    default_max_output_tokens: modelForm.default_max_output_tokens,
    max_output_tokens: modelForm.capability_type === 'chat' ? (modelForm.max_output_tokens || undefined) : undefined,
    status: modelForm.status
  }

  let modelId = editingModelId.value
  if (modelId) {
    await updateModel(modelId, payload)
    ElMessage.success('模型已保存')
  } else {
    const created = await createModel(payload)
    modelId = created?.id
    if (!modelId) {
      ElMessage.error('创建模型失败：未返回 id')
      return
    }
    // 新建模型时一并写入价格
    await upsertModelPrice(modelId, buildPricePayload(modelForm.capability_type))
    ElMessage.success('模型与价格已创建')
  }

  modelDialogVisible.value = false
  selectedModelId.value = modelId
  await fetchModels()
  await fetchSelectedModelDetail()
}

const buildPricePayload = (cap) => ({
  input_price_per_1m_credits: isChatLike(cap) ? (priceForm.input_price_per_1m_credits || 0) : 0,
  output_price_per_1m_credits: cap === 'chat' ? (priceForm.output_price_per_1m_credits || 0) : 0,
  image_prices: isImage(cap) ? priceForm.image_prices : [],
  video_prices: isVideo(cap) ? priceForm.video_prices : [],
  audio_tts_price_per_1m_chars_credits: isAudioTTS(cap) ? (priceForm.audio_tts_price_per_1m_chars_credits || 0) : 0,
  audio_stt_price_per_minute_credits: isAudioSTT(cap) ? (priceForm.audio_stt_price_per_minute_credits || 0) : 0
})

// ── Price dialog (单独编辑现有模型价格) ─────────────────────────────────────
const openPriceDialog = () => {
  if (!selectedModelId.value) { ElMessage.warning('请先选择模型'); return }
  seedPriceFormFromModel()
  priceDialogVisible.value = true
}

const submitModelPrice = async () => {
  await upsertModelPrice(selectedModelId.value, buildPricePayload(selectedCapabilityType.value))
  ElMessage.success('销售价已保存')
  priceDialogVisible.value = false
  await fetchModelPrice()
}

// ── Route dialog ────────────────────────────────────────────────────────────
const resetRoutePicker = () => {
  pickerSourceFilter.value = ''
  pickerProviderFilter.value = ''
  pickerEndpointFilter.value = ''
  pickerCapabilityFilter.value = selectedCapabilityType.value || ''
  pickerSearch.value = ''
  pickerSelectedIds.value = []
  for (const key of Object.keys(pickerOverrides)) delete pickerOverrides[key]
  Object.assign(pickerDefaults, { priority: 100, weight: 100, supports_stream: true, status: 'active' })
}

const openRouteDialog = async () => {
  if (!selectedModelId.value) { ElMessage.warning('请先选择模型'); return }
  editingRouteId.value = ''
  await fetchAllUpstreamDeployments()
  resetRoutePicker()
  routeDialogVisible.value = true
}

const openRouteEditDialog = async (row) => {
  editingRouteId.value = row.id
  await fetchAllUpstreamDeployments()
  resetRoutePicker()
  pickerSelectedIds.value = [row.upstream_deployment_id]
  Object.assign(pickerDefaults, {
    priority: row.priority,
    weight: row.weight,
    supports_stream: row.supports_stream,
    status: row.status
  })
  routeDialogVisible.value = true
}

// Filter options derived from deployments
const providerOptions = computed(() => {
  const m = new Map()
  for (const d of allUpstreamDeployments.value) m.set(d.provider_id, d.provider_name || d.provider_code)
  return [...m.entries()].map(([id, name]) => ({ id, name }))
})

const endpointOptions = computed(() => {
  const m = new Map()
  for (const d of allUpstreamDeployments.value) {
    if (pickerProviderFilter.value && d.provider_id !== pickerProviderFilter.value) continue
    m.set(d.endpoint_id, d.endpoint_name)
  }
  return [...m.entries()].map(([id, name]) => ({ id, name }))
})

// Routes already attached to current model — to disable duplicates
const existingRouteDeploymentIds = computed(() => {
  const ids = new Set()
  for (const r of modelRoutes.value) {
    if (r.upstream_deployment_id) ids.add(r.upstream_deployment_id)
  }
  // 编辑模式下放行当前编辑的部署
  if (editingRouteId.value) {
    const current = modelRoutes.value.find((r) => r.id === editingRouteId.value)
    if (current?.upstream_deployment_id) ids.delete(current.upstream_deployment_id)
  }
  return ids
})

const filteredDeployments = computed(() => {
  const q = pickerSearch.value.trim().toLowerCase()
  return allUpstreamDeployments.value.filter((d) => {
    if (d.status !== 'active') return false
    if (pickerSourceFilter.value && d.credential_source !== pickerSourceFilter.value) return false
    if (pickerProviderFilter.value && d.provider_id !== pickerProviderFilter.value) return false
    if (pickerEndpointFilter.value && d.endpoint_id !== pickerEndpointFilter.value) return false
    if (pickerCapabilityFilter.value && d.capability_type !== pickerCapabilityFilter.value) return false
    if (!q) return true
    return (
      d.upstream_model.toLowerCase().includes(q) ||
      (d.endpoint_name || '').toLowerCase().includes(q) ||
      (d.pool_name || '').toLowerCase().includes(q) ||
      (d.provider_name || '').toLowerCase().includes(q)
    )
  })
})

const allFilteredSelected = computed(() => {
  const list = filteredDeployments.value.filter((d) => !existingRouteDeploymentIds.value.has(d.id))
  if (list.length === 0) return false
  return list.every((d) => pickerSelectedIds.value.includes(d.id))
})

const togglePickerRow = (id) => {
  const set = new Set(pickerSelectedIds.value)
  if (set.has(id)) set.delete(id)
  else set.add(id)
  pickerSelectedIds.value = [...set]
}

const togglePickerAll = (val) => {
  if (val) {
    const ids = filteredDeployments.value
      .filter((d) => !existingRouteDeploymentIds.value.has(d.id))
      .map((d) => d.id)
    pickerSelectedIds.value = [...new Set([...pickerSelectedIds.value, ...ids])]
  } else {
    const exclude = new Set(filteredDeployments.value.map((d) => d.id))
    pickerSelectedIds.value = pickerSelectedIds.value.filter((id) => !exclude.has(id))
  }
}

const isOverridden = (id) => Boolean(pickerOverrides[id])

const toggleOverride = (id) => {
  if (pickerOverrides[id]) {
    delete pickerOverrides[id]
  } else {
    pickerOverrides[id] = {
      priority: pickerDefaults.priority,
      weight: pickerDefaults.weight,
      supports_stream: pickerDefaults.supports_stream
    }
  }
}

const buildRoutePayload = (deploymentId) => {
  const o = pickerOverrides[deploymentId]
  return {
    upstream_deployment_id: deploymentId,
    priority: o?.priority ?? pickerDefaults.priority,
    weight: o?.weight ?? pickerDefaults.weight,
    supports_stream: o?.supports_stream ?? pickerDefaults.supports_stream,
    status: pickerDefaults.status
  }
}

const submitRoutes = async () => {
  if (editingRouteId.value) {
    if (pickerSelectedIds.value.length !== 1) {
      ElMessage.error('编辑路由时只能选择一个上游部署')
      return
    }
    submittingRoutes.value = true
    try {
      const payload = buildRoutePayload(pickerSelectedIds.value[0])
      await updateModelRoute(selectedModelId.value, editingRouteId.value, payload)
      ElMessage.success('路由已保存')
      routeDialogVisible.value = false
      await fetchModelRoutes()
    } finally {
      submittingRoutes.value = false
    }
    return
  }

  if (pickerSelectedIds.value.length === 0) {
    ElMessage.warning('请至少选择一个上游部署')
    return
  }
  submittingRoutes.value = true
  let ok = 0
  let fail = 0
  try {
    for (const id of pickerSelectedIds.value) {
      try {
        await createModelRoute(selectedModelId.value, buildRoutePayload(id))
        ok++
      } catch {
        fail++
      }
    }
    if (fail === 0) ElMessage.success(`已创建 ${ok} 条路由`)
    else ElMessage.warning(`成功 ${ok} 条，失败 ${fail} 条`)
    routeDialogVisible.value = false
    await fetchModelRoutes()
  } finally {
    submittingRoutes.value = false
  }
}

// ── Toggle & delete ────────────────────────────────────────────────────────
const toggleModel = async (row) => {
  await updateModelStatus(row.id, row.status === 'active' ? 'disabled' : 'active')
  ElMessage.success('模型状态已更新')
  await fetchModels()
}

const toggleModelRoute = async (row) => {
  await updateModelRouteStatus(selectedModelId.value, row.id, row.status === 'active' ? 'disabled' : 'active')
  ElMessage.success('路由状态已更新')
  await fetchModelRoutes()
}

const deleteRoute = async (row) => {
  try {
    await ElMessageBox.confirm('确定删除该路由吗？删除后不可恢复。', '删除确认', {
      confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning'
    })
    await deleteModelRoute(selectedModelId.value, row.id)
    ElMessage.success('路由已删除')
    await fetchModelRoutes()
  } catch { /* cancelled */ }
}

// ── Lifecycle ───────────────────────────────────────────────────────────────
onMounted(() => { fetchModels() })
</script>

<template>
  <div class="model-workbench">
    <!-- ── Left rail ── -->
    <aside class="model-rail">
      <div class="rail-head">
        <div>
          <p class="eyebrow">Public Models</p>
          <h2>公开模型</h2>
        </div>
        <el-button type="primary" :icon="Plus" @click="openModelDialog">新增</el-button>
      </div>

      <div class="rail-filter">
        <el-input v-model="searchQuery" placeholder="搜索模型名称" clearable size="small" />
        <div class="cap-tabs">
          <span
            v-for="opt in capabilityFilterOptions"
            :key="opt.value"
            class="cap-tab"
            :class="{ active: capabilityFilter === opt.value }"
            @click="capabilityFilter = opt.value"
          >{{ opt.label }}</span>
        </div>
      </div>

      <div v-loading="loading" class="model-list">
        <div v-if="filteredModels.length === 0" class="empty-list">暂无模型</div>
        <div
          v-for="model in filteredModels"
          :key="model.id"
          class="model-item"
          :class="{ active: model.id === selectedModelId }"
          @click="selectModel(model)"
        >
          <span :class="['dot', capabilityDotClass(model.capability_type)]" :title="capabilityLabel(model.capability_type)" />
          <div class="model-item-text">
            <strong>{{ model.model_code }}</strong>
            <small>{{ capabilityLabel(model.capability_type) }}</small>
          </div>
          <div class="model-item-actions" @click.stop>
            <el-button link type="primary" size="small" :icon="Edit" @click="openModelEditDialog(model)" />
            <el-button
              link
              :type="model.status === 'active' ? 'warning' : 'success'"
              size="small"
              @click="toggleModel(model)"
            >{{ model.status === 'active' ? '禁' : '启' }}</el-button>
          </div>
        </div>
      </div>
    </aside>

    <!-- ── Right workspace ── -->
    <main v-if="selectedModel" class="model-workspace">
      <!-- Hero -->
      <section class="workspace-hero">
        <div class="hero-main">
          <p class="eyebrow">Current Model</p>
          <div class="hero-title-row">
            <h2>{{ selectedModel.model_code }}</h2>
            <el-tag :type="statusTagType(selectedModel.status)" effect="dark">{{ selectedModel.status }}</el-tag>
          </div>
          <p>{{ capabilityLabel(selectedModel.capability_type) }}</p>
        </div>
        <div class="hero-actions">
          <el-button :icon="Refresh" @click="fetchSelectedModelDetail" />
          <el-button :icon="Edit" @click="openModelEditDialog(selectedModel)">编辑模型</el-button>
          <el-button
            plain
            :type="selectedModel.status === 'active' ? 'warning' : 'success'"
            @click="toggleModel(selectedModel)"
          >{{ selectedModel.status === 'active' ? '禁用' : '启用' }}</el-button>
        </div>

        <div class="metric-grid">
          <div class="metric-cell">
            <span>路由数</span>
            <strong>{{ modelRoutes.length }}</strong>
            <small>{{ activeRouteCount }} active</small>
          </div>
          <div class="metric-cell">
            <span>能力类型</span>
            <strong>{{ capabilityLabel(selectedModel.capability_type) }}</strong>
          </div>
          <div v-if="selectedModel.capability_type === 'chat'" class="metric-cell">
            <span>上下文窗口</span>
            <strong>{{ selectedModel.context_window ? selectedModel.context_window.toLocaleString() : '-' }}</strong>
            <small>tokens</small>
          </div>
          <div v-if="selectedModel.capability_type === 'chat'" class="metric-cell">
            <span>默认输出</span>
            <strong>{{ selectedModel.default_max_output_tokens?.toLocaleString() }}</strong>
            <small>tokens</small>
          </div>
        </div>
      </section>

      <!-- Routes -->
      <section class="panel">
        <div class="section-head">
          <div>
            <h3>模型路由</h3>
            <p>配置上游部署映射和优先级权重</p>
          </div>
          <el-button type="primary" :icon="Plus" @click="openRouteDialog">新增路由</el-button>
        </div>
        <el-table v-loading="routeLoading" :data="modelRoutes" border stripe class="w-full">
          <el-table-column label="上游" min-width="240">
            <template #default="{ row }">
              <div class="route-deployment">
                <div class="route-endpoint-row">
                  <el-tag v-if="row.credential_source === 'pool'" size="small" type="warning" effect="plain" class="oauth-badge">OAuth</el-tag>
                  <span class="route-endpoint">{{ row.pool_name || row.endpoint_name || '-' }}</span>
                </div>
                <span class="route-model">{{ row.upstream_model || '-' }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="priority" label="优先级" width="90" align="right">
            <template #default="{ row }">
              <span class="priority-badge">{{ row.priority }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="weight" label="权重" width="80" align="right" />
          <el-table-column label="流式" width="75" align="center">
            <template #default="{ row }">
              <el-tag :type="row.supports_stream ? 'success' : 'info'" size="small">
                {{ row.supports_stream ? '是' : '否' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="230" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" :icon="Edit" @click="openRouteEditDialog(row)">编辑</el-button>
              <el-button link :type="row.status === 'active' ? 'warning' : 'success'" @click="toggleModelRoute(row)">
                {{ row.status === 'active' ? '禁用' : '启用' }}
              </el-button>
              <el-button link type="danger" :icon="Delete" @click="deleteRoute(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </section>

      <!-- Price -->
      <section class="panel">
        <div class="section-head">
          <div>
            <h3>销售价配置</h3>
            <p>所有价格为积分单位</p>
          </div>
          <el-button type="primary" :icon="Edit" @click="openPriceDialog">
            {{ modelPrice ? '修改价格' : '设置价格' }}
          </el-button>
        </div>

        <div v-loading="priceLoading">
          <div v-if="modelPrice" class="price-display">
            <template v-if="showTokenPrice">
              <div class="price-item">
                <span>输入 / 1M tokens</span>
                <strong>{{ formatWholeCredits(modelPrice.input_price_per_1m_credits) }} 积分</strong>
              </div>
              <div v-if="showOutputPrice" class="price-item">
                <span>输出 / 1M tokens</span>
                <strong>{{ formatWholeCredits(modelPrice.output_price_per_1m_credits) }} 积分</strong>
              </div>
            </template>
            <template v-if="showImagePrice">
              <div v-for="(entry, idx) in (modelPrice.image_prices || [])" :key="idx" class="price-item">
                <span>{{ entry.resolution }}</span>
                <strong>{{ formatWholeCredits(entry.price_credits) }} 积分 / 张</strong>
              </div>
              <div v-if="!(modelPrice.image_prices?.length)" class="price-empty-hint">
                尚未配置尺寸定价
              </div>
            </template>
            <template v-if="showVideoPrice">
              <div v-for="(entry, idx) in (modelPrice.video_prices || [])" :key="idx" class="price-item">
                <span>{{ entry.resolution }}</span>
                <strong>{{ formatWholeCredits(entry.price_credits) }} 积分 / 秒</strong>
              </div>
              <div v-if="!(modelPrice.video_prices?.length)" class="price-empty-hint">
                尚未配置分辨率定价
              </div>
            </template>
            <div v-if="showAudioTTSPrice" class="price-item">
              <span>TTS / 1M 字符</span>
              <strong>{{ formatWholeCredits(modelPrice.audio_tts_price_per_1m_chars_credits) }} 积分</strong>
            </div>
            <div v-if="showAudioSTTPrice" class="price-item">
              <span>STT / 分钟</span>
              <strong>{{ formatWholeCredits(modelPrice.audio_stt_price_per_minute_credits) }} 积分</strong>
            </div>
            <div class="price-updated">上次更新：{{ formatTimestamp(modelPrice.updated_at) }}</div>
          </div>

          <div v-else class="price-unset">
            <p>该模型尚未设置销售价，运行时请求将被拒绝。</p>
          </div>
        </div>
      </section>
    </main>

    <main v-else class="empty-workspace">
      <p class="eyebrow">No Model</p>
      <h2>先创建一个公开模型</h2>
      <p>公开模型是用户调用时的统一入口。创建后即可配置路由映射和销售价格。</p>
      <el-button type="primary" :icon="Plus" @click="openModelDialog">新增模型</el-button>
    </main>

    <!-- ── 模型对话框（分步） ── -->
    <el-dialog
      v-model="modelDialogVisible"
      :title="isEditingModel ? '编辑公开模型' : '新增公开模型'"
      width="680px"
      append-to-body
      :close-on-click-modal="false"
    >
      <el-steps v-if="!isEditingModel" :active="modelDialogStep" simple class="model-stepper">
        <el-step title="基础信息" />
        <el-step title="销售价格" />
      </el-steps>

      <!-- Step 1: 基础信息 -->
      <el-form v-if="modelDialogStep === 0 || isEditingModel" :model="modelForm" label-position="top">
        <el-form-item label="模型名称（调用 model 字段）" required>
          <el-input
            v-model="modelForm.model_code"
            placeholder="例如 gpt-4o、claude-sonnet-4-5"
            @blur="onModelCodeBlur"
          />
        </el-form-item>

        <el-alert
          v-if="suggestionHint && !isEditingModel"
          class="suggestion-banner"
          type="success"
          show-icon
          :closable="false"
        >
          <template #title>
            <span>已识别为 <strong>{{ suggestionHint.label }}</strong>，自动填充上下文窗口/输出 token 配置（你可以修改）</span>
          </template>
        </el-alert>

        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="能力类型">
            <el-select v-model="modelForm.capability_type" class="w-full" @change="onModelCapabilityChange">
              <el-option v-for="item in capabilityOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="状态">
            <el-select v-model="modelForm.status" class="w-full">
              <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
        </div>
        <div v-if="showChatFieldsInForm" class="grid grid-cols-3 gap-4">
          <el-form-item label="上下文窗口">
            <el-input-number v-model="modelForm.context_window" :min="0" :precision="0" class="w-full" />
          </el-form-item>
          <el-form-item label="默认输出 Token">
            <el-input-number v-model="modelForm.default_max_output_tokens" :min="1" class="w-full" />
          </el-form-item>
          <el-form-item label="最大输出 Token">
            <el-input-number v-model="modelForm.max_output_tokens" :min="0" :precision="0" class="w-full" />
          </el-form-item>
        </div>
      </el-form>

      <!-- Step 2: 销售价格 -->
      <el-form v-if="!isEditingModel && modelDialogStep === 1" :model="priceForm" label-position="top">
        <p class="form-section-hint">当前能力：<strong>{{ capabilityLabel(modelForm.capability_type) }}</strong>，下方仅显示该能力相关的价格字段（积分）。</p>

        <template v-if="isChatLike(modelForm.capability_type)">
          <div class="grid grid-cols-2 gap-4">
            <el-form-item label="输入 / 1M tokens（积分）">
              <el-input-number v-model="priceForm.input_price_per_1m_credits" :min="0" :precision="0" :step="1" class="w-full" />
            </el-form-item>
            <el-form-item v-if="modelForm.capability_type === 'chat'" label="输出 / 1M tokens（积分）">
              <el-input-number v-model="priceForm.output_price_per_1m_credits" :min="0" :precision="0" :step="1" class="w-full" />
            </el-form-item>
          </div>
        </template>

        <template v-if="isImage(modelForm.capability_type)">
          <el-form-item label="尺寸定价（积分 / 张）">
            <ResolutionPricingEditor v-model="priceForm.image_prices" mode="image" :presets="imagePresets" />
            <small class="form-hint">不在列表中的尺寸将拒绝请求</small>
          </el-form-item>
        </template>

        <template v-if="isVideo(modelForm.capability_type)">
          <el-form-item label="分辨率定价（积分 / 秒）">
            <ResolutionPricingEditor v-model="priceForm.video_prices" mode="video" :presets="videoPresets" />
            <small class="form-hint">不在列表中的分辨率将拒绝请求</small>
          </el-form-item>
        </template>

        <el-form-item v-if="isAudioTTS(modelForm.capability_type)" label="TTS / 1M 字符（积分）">
          <el-input-number v-model="priceForm.audio_tts_price_per_1m_chars_credits" :min="0" :precision="0" :step="1" class="w-full" />
        </el-form-item>

        <el-form-item v-if="isAudioSTT(modelForm.capability_type)" label="STT / 分钟（积分）">
          <el-input-number v-model="priceForm.audio_stt_price_per_minute_credits" :min="0" :precision="0" :step="1" class="w-full" />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="modelDialogVisible = false">取消</el-button>
        <template v-if="isEditingModel">
          <el-button type="primary" @click="submitModel">保存</el-button>
        </template>
        <template v-else>
          <el-button v-if="modelDialogStep === 1" @click="modelDialogStep = 0">上一步</el-button>
          <el-button v-if="modelDialogStep === 0" type="primary" @click="modelStepNext">下一步</el-button>
          <el-button v-else type="primary" @click="submitModel">创建</el-button>
        </template>
      </template>
    </el-dialog>

    <!-- ── 销售价对话框（编辑现有模型） ── -->
    <el-dialog v-model="priceDialogVisible" title="修改销售价" width="600px" append-to-body>
      <el-form :model="priceForm" label-position="top">
        <template v-if="showTokenPrice">
          <div class="grid grid-cols-2 gap-4">
            <el-form-item label="输入 / 1M tokens（积分）">
              <el-input-number v-model="priceForm.input_price_per_1m_credits" :min="0" :precision="0" :step="1" class="w-full" />
            </el-form-item>
            <el-form-item v-if="showOutputPrice" label="输出 / 1M tokens（积分）">
              <el-input-number v-model="priceForm.output_price_per_1m_credits" :min="0" :precision="0" :step="1" class="w-full" />
            </el-form-item>
          </div>
        </template>

        <template v-if="showImagePrice">
          <el-form-item label="尺寸定价（积分 / 张）">
            <ResolutionPricingEditor v-model="priceForm.image_prices" mode="image" :presets="imagePresets" />
            <small class="form-hint">不在列表中的尺寸将拒绝请求</small>
          </el-form-item>
        </template>

        <template v-if="showVideoPrice">
          <el-form-item label="分辨率定价（积分 / 秒）">
            <ResolutionPricingEditor v-model="priceForm.video_prices" mode="video" :presets="videoPresets" />
            <small class="form-hint">不在列表中的分辨率将拒绝请求</small>
          </el-form-item>
        </template>

        <el-form-item v-if="showAudioTTSPrice" label="TTS / 1M 字符（积分）">
          <el-input-number v-model="priceForm.audio_tts_price_per_1m_chars_credits" :min="0" :precision="0" :step="1" class="w-full" />
        </el-form-item>

        <el-form-item v-if="showAudioSTTPrice" label="STT / 分钟（积分）">
          <el-input-number v-model="priceForm.audio_stt_price_per_minute_credits" :min="0" :precision="0" :step="1" class="w-full" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="priceDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitModelPrice">保存价格</el-button>
      </template>
    </el-dialog>

    <!-- ── 路由对话框 ── -->
    <el-dialog
      v-model="routeDialogVisible"
      :title="isEditingRoute ? '编辑模型路由' : '新增模型路由'"
      width="960px"
      append-to-body
      :close-on-click-modal="false"
    >
      <div class="picker-filters">
        <el-select v-model="pickerSourceFilter" placeholder="来源类型" clearable class="picker-filter">
          <el-option label="API Key" value="endpoint" />
          <el-option label="OAuth 池" value="pool" />
        </el-select>
        <el-select v-model="pickerProviderFilter" placeholder="厂商" clearable class="picker-filter">
          <el-option v-for="p in providerOptions" :key="p.id" :label="p.name" :value="p.id" />
        </el-select>
        <el-select v-model="pickerEndpointFilter" placeholder="账户" clearable class="picker-filter">
          <el-option v-for="e in endpointOptions" :key="e.id" :label="e.name" :value="e.id" />
        </el-select>
        <el-select v-model="pickerCapabilityFilter" placeholder="能力类型" clearable class="picker-filter">
          <el-option v-for="c in capabilityOptions" :key="c.value" :label="c.label" :value="c.value" />
        </el-select>
        <el-input v-model="pickerSearch" :prefix-icon="Search" placeholder="搜索 model / 账户 / 池" clearable class="picker-search" />
      </div>

      <div class="picker-table-wrap">
        <div class="picker-row picker-row-head">
          <span class="cell-check">
            <el-checkbox :model-value="allFilteredSelected" @change="togglePickerAll" />
          </span>
          <span class="cell-endpoint">来源 · 上游模型</span>
          <span class="cell-cap">能力</span>
          <span class="cell-protocol">协议</span>
          <span class="cell-actions">行内覆盖</span>
        </div>
        <div v-if="filteredDeployments.length === 0" class="picker-empty">无匹配的上游部署</div>
        <template v-else>
          <div
            v-for="d in filteredDeployments"
            :key="d.id"
            class="picker-row"
            :class="{ 'is-disabled': existingRouteDeploymentIds.has(d.id), 'is-selected': pickerSelectedIds.includes(d.id) }"
          >
            <span class="cell-check">
              <el-checkbox
                :model-value="pickerSelectedIds.includes(d.id)"
                :disabled="existingRouteDeploymentIds.has(d.id)"
                @change="togglePickerRow(d.id)"
              />
            </span>
            <span class="cell-endpoint">
              <el-tag v-if="d.credential_source === 'pool'" size="small" type="warning" effect="plain" class="oauth-badge">OAuth</el-tag>
              <span class="endpoint-name">{{ d.pool_name || d.endpoint_name || '-' }}</span>
              <span class="endpoint-sep">·</span>
              <span class="upstream-model">{{ d.upstream_model }}</span>
              <small v-if="existingRouteDeploymentIds.has(d.id)" class="already-tag">已配置</small>
            </span>
            <span class="cell-cap">
              <el-tag size="small">{{ capabilityLabel(d.capability_type) }}</el-tag>
            </span>
            <span class="cell-protocol">{{ d.upstream_protocol }}</span>
            <span class="cell-actions">
              <el-button
                link
                size="small"
                :type="isOverridden(d.id) ? 'warning' : 'primary'"
                :disabled="!pickerSelectedIds.includes(d.id)"
                @click="toggleOverride(d.id)"
              >{{ isOverridden(d.id) ? '收起' : '自定义' }}</el-button>
            </span>
            <div v-if="isOverridden(d.id) && pickerSelectedIds.includes(d.id)" class="row-override">
              <span class="ov-field">
                优先级
                <el-input-number v-model="pickerOverrides[d.id].priority" :min="0" :precision="0" size="small" controls-position="right" />
              </span>
              <span class="ov-field">
                权重
                <el-input-number v-model="pickerOverrides[d.id].weight" :min="0" :precision="0" size="small" controls-position="right" />
              </span>
              <span class="ov-field">
                流式
                <el-switch v-model="pickerOverrides[d.id].supports_stream" />
              </span>
            </div>
          </div>
        </template>
      </div>

      <div class="picker-summary">
        <span class="summary-count">已选 <strong>{{ pickerSelectedIds.length }}</strong> 条</span>
        <span class="summary-sep">·</span>
        <span>默认</span>
        <span class="ov-field">
          优先级
          <el-input-number v-model="pickerDefaults.priority" :min="0" :precision="0" size="small" controls-position="right" />
        </span>
        <span class="ov-field">
          权重
          <el-input-number v-model="pickerDefaults.weight" :min="0" :precision="0" size="small" controls-position="right" />
        </span>
        <span class="ov-field">
          流式
          <el-switch v-model="pickerDefaults.supports_stream" />
        </span>
        <span class="ov-field">
          状态
          <el-select v-model="pickerDefaults.status" size="small" style="width: 100px">
            <el-option v-for="s in statusOptions" :key="s.value" :label="s.label" :value="s.value" />
          </el-select>
        </span>
      </div>

      <template #footer>
        <el-button @click="routeDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submittingRoutes" @click="submitRoutes">
          {{ isEditingRoute ? '保存' : `创建 ${pickerSelectedIds.length} 条路由` }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.model-workbench {
  display: grid;
  grid-template-columns: 280px minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}

.model-rail {
  position: sticky;
  top: 16px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fff;
  padding: 16px;
}

.rail-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.rail-head h2 {
  margin: 0;
  color: #111827;
  font-size: 20px;
  font-weight: 900;
}

.eyebrow {
  margin: 0 0 4px;
  color: #64748b;
  font-size: 11px;
  font-weight: 800;
  text-transform: uppercase;
}

.rail-filter {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.cap-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.cap-tab {
  padding: 2px 8px;
  border-radius: 4px;
  border: 1px solid #e5e7eb;
  font-size: 11px;
  font-weight: 700;
  color: #64748b;
  cursor: pointer;
  transition: all 0.14s;
  white-space: nowrap;
}

.cap-tab.active {
  border-color: #2563eb;
  background: #eff6ff;
  color: #2563eb;
}

.model-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: calc(100vh - 320px);
  overflow-y: auto;
  margin-top: 10px;
  padding-right: 2px;
}

.model-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 8px;
  border-radius: 6px;
  cursor: pointer;
  transition: background 0.13s;
}

.model-item:hover { background: #f1f5f9; }
.model-item.active { background: #eff6ff; }

.dot {
  flex-shrink: 0;
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
.dot-chat      { background: #22c55e; }
.dot-image     { background: #a855f7; }
.dot-video     { background: #f97316; }
.dot-embedding { background: #3b82f6; }
.dot-audio     { background: #06b6d4; }
.dot-rerank    { background: #f59e0b; }
.dot-default   { background: #94a3b8; }

.model-item-text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.model-item-text strong {
  font-size: 13px;
  font-weight: 700;
  color: #111827;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

.model-item-text small {
  font-size: 11px;
  color: #94a3b8;
  font-weight: 600;
}

.model-item-actions {
  display: flex;
  gap: 2px;
  flex-shrink: 0;
}

.empty-list {
  display: flex;
  min-height: 120px;
  align-items: center;
  justify-content: center;
  border: 1px dashed #cbd5e1;
  border-radius: 8px;
  color: #94a3b8;
  font-size: 13px;
  font-weight: 700;
  margin-top: 8px;
}

.model-workspace {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.workspace-hero,
.panel {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fff;
  padding: 18px;
}

.workspace-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 16px;
}

.hero-main { min-width: 0; }

.hero-title-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
}

.hero-title-row h2 {
  margin: 0;
  color: #111827;
  font-size: 22px;
  font-weight: 900;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

.workspace-hero > .hero-main > p:not(.eyebrow) {
  margin: 6px 0 0;
  color: #64748b;
  font-size: 13px;
  font-weight: 700;
}

.hero-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  justify-content: flex-end;
  align-self: start;
}

.metric-grid {
  grid-column: 1 / -1;
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}

.metric-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #f8fafc;
  padding: 12px;
}

.metric-cell span, .metric-cell small { color: #64748b; font-size: 11px; font-weight: 800; }
.metric-cell strong { color: #0f172a; font-size: 18px; font-weight: 900; line-height: 1.2; }

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.section-head h3 { margin: 0; color: #0f172a; font-size: 15px; font-weight: 900; }
.section-head p { margin: 4px 0 0; color: #64748b; font-size: 12px; font-weight: 700; }

.route-deployment { display: flex; flex-direction: column; gap: 2px; }
.route-endpoint-row { display: flex; align-items: center; gap: 6px; }
.route-endpoint { color: #2563eb; font-weight: 700; font-size: 13px; }
.route-model { color: #334155; font-weight: 700; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }
.oauth-badge { flex-shrink: 0; }

.priority-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 26px;
  height: 20px;
  border-radius: 4px;
  background: #f1f5f9;
  color: #475569;
  font-size: 12px;
  font-weight: 700;
}

/* Price display */
.price-display { display: flex; flex-direction: column; gap: 0; }

.price-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 0;
  border-bottom: 1px solid #f1f5f9;
  font-size: 13px;
}

.price-item span { color: #64748b; font-weight: 700; }
.price-item strong { color: #0f172a; font-weight: 900; }

.price-updated { margin-top: 10px; color: #94a3b8; font-size: 11px; font-weight: 600; }
.price-empty-hint { color: #94a3b8; font-size: 12px; font-weight: 700; padding: 10px 0; }

.price-unset {
  padding: 16px;
  border: 1px dashed #fca5a5;
  border-radius: 8px;
  background: #fff7f7;
}

.price-unset p { margin: 0; color: #dc2626; font-size: 13px; font-weight: 700; }

/* Empty workspace */
.empty-workspace {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  min-height: 380px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fff;
  padding: 40px;
}

.empty-workspace h2 { margin: 0; color: #111827; font-size: 22px; font-weight: 900; }
.empty-workspace p:not(.eyebrow) {
  max-width: 480px;
  margin: 12px 0 22px;
  color: #64748b;
  font-size: 14px;
  font-weight: 700;
  line-height: 1.7;
}

.form-hint { display: block; margin-top: 4px; color: #94a3b8; font-size: 11px; }

.model-stepper { margin-bottom: 18px; }

.form-section-hint { margin: 0 0 14px; color: #475569; font-size: 13px; }
.form-section-hint strong { color: #2563eb; }

.suggestion-banner { margin-bottom: 14px; }

/* Route picker */
.picker-filters {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}

.picker-filter { width: 160px; }
.picker-search { flex: 1; min-width: 200px; }

.picker-table-wrap {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  max-height: 420px;
  overflow-y: auto;
  background: #fff;
}

.picker-row {
  display: grid;
  grid-template-columns: 36px minmax(0, 1fr) 90px 140px 80px;
  gap: 8px;
  align-items: center;
  padding: 8px 12px;
  border-bottom: 1px solid #f1f5f9;
  font-size: 13px;
  transition: background 0.13s;
}

.picker-row:hover:not(.is-disabled) { background: #f8fafc; }
.picker-row.is-selected { background: #eff6ff; }
.picker-row.is-disabled { background: #fafafa; color: #cbd5e1; }
.picker-row.is-disabled .endpoint-name,
.picker-row.is-disabled .upstream-model { color: #cbd5e1; }

.picker-row-head {
  position: sticky;
  top: 0;
  background: #f8fafc;
  font-size: 11px;
  font-weight: 800;
  color: #64748b;
  text-transform: uppercase;
  z-index: 1;
}

.cell-endpoint { display: flex; align-items: center; gap: 6px; overflow: hidden; }
.endpoint-name { color: #2563eb; font-weight: 700; }
.endpoint-sep { color: #cbd5e1; }
.upstream-model { color: #0f172a; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-weight: 700; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.already-tag { color: #94a3b8; font-size: 10px; font-weight: 700; margin-left: 4px; }

.cell-protocol { color: #475569; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11px; }
.cell-actions { text-align: right; }

.picker-empty {
  padding: 40px;
  text-align: center;
  color: #94a3b8;
  font-size: 13px;
}

.row-override {
  grid-column: 1 / -1;
  display: flex;
  gap: 14px;
  padding: 8px 12px;
  background: #fef3c7;
  border-radius: 6px;
  margin-top: 6px;
}

.ov-field {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 700;
  color: #475569;
}

.picker-summary {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  margin-top: 12px;
  background: #f8fafc;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  font-size: 12px;
  color: #475569;
  font-weight: 700;
}

.summary-count strong { color: #2563eb; font-size: 14px; }
.summary-sep { color: #cbd5e1; }

@media (max-width: 1280px) {
  .model-workbench { grid-template-columns: 1fr; }
  .model-rail { position: static; }
  .model-list { max-height: none; }
}

@media (max-width: 900px) {
  .workspace-hero,
  .metric-grid { grid-template-columns: 1fr; }
  .hero-actions { justify-content: flex-start; }
}
</style>
