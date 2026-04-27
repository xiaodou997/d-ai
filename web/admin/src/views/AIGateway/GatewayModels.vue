<script setup>
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { ElMessage } from 'element-plus'
import { Edit, Plus, Refresh } from '@element-plus/icons-vue'
import {
  capabilityOptions,
  createModel,
  createModelDeployment,
  createModelPrice,
  centsToYuan,
  formatTimestamp,
  formatYuan,
  listModelDeployments,
  listModelPrices,
  listModels,
  listProviderEndpoints,
  listProviders,
  nowTimestamp,
  protocolOptions,
  statusOptions,
  updateModel,
  updateModelDeployment,
  updateModelDeploymentStatus,
  updateModelPrice,
  updateModelPriceStatus,
  updateModelStatus,
  yuanToCents
} from '@/api/aiGateway'

const loading = shallowRef(false)
const deploymentLoading = shallowRef(false)
const priceLoading = shallowRef(false)
const modelDialogVisible = shallowRef(false)
const deploymentDialogVisible = shallowRef(false)
const priceDialogVisible = shallowRef(false)
const editingModelId = shallowRef('')
const editingDeploymentId = shallowRef('')
const editingPriceId = shallowRef('')
const models = shallowRef([])
const deployments = shallowRef([])
const modelPrices = shallowRef([])
const providers = shallowRef([])
const endpoints = shallowRef([])
const selectedModelId = shallowRef('')
const selectedProviderId = shallowRef('')

const modelForm = reactive({
  model_code: '',
  display_name: '',
  capability_type: 'chat',
  context_window: null,
  default_max_output_tokens: 4096,
  max_output_tokens: null,
  status: 'active'
})

const deploymentForm = reactive({
  endpoint_id: '',
  upstream_model: '',
  capability_type: 'chat',
  upstream_protocol: 'openai_chat_completions',
  upstream_parameters_text: '{}',
  priority: 100,
  weight: 100,
  supports_stream: true,
  status: 'active'
})

const priceForm = reactive({
  platform_input_price_per_1m: 0,
  platform_output_price_per_1m: 0,
  platform_image_price: 0,
  tenant_input_price_per_1m: 0,
  tenant_output_price_per_1m: 0,
  tenant_image_price: 0,
  status: 'active'
})

const isEditingModel = computed(() => Boolean(editingModelId.value))
const isEditingDeployment = computed(() => Boolean(editingDeploymentId.value))
const isEditingPrice = computed(() => Boolean(editingPriceId.value))

const selectedModel = computed(() =>
  models.value.find((item) => item.id === selectedModelId.value)
)

const endpointOptions = computed(() =>
  endpoints.value.map((item) => ({
    label: `${item.name} · ${item.base_url}`,
    value: item.id
  }))
)

const statusTagType = (status) => {
  const map = { active: 'success', inactive: 'warning', disabled: 'danger' }
  return map[status] || 'info'
}

const resetPriceForm = () => {
  editingPriceId.value = ''
  Object.assign(priceForm, {
    platform_input_price_per_1m: 0,
    platform_output_price_per_1m: 0,
    platform_image_price: 0,
    tenant_input_price_per_1m: 0,
    tenant_output_price_per_1m: 0,
    tenant_image_price: 0,
    status: 'active'
  })
}

const resetModelForm = () => {
  editingModelId.value = ''
  Object.assign(modelForm, {
    model_code: '',
    display_name: '',
    capability_type: 'chat',
    context_window: null,
    default_max_output_tokens: 4096,
    max_output_tokens: null,
    status: 'active'
  })
}

const resetDeploymentForm = () => {
  editingDeploymentId.value = ''
  const capabilityType = selectedModel.value?.capability_type || 'chat'
  Object.assign(deploymentForm, {
    endpoint_id: '',
    upstream_model: selectedModel.value?.model_code || '',
    capability_type: capabilityType,
    upstream_protocol: capabilityType === 'image' ? 'openai_images_generations' : 'openai_chat_completions',
    upstream_parameters_text: '{}',
    priority: 100,
    weight: 100,
    supports_stream: true,
    status: 'active'
  })
}

