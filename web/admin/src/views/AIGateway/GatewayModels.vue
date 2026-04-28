<script setup>
import { computed, onMounted, reactive, shallowRef, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Edit, Plus, Refresh } from '@element-plus/icons-vue'
import {
  capabilityOptions,
  createModel,
  createModelRoute,
  deleteModelRoute,
  formatCredits,
  formatTimestamp,
  getModelPrice,
  listModelRoutes,
  listModels,
  listUpstreamDeployments,
  nowTimestamp,
  statusOptions,
  updateModel,
  updateModelRoute,
  updateModelRouteStatus,
  updateModelStatus,
  upsertModelPrice
} from '@/api/aiGateway'

// ── Loading states ──────────────────────────────────────────────────────────
const loading = shallowRef(false)
const priceLoading = shallowRef(false)
const routeLoading = shallowRef(false)

// ── Dialog visibility ───────────────────────────────────────────────────────
const modelDialogVisible = shallowRef(false)
const priceDialogVisible = shallowRef(false)
const routeDialogVisible = shallowRef(false)

// ── Editing state ───────────────────────────────────────────────────────────
const editingModelId = shallowRef('')
const editingRouteId = shallowRef('')

// ── Data ────────────────────────────────────────────────────────────────────
const models = shallowRef([])
const modelPrice = shallowRef(null)
const modelRoutes = shallowRef([])
const upstreamDeployments = shallowRef([])

// ── Selection & filter ──────────────────────────────────────────────────────
const selectedModelId = shallowRef('')
const searchQuery = shallowRef('')
const capabilityFilter = shallowRef('all')

// ── Forms ───────────────────────────────────────────────────────────────────
const modelForm = reactive({
  model_code: '',
  display_name: '',
  capability_type: 'chat',
  context_window: null,
  default_max_output_tokens: 4096,
  max_output_tokens: null,
  status: 'active'
})

const priceForm = reactive({
  input_price_per_1m: 0,
  output_price_per_1m: 0,
  image_size_prices: [],     // [{size:'1024x1024', price:100}]
  video_price_per_second: 0,
  audio_tts_price_per_1m_chars: 0,
  audio_stt_price_per_minute: 0
})

const routeForm = reactive({
  upstream_deployment_id: '',
  priority: 100,
  weight: 100,
  supports_stream: true,
  status: 'active'
})

// ── Computed ─────────────────────────────────────────────────────────────────
const isEditingModel = computed(() => Boolean(editingModelId.value))
const isEditingRoute = computed(() => Boolean(editingRouteId.value))

const selectedModel = computed(() =>
  models.value.find((m) => m.id === selectedModelId.value)
)

const selectedCapabilityType = computed(() => selectedModel.value?.capability_type || '')

const filteredModels = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return models.value.filter((m) => {
    const matchesCap = capabilityFilter.value === 'all' || m.capability_type === capabilityFilter.value
    if (!matchesCap) return false
    if (!q) return true
    return m.display_name.toLowerCase().includes(q) || m.model_code.toLowerCase().includes(q)
  })
})

const upstreamDeploymentOptions = computed(() =>
  upstreamDeployments.value.map((d) => ({
    label: `${d.name} · ${d.upstream_model}`,
    value: d.id
  }))
)

const capabilityFilterOptions = computed(() => [
  { label: '全部', value: 'all' },
  ...capabilityOptions
])

const showTokenPrice = computed(() =>
  ['chat', 'embedding', 'rerank'].includes(selectedCapabilityType.value)
)
const showOutputPrice = computed(() => selectedCapabilityType.value === 'chat')
const showImagePrice = computed(() => selectedCapabilityType.value === 'image')
const showVideoPrice = computed(() => selectedCapabilityType.value === 'video')
const showAudioTTSPrice = computed(() => selectedCapabilityType.value === 'audio_tts')
const showAudioSTTPrice = computed(() => selectedCapabilityType.value === 'audio_stt')

const activeRouteCount = computed(() => modelRoutes.value.filter((r) => r.status === 'active').length)

// ── Utility ──────────────────────────────────────────────────────────────────
const statusTagType = (status) => ({ active: 'success', inactive: 'warning', disabled: 'danger' }[status] || 'info')