const applyModelForm = (row) => {
  editingModelId.value = row.id
  Object.assign(modelForm, {
    model_code: row.model_code,
    display_name: row.display_name,
    capability_type: row.capability_type,
    context_window: row.context_window ?? null,
    default_max_output_tokens: row.default_max_output_tokens,
    max_output_tokens: row.max_output_tokens ?? null,
    status: row.status
  })
}

const applyDeploymentForm = (row) => {
  editingDeploymentId.value = row.id
  Object.assign(deploymentForm, {
    endpoint_id: row.endpoint_id,
    upstream_model: row.upstream_model,
    capability_type: row.capability_type,
    upstream_protocol: row.upstream_protocol,
    upstream_parameters_text: JSON.stringify(row.upstream_parameters || {}, null, 2),
    priority: row.priority,
    weight: row.weight,
    supports_stream: row.supports_stream,
    status: row.status
  })
}

const applyPriceForm = (row) => {
  editingPriceId.value = row.id
  Object.assign(priceForm, {
    platform_input_price_per_1m: centsToYuan(row.platform_input_price_per_1m),
    platform_output_price_per_1m: centsToYuan(row.platform_output_price_per_1m),
    platform_image_price: centsToYuan(row.platform_image_price),
    tenant_input_price_per_1m: centsToYuan(row.tenant_input_price_per_1m),
    tenant_output_price_per_1m: centsToYuan(row.tenant_output_price_per_1m),
    tenant_image_price: centsToYuan(row.tenant_image_price),
    status: row.status
  })
}

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
  await Promise.all([fetchDeployments(), fetchModelPrices()])
}

const fetchDeployments = async () => {
  if (!selectedModelId.value) {
    deployments.value = []
    return
  }
  deploymentLoading.value = true
  try {
    deployments.value = await listModelDeployments(selectedModelId.value)
  } finally {
    deploymentLoading.value = false
  }
}

const fetchModelPrices = async () => {
  if (!selectedModelId.value) {
    modelPrices.value = []
    return
  }
  priceLoading.value = true
  try {
    modelPrices.value = await listModelPrices(selectedModelId.value)
  } finally {
    priceLoading.value = false
  }
}

const selectModel = async (row) => {
  selectedModelId.value = row?.id || ''
  await fetchSelectedModelDetail()
}

const fetchProviders = async () => {
  providers.value = await listProviders()
  if (!selectedProviderId.value && providers.value.length > 0) {
    selectedProviderId.value = providers.value[0].id
  }
  await fetchEndpoints()
}

const fetchEndpoints = async () => {
  if (!selectedProviderId.value) {
    endpoints.value = []
    return
  }
  endpoints.value = await listProviderEndpoints(selectedProviderId.value)
}

const openModelDialog = () => {
  resetModelForm()
  modelDialogVisible.value = true
}

const openModelEditDialog = (row) => {
  applyModelForm(row)
  modelDialogVisible.value = true
}

const openDeploymentDialog = async () => {
  if (!selectedModelId.value) {
    ElMessage.warning('请先选择模型')
    return
  }
  resetDeploymentForm()
  await fetchProviders()
  deploymentDialogVisible.value = true
}

const openDeploymentEditDialog = async (row) => {
  if (!selectedModelId.value) {
    ElMessage.warning('请先选择模型')
    return
  }
  await fetchProviders()
  const provider = providers.value.find((item) => item.code === row.provider_code || item.name === row.provider_name)
  if (provider) {
    selectedProviderId.value = provider.id
    await fetchEndpoints()
  }
  applyDeploymentForm(row)
  deploymentDialogVisible.value = true
}

const openPriceDialog = () => {
  if (!selectedModelId.value) {
    ElMessage.warning('请先选择模型')
    return
  }
  resetPriceForm()
  priceDialogVisible.value = true
}

const openPriceEditDialog = (row) => {
  applyPriceForm(row)
  priceDialogVisible.value = true
}

const modelPayload = () => ({
  ...modelForm,
  context_window: modelForm.context_window || undefined,
  max_output_tokens: modelForm.max_output_tokens || undefined
})

const submitModel = async () => {
  if (editingModelId.value) {
    await updateModel(editingModelId.value, modelPayload())
    ElMessage.success('模型已保存')
  } else {
    await createModel(modelPayload())
    ElMessage.success('模型已创建')
  }
  modelDialogVisible.value = false
  await fetchModels()
}

const submitDeployment = async () => {
  let upstreamParameters = {}
  try {
    upstreamParameters = JSON.parse(deploymentForm.upstream_parameters_text || '{}')
  } catch {
    ElMessage.error('上游参数必须是合法 JSON')
    return
  }

  const payload = {
    endpoint_id: deploymentForm.endpoint_id,
    upstream_model: deploymentForm.upstream_model,
    capability_type: deploymentForm.capability_type,
    upstream_protocol: deploymentForm.upstream_protocol,
    upstream_parameters: upstreamParameters,
    priority: deploymentForm.priority,
    weight: deploymentForm.weight,
    supports_stream: deploymentForm.supports_stream,
    status: deploymentForm.status
  }
  if (editingDeploymentId.value) {
    await updateModelDeployment(selectedModelId.value, editingDeploymentId.value, payload)
    ElMessage.success('部署映射已保存')
  } else {
    await createModelDeployment(selectedModelId.value, payload)
    ElMessage.success('部署映射已创建')
  }
  deploymentDialogVisible.value = false
  await fetchDeployments()
}

const submitModelPrice = async () => {
  const payload = {
    platform_input_price_per_1m: yuanToCents(priceForm.platform_input_price_per_1m),
    platform_output_price_per_1m: yuanToCents(priceForm.platform_output_price_per_1m),
    platform_image_price: yuanToCents(priceForm.platform_image_price),
    tenant_input_price_per_1m: yuanToCents(priceForm.tenant_input_price_per_1m),
    tenant_output_price_per_1m: yuanToCents(priceForm.tenant_output_price_per_1m),
    tenant_image_price: yuanToCents(priceForm.tenant_image_price),
    effective_from: nowTimestamp(),
    status: priceForm.status
  }
  if (editingPriceId.value) {
    await updateModelPrice(selectedModelId.value, editingPriceId.value, payload)
    ElMessage.success('销售价已保存')
  } else {
    await createModelPrice(selectedModelId.value, payload)
    ElMessage.success('销售价已创建')
  }
  priceDialogVisible.value = false
  await fetchModelPrices()
}

const toggleModel = async (row) => {
  const nextStatus = row.status === 'active' ? 'disabled' : 'active'
  await updateModelStatus(row.id, nextStatus)
  ElMessage.success('模型状态已更新')
  await fetchModels()
}

const toggleDeployment = async (row) => {
  const nextStatus = row.status === 'active' ? 'disabled' : 'active'
  await updateModelDeploymentStatus(selectedModelId.value, row.id, nextStatus)
  ElMessage.success('部署状态已更新')
  await fetchDeployments()
}

const toggleModelPrice = async (row) => {
  const nextStatus = row.status === 'active' ? 'disabled' : 'active'
  await updateModelPriceStatus(selectedModelId.value, row.id, nextStatus)
  ElMessage.success('销售价状态已更新')
  await fetchModelPrices()
}

onMounted(async () => {
  await fetchModels()
  await fetchProviders()
})
</script>