const capabilityLabel = (value) => capabilityOptions.find((o) => o.value === value)?.label || value || '-'

const capabilityDotClass = (value) => {
  const map = { chat: 'dot-chat', image: 'dot-image', video: 'dot-video', embedding: 'dot-embedding', audio_tts: 'dot-audio', audio_stt: 'dot-audio', rerank: 'dot-rerank' }
  return map[value] || 'dot-default'
}

// ── Reset forms ───────────────────────────────────────────────────────────────
const resetModelForm = () => {
  editingModelId.value = ''
  Object.assign(modelForm, {
    model_code: '', display_name: '', capability_type: 'chat',
    context_window: null, default_max_output_tokens: 4096, max_output_tokens: null, status: 'active'
  })
}

const resetPriceForm = () => {
  const p = modelPrice.value
  Object.assign(priceForm, {
    input_price_per_1m: p?.input_price_per_1m ?? 0,
    output_price_per_1m: p?.output_price_per_1m ?? 0,
    image_size_prices: parseSizePrices(p?.image_size_prices),
    video_price_per_second: p?.video_price_per_second ?? 0,
    audio_tts_price_per_1m_chars: p?.audio_tts_price_per_1m_chars ?? 0,
    audio_stt_price_per_minute: p?.audio_stt_price_per_minute ?? 0
  })
}

const resetRouteForm = () => {
  editingRouteId.value = ''
  Object.assign(routeForm, { upstream_deployment_id: '', priority: 100, weight: 100, supports_stream: true, status: 'active' })
}

const applyModelForm = (row) => {
  editingModelId.value = row.id
  Object.assign(modelForm, {
    model_code: row.model_code, display_name: row.display_name,
    capability_type: row.capability_type, context_window: row.context_window ?? null,
    default_max_output_tokens: row.default_max_output_tokens,
    max_output_tokens: row.max_output_tokens ?? null, status: row.status
  })
}

const applyRouteForm = (row) => {
  editingRouteId.value = row.id
  Object.assign(routeForm, {
    upstream_deployment_id: row.upstream_deployment_id,
    priority: row.priority, weight: row.weight,
    supports_stream: row.supports_stream, status: row.status
  })
}

// ── Image size price helpers ──────────────────────────────────────────────────
const parseSizePrices = (raw) => {
  if (!raw) return []
  try {
    const obj = typeof raw === 'string' ? JSON.parse(raw) : raw
    return Object.entries(obj).map(([size, price]) => ({ size, price: Number(price) }))
  } catch {
    return []
  }
}

const serializeSizePrices = (arr) => {
  const obj = {}
  for (const { size, price } of arr) {
    if (size && size.trim()) obj[size.trim()] = Number(price) || 0
  }
  return obj
}

const addSizePrice = () => priceForm.image_size_prices.push({ size: '', price: 0 })
const removeSizePrice = (idx) => priceForm.image_size_prices.splice(idx, 1)

// ── Fetch ─────────────────────────────────────────────────────────────────────
const fetchModels = async () => {
  loading.value = true
  try {
    models.value = await listModels()
    if (!selectedModelId.value && models.value.length > 0) {
      selectedModelId.value = models.value[0].id
      await fetchSelectedModelDetail()
    }
  } finally {
    loading.value = false
  }
}