<template>
  <div class="grid grid-cols-1 xl:grid-cols-[460px_minmax(0,1fr)] gap-4">
    <section class="panel">
      <div class="section-head">
        <div>
          <h3>对外模型</h3>
          <p>用户请求时使用的统一模型名</p>
        </div>
        <div class="flex gap-2">
          <el-button :icon="Refresh" circle @click="fetchModels" />
          <el-button type="primary" :icon="Plus" @click="openModelDialog">新增</el-button>
        </div>
      </div>

      <el-table
        v-loading="loading"
        :data="models"
        border
        stripe
        highlight-current-row
        class="w-full"
        @current-change="selectModel"
      >
        <el-table-column label="模型" min-width="190">
          <template #default="{ row }">
            <button class="model-link" type="button" @click="selectModel(row)">
              <span>{{ row.model_code }}</span>
              <small>{{ row.display_name }}</small>
            </button>
          </template>
        </el-table-column>
        <el-table-column prop="capability_type" label="能力" width="80" />
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="130" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" :icon="Edit" @click="openModelEditDialog(row)">编辑</el-button>
            <el-button link :type="row.status === 'active' ? 'warning' : 'success'" @click="toggleModel(row)">
              {{ row.status === 'active' ? '禁用' : '启用' }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <section class="panel xl:col-span-2">
      <div class="section-head">
        <div>
          <h3>模型销售价</h3>
          <p>{{ selectedModel?.model_code || '请选择模型' }}</p>
        </div>
        <div class="flex gap-2">
          <el-button :icon="Refresh" circle @click="fetchModelPrices" />
          <el-button type="primary" :icon="Plus" @click="openPriceDialog">新增销售价</el-button>
        </div>
      </div>

      <el-table v-loading="priceLoading" :data="modelPrices" border stripe class="w-full">
        <el-table-column label="平台输入/1M(元)" width="150" align="right">
          <template #default="{ row }">{{ formatYuan(row.platform_input_price_per_1m) }}</template>
        </el-table-column>
        <el-table-column label="平台输出/1M(元)" width="150" align="right">
          <template #default="{ row }">{{ formatYuan(row.platform_output_price_per_1m) }}</template>
        </el-table-column>
        <el-table-column label="平台单图(元)" width="130" align="right">
          <template #default="{ row }">{{ formatYuan(row.platform_image_price) }}</template>
        </el-table-column>
        <el-table-column label="租户输入/1M(元)" width="150" align="right">
          <template #default="{ row }">{{ formatYuan(row.tenant_input_price_per_1m) }}</template>
        </el-table-column>
        <el-table-column label="租户输出/1M(元)" width="150" align="right">
          <template #default="{ row }">{{ formatYuan(row.tenant_output_price_per_1m) }}</template>
        </el-table-column>
        <el-table-column label="租户单图(元)" width="130" align="right">
          <template #default="{ row }">{{ formatYuan(row.tenant_image_price) }}</template>
        </el-table-column>
        <el-table-column label="生效时间" min-width="180" show-overflow-tooltip>
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
            <el-button link :type="row.status === 'active' ? 'warning' : 'success'" @click="toggleModelPrice(row)">
              {{ row.status === 'active' ? '禁用' : '启用' }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <section class="panel">
      <div class="section-head">
        <div>
          <h3>部署映射</h3>
          <p>{{ selectedModel?.model_code || '请选择模型' }}</p>
        </div>
        <el-button type="primary" :icon="Plus" @click="openDeploymentDialog">新增映射</el-button>
      </div>

      <el-table v-loading="deploymentLoading" :data="deployments" border stripe class="w-full">
        <el-table-column prop="upstream_model" label="上游模型" min-width="160" />
        <el-table-column prop="provider_code" label="厂商" width="120" />
        <el-table-column prop="endpoint_name" label="接入点" min-width="140" />
        <el-table-column prop="upstream_protocol" label="协议" min-width="190" show-overflow-tooltip />
        <el-table-column prop="priority" label="优先级" width="90" align="right" />
        <el-table-column prop="weight" label="权重" width="80" align="right" />
        <el-table-column label="状态" width="95">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="130" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" :icon="Edit" @click="openDeploymentEditDialog(row)">编辑</el-button>
            <el-button link :type="row.status === 'active' ? 'warning' : 'success'" @click="toggleDeployment(row)">
              {{ row.status === 'active' ? '禁用' : '启用' }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

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
            <el-input-number v-model="modelForm.context_window" :min="0" class="w-full" />
          </el-form-item>
          <el-form-item label="默认输出 Token">
            <el-input-number v-model="modelForm.default_max_output_tokens" :min="1" class="w-full" />
          </el-form-item>
          <el-form-item label="最大输出 Token">
            <el-input-number v-model="modelForm.max_output_tokens" :min="0" class="w-full" />
          </el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="modelDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitModel">{{ isEditingModel ? '保存' : '创建' }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="deploymentDialogVisible" :title="isEditingDeployment ? '编辑部署映射' : '新增部署映射'" width="680px">
      <el-form :model="deploymentForm" label-position="top">
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="厂商">
            <el-select v-model="selectedProviderId" class="w-full" @change="fetchEndpoints">
              <el-option v-for="item in providers" :key="item.id" :label="item.name" :value="item.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="接入点" required>
            <el-select v-model="deploymentForm.endpoint_id" class="w-full" filterable>
              <el-option v-for="item in endpointOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="上游模型名" required>
            <el-input v-model="deploymentForm.upstream_model" placeholder="provider-model-name" />
          </el-form-item>
          <el-form-item label="上游协议">
            <el-select v-model="deploymentForm.upstream_protocol" class="w-full">
              <el-option v-for="item in protocolOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
        </div>
        <div class="grid grid-cols-4 gap-4">
          <el-form-item label="优先级">
            <el-input-number v-model="deploymentForm.priority" :min="0" class="w-full" />
          </el-form-item>
          <el-form-item label="权重">
            <el-input-number v-model="deploymentForm.weight" :min="0" class="w-full" />
          </el-form-item>
          <el-form-item label="流式">
            <el-switch v-model="deploymentForm.supports_stream" />
          </el-form-item>
          <el-form-item label="状态">
            <el-select v-model="deploymentForm.status" class="w-full">
              <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item label="默认上游参数 JSON">
          <el-input v-model="deploymentForm.upstream_parameters_text" type="textarea" :rows="4" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="deploymentDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitDeployment">{{ isEditingDeployment ? '保存' : '创建' }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="priceDialogVisible" :title="isEditingPrice ? '编辑模型销售价' : '新增模型销售价'" width="680px">
      <el-form :model="priceForm" label-position="top">
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="平台输入/1M(元)">
            <el-input-number v-model="priceForm.platform_input_price_per_1m" :min="0" class="w-full" />
          </el-form-item>
          <el-form-item label="平台输出/1M(元)">
            <el-input-number v-model="priceForm.platform_output_price_per_1m" :min="0" class="w-full" />
          </el-form-item>
          <el-form-item label="平台单图(元)">
            <el-input-number v-model="priceForm.platform_image_price" :min="0" class="w-full" />
          </el-form-item>
          <el-form-item label="租户输入/1M(元)">
            <el-input-number v-model="priceForm.tenant_input_price_per_1m" :min="0" class="w-full" />
          </el-form-item>
          <el-form-item label="租户输出/1M(元)">
            <el-input-number v-model="priceForm.tenant_output_price_per_1m" :min="0" class="w-full" />
          </el-form-item>
          <el-form-item label="租户单图(元)">
            <el-input-number v-model="priceForm.tenant_image_price" :min="0" class="w-full" />
          </el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="priceDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitModelPrice">{{ isEditingPrice ? '保存' : '创建' }}</el-button>
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

.section-head p {
  margin: 4px 0 0;
  color: #64748b;
  font-size: 12px;
}

.model-link {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-width: 100%;
  padding: 0;
  border: 0;
  background: transparent;
  color: #334155;
  text-align: left;
  cursor: pointer;
}

.model-link span {
  font-weight: 800;
}

.model-link small {
  color: #94a3b8;
  font-size: 11px;
}
</style>