const fetchSelectedModelDetail = async () => {
  await Promise.all([fetchModelPrice(), fetchModelRoutes(), fetchUpstreamDeployments()])
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

const fetchUpstreamDeployments = async () => {
  try { upstreamDeployments.value = await listUpstreamDeployments() }
  catch { upstreamDeployments.value = [] }
}

// ── Selection ─────────────────────────────────────────────────────────────────
const selectModel = async (model) => {
  selectedModelId.value = model.id
  await fetchSelectedModelDetail()
}

// ── Dialog handlers ───────────────────────────────────────────────────────────
const openModelDialog = () => { resetModelForm(); modelDialogVisible.value = true }
const openModelEditDialog = (row) => { applyModelForm(row); modelDialogVisible.value = true }

const openPriceDialog = () => {
  if (!selectedModelId.value) { ElMessage.warning('请先选择模型'); return }
  resetPriceForm()
  priceDialogVisible.value = true
}

const openRouteDialog = async () => {
  if (!selectedModelId.value) { ElMessage.warning('请先选择模型'); return }
  await fetchUpstreamDeployments()
  resetRouteForm()
  routeDialogVisible.value = true
}

const openRouteEditDialog = async (row) => {
  await fetchUpstreamDeployments()
  applyRouteForm(row)
  routeDialogVisible.value = true
}

// ── Submit ─────────────────────────────────────────────────────────────────────
const submitModel = async () => {
  const payload = {
    ...modelForm,
    context_window: modelForm.context_window || undefined,
    max_output_tokens: modelForm.max_output_tokens || undefined
  }
  if (editingModelId.value) {
    await updateModel(editingModelId.value, payload)
    ElMessage.success('模型已保存')
  } else {
    await createModel(payload)
    ElMessage.success('模型已创建')
  }
  modelDialogVisible.value = false
  await fetchModels()
}

const submitModelPrice = async () => {
  const payload = {
    input_price_per_1m: priceForm.input_price_per_1m,
    output_price_per_1m: priceForm.output_price_per_1m,
    image_size_prices: serializeSizePrices(priceForm.image_size_prices),
    video_price_per_second: priceForm.video_price_per_second,
    audio_tts_price_per_1m_chars: priceForm.audio_tts_price_per_1m_chars,
    audio_stt_price_per_minute: priceForm.audio_stt_price_per_minute
  }
  await upsertModelPrice(selectedModelId.value, payload)
  ElMessage.success('销售价已保存')
  priceDialogVisible.value = false
  await fetchModelPrice()
}

const submitModelRoute = async () => {
  const payload = {
    upstream_deployment_id: routeForm.upstream_deployment_id,
    priority: routeForm.priority,
    weight: routeForm.weight,
    supports_stream: routeForm.supports_stream,
    status: routeForm.status
  }
  if (editingRouteId.value) {
    await updateModelRoute(selectedModelId.value, editingRouteId.value, payload)
    ElMessage.success('路由已保存')
  } else {
    await createModelRoute(selectedModelId.value, payload)
    ElMessage.success('路由已创建')
  }
  routeDialogVisible.value = false
  await fetchModelRoutes()
}

// ── Toggle & delete ───────────────────────────────────────────────────────────
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

// ── Lifecycle ─────────────────────────────────────────────────────────────────
onMounted(fetchModels)
</script>

<template>
  <div class="model-workbench">
    <!-- ── Left rail ── -->
    <aside class="model-rail">
      <div class="rail-head">
        <div>
          <p class="eyebrow">Public Models</p>
          <h2>对外模型</h2>
        </div>
        <el-button type="primary" :icon="Plus" @click="openModelDialog">新增</el-button>
      </div>

      <!-- Search + filter -->
      <div class="rail-filter">
        <el-input v-model="searchQuery" placeholder="搜索模型名称或编码" clearable size="small" />
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
            <strong>{{ model.display_name }}</strong>
            <small>{{ model.model_code }}</small>
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
            <h2>{{ selectedModel.display_name }}</h2>
            <el-tag :type="statusTagType(selectedModel.status)" effect="dark">{{ selectedModel.status }}</el-tag>
          </div>
          <p>{{ selectedModel.model_code }} · {{ capabilityLabel(selectedModel.capability_type) }}</p>
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
          <div class="metric-cell">
            <span>上下文窗口</span>
            <strong>{{ selectedModel.context_window ? selectedModel.context_window.toLocaleString() : '-' }}</strong>
            <small>tokens</small>
          </div>
          <div class="metric-cell">
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
            <p>{{ selectedModel.model_code }} · 配置上游部署映射和优先级权重</p>
          </div>
          <el-button type="primary" :icon="Plus" @click="openRouteDialog">新增路由</el-button>
        </div>
        <el-table v-loading="routeLoading" :data="modelRoutes" border stripe class="w-full">
          <el-table-column label="上游部署" min-width="140">
            <template #default="{ row }">
              <div class="route-deployment">
                <strong>{{ row.upstream_deployment_name || '-' }}</strong>
                <small v-if="row.upstream_model">{{ row.upstream_model }}</small>
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
          <el-table-column label="操作" width="210" fixed="right">
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
            <p>{{ selectedModel.model_code }} · 所有价格为积分单位</p>
          </div>
          <el-button type="primary" :icon="Edit" @click="openPriceDialog">
            {{ modelPrice ? '修改价格' : '设置价格' }}
          </el-button>
        </div>

        <div v-loading="priceLoading">
          <div v-if="modelPrice" class="price-display">
            <!-- Token prices (chat / embedding / rerank) -->
            <template v-if="showTokenPrice">
              <div class="price-item">
                <span>输入 / 1M tokens</span>
                <strong>{{ formatCredits(modelPrice.input_price_per_1m) }} 积分</strong>
              </div>
              <div v-if="showOutputPrice" class="price-item">
                <span>输出 / 1M tokens</span>
                <strong>{{ formatCredits(modelPrice.output_price_per_1m) }} 积分</strong>
              </div>
            </template>
            <!-- Image prices -->
            <template v-if="showImagePrice">
              <div v-for="(entry, idx) in parseSizePrices(modelPrice.image_size_prices)" :key="idx" class="price-item">
                <span>{{ entry.size }}</span>
                <strong>{{ formatCredits(entry.price) }} 积分 / 张</strong>
              </div>
              <div v-if="parseSizePrices(modelPrice.image_size_prices).length === 0" class="price-empty-hint">
                尚未配置尺寸定价
              </div>
            </template>
            <!-- Video price -->
            <div v-if="showVideoPrice" class="price-item">
              <span>视频 / 秒</span>
              <strong>{{ formatCredits(modelPrice.video_price_per_second) }} 积分</strong>
            </div>
            <!-- Audio TTS price -->
            <div v-if="showAudioTTSPrice" class="price-item">
              <span>TTS / 1M 字符</span>
              <strong>{{ formatCredits(modelPrice.audio_tts_price_per_1m_chars) }} 积分</strong>
            </div>
            <!-- Audio STT price -->
            <div v-if="showAudioSTTPrice" class="price-item">
              <span>STT / 分钟</span>
              <strong>{{ formatCredits(modelPrice.audio_stt_price_per_minute) }} 积分</strong>
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
      <h2>先创建一个对外模型</h2>
      <p>对外模型是用户调用时的统一入口。创建后即可配置路由映射和销售价格。</p>
      <el-button type="primary" :icon="Plus" @click="openModelDialog">新增模型</el-button>
    </main>

    <!-- ── 模型对话框 ── -->
    <el-dialog v-model="modelDialogVisible" :title="isEditingModel ? '编辑对外模型' : '新增对外模型'" width="620px">
      <el-form :model="modelForm" label-position="top">
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="模型编码" required>
            <el-input v-model="modelForm.model_code" placeholder="gpt-5.4" />
          </el-form-item>
          <el-form-item label="显示名称" required>
            <el-input v-model="modelForm.display_name" placeholder="GPT 5.4" />
          </el-form-item>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="能力类型">
            <el-select v-model="modelForm.capability_type" class="w-full">
              <el-option v-for="item in capabilityOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="状态">
            <el-select v-model="modelForm.status" class="w-full">
              <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
        </div>
        <div class="grid grid-cols-3 gap-4">
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
      <template #footer>
        <el-button @click="modelDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitModel">{{ isEditingModel ? '保存' : '创建' }}</el-button>
      </template>
    </el-dialog>

    <!-- ── 销售价对话框 ── -->
    <el-dialog v-model="priceDialogVisible" title="设置销售价" width="580px">
      <el-form :model="priceForm" label-position="top">

        <!-- Token prices -->
        <template v-if="showTokenPrice">
          <div class="grid grid-cols-2 gap-4">
            <el-form-item label="输入价格 / 1M tokens（积分）">
              <el-input-number v-model="priceForm.input_price_per_1m" :min="0" :precision="0" class="w-full" />
            </el-form-item>
            <el-form-item v-if="showOutputPrice" label="输出价格 / 1M tokens（积分）">
              <el-input-number v-model="priceForm.output_price_per_1m" :min="0" :precision="0" class="w-full" />
            </el-form-item>
          </div>
        </template>

        <!-- Image size prices -->
        <template v-if="showImagePrice">
          <el-form-item label="尺寸定价（积分/张）">
            <div class="size-price-editor">
              <div
                v-for="(entry, idx) in priceForm.image_size_prices"
                :key="idx"
                class="size-price-row"
              >
                <el-input v-model="entry.size" placeholder="1024x1024" class="size-input" />
                <el-input-number v-model="entry.price" :min="0" :precision="0" class="price-input" />
                <el-button link type="danger" :icon="Delete" @click="removeSizePrice(idx)" />
              </div>
              <el-button :icon="Plus" @click="addSizePrice">添加尺寸</el-button>
            </div>
            <small class="form-hint">不在列表中的尺寸将直接拒绝请求</small>
          </el-form-item>
        </template>

        <!-- Video price -->
        <el-form-item v-if="showVideoPrice" label="视频价格 / 秒（积分）">
          <el-input-number v-model="priceForm.video_price_per_second" :min="0" :precision="0" class="w-full" />
        </el-form-item>

        <!-- Audio TTS price -->
        <el-form-item v-if="showAudioTTSPrice" label="TTS 价格 / 1M 字符（积分）">
          <el-input-number v-model="priceForm.audio_tts_price_per_1m_chars" :min="0" :precision="0" class="w-full" />
        </el-form-item>

        <!-- Audio STT price -->
        <el-form-item v-if="showAudioSTTPrice" label="STT 价格 / 分钟（积分）">
          <el-input-number v-model="priceForm.audio_stt_price_per_minute" :min="0" :precision="0" class="w-full" />
        </el-form-item>

      </el-form>
      <template #footer>
        <el-button @click="priceDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitModelPrice">保存价格</el-button>
      </template>
    </el-dialog>

    <!-- ── 路由对话框 ── -->
    <el-dialog v-model="routeDialogVisible" :title="isEditingRoute ? '编辑模型路由' : '新增模型路由'" width="560px">
      <el-form :model="routeForm" label-position="top">
        <el-form-item label="上游部署" required>
          <el-select v-model="routeForm.upstream_deployment_id" class="w-full" filterable placeholder="选择上游部署">
            <el-option v-for="item in upstreamDeploymentOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="优先级">
            <el-input-number v-model="routeForm.priority" :min="0" :precision="0" class="w-full" />
            <small class="form-hint">数字越小优先级越高</small>
          </el-form-item>
          <el-form-item label="权重">
            <el-input-number v-model="routeForm.weight" :min="0" :precision="0" class="w-full" />
            <small class="form-hint">用于加权随机选择</small>
          </el-form-item>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="支持流式">
            <el-switch v-model="routeForm.supports_stream" />
          </el-form-item>
          <el-form-item label="状态">
            <el-select v-model="routeForm.status" class="w-full">
              <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="routeDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitModelRoute">{{ isEditingRoute ? '保存' : '创建' }}</el-button>
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

/* ── Rail ── */
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

/* Status dot */
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
}

.model-item-text small {
  font-size: 11px;
  color: #94a3b8;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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

/* ── Workspace ── */
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
.route-deployment strong { color: #334155; font-weight: 700; }
.route-deployment small { color: #94a3b8; font-size: 11px; }

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

/* ── Price display ── */
.price-display {
  display: flex;
  flex-direction: column;
  gap: 0;
}

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

.price-updated {
  margin-top: 10px;
  color: #94a3b8;
  font-size: 11px;
  font-weight: 600;
}

.price-empty-hint {
  color: #94a3b8;
  font-size: 12px;
  font-weight: 700;
  padding: 10px 0;
}

.price-unset {
  padding: 16px;
  border: 1px dashed #fca5a5;
  border-radius: 8px;
  background: #fff7f7;
}

.price-unset p {
  margin: 0;
  color: #dc2626;
  font-size: 13px;
  font-weight: 700;
}

/* ── Image size price editor ── */
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

/* ── Empty workspace ── */
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
