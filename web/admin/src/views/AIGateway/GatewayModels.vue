<script setup>
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus, Refresh } from '@element-plus/icons-vue'
import {
  capabilityOptions,
  createModel,
  createModelDeployment,
  createModelPrice,
  listModelDeployments,
  listModelPrices,
  listModels,
  listProviderEndpoints,
  listProviders,
  protocolOptions,
  statusOptions,
  updateModelDeploymentStatus,
  updateModelPriceStatus,
  updateModelStatus
} from '@/api/aiGateway'

const loading = shallowRef(false)
const deploymentLoading = shallowRef(false)
const priceLoading = shallowRef(false)
const modelDialogVisible = shallowRef(false)
const deploymentDialogVisible = shallowRef(false)
const priceDialogVisible = shallowRef(false)
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

const openDeploymentDialog = async () => {
  if (!selectedModelId.value) {
    ElMessage.warning('请先选择模型')
    return
  }
  resetDeploymentForm()
  await fetchProviders()
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

const submitModel = async () => {
  await createModel({
    ...modelForm,
    context_window: modelForm.context_window || undefined,
    max_output_tokens: modelForm.max_output_tokens || undefined
  })
  ElMessage.success('模型已创建')
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

  await createModelDeployment(selectedModelId.value, {
    endpoint_id: deploymentForm.endpoint_id,
    upstream_model: deploymentForm.upstream_model,
    capability_type: deploymentForm.capability_type,
    upstream_protocol: deploymentForm.upstream_protocol,
    upstream_parameters: upstreamParameters,
    priority: deploymentForm.priority,
    weight: deploymentForm.weight,
    supports_stream: deploymentForm.supports_stream,
    status: deploymentForm.status
  })
  ElMessage.success('部署映射已创建')
  deploymentDialogVisible.value = false
  await fetchDeployments()
}

const submitModelPrice = async () => {
  await createModelPrice(selectedModelId.value, {
    ...priceForm,
    effective_from: new Date().toISOString()
  })
  ElMessage.success('销售价已创建')
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
        <el-table-column label="操作" width="88" fixed="right">
          <template #default="{ row }">
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
        <el-table-column prop="platform_input_price_per_1m" label="平台输入/1M" width="130" align="right" />
        <el-table-column prop="platform_output_price_per_1m" label="平台输出/1M" width="130" align="right" />
        <el-table-column prop="platform_image_price" label="平台单图" width="110" align="right" />
        <el-table-column prop="tenant_input_price_per_1m" label="租户输入/1M" width="130" align="right" />
        <el-table-column prop="tenant_output_price_per_1m" label="租户输出/1M" width="130" align="right" />
        <el-table-column prop="tenant_image_price" label="租户单图" width="110" align="right" />
        <el-table-column prop="effective_from" label="生效时间" min-width="180" show-overflow-tooltip />
        <el-table-column label="状态" width="95">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="88" fixed="right">
          <template #default="{ row }">
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
        <el-table-column label="操作" width="90" fixed="right">
          <template #default="{ row }">
            <el-button link :type="row.status === 'active' ? 'warning' : 'success'" @click="toggleDeployment(row)">
              {{ row.status === 'active' ? '禁用' : '启用' }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <el-dialog v-model="modelDialogVisible" title="新增对外模型" width="620px">
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
        <el-button type="primary" @click="submitModel">创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="deploymentDialogVisible" title="新增部署映射" width="680px">
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
        <el-button type="primary" @click="submitDeployment">创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="priceDialogVisible" title="新增模型销售价" width="680px">
      <el-form :model="priceForm" label-position="top">
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="平台输入/1M">
            <el-input-number v-model="priceForm.platform_input_price_per_1m" :min="0" class="w-full" />
          </el-form-item>
          <el-form-item label="平台输出/1M">
            <el-input-number v-model="priceForm.platform_output_price_per_1m" :min="0" class="w-full" />
          </el-form-item>
          <el-form-item label="平台单图">
            <el-input-number v-model="priceForm.platform_image_price" :min="0" class="w-full" />
          </el-form-item>
          <el-form-item label="租户输入/1M">
            <el-input-number v-model="priceForm.tenant_input_price_per_1m" :min="0" class="w-full" />
          </el-form-item>
          <el-form-item label="租户输出/1M">
            <el-input-number v-model="priceForm.tenant_output_price_per_1m" :min="0" class="w-full" />
          </el-form-item>
          <el-form-item label="租户单图">
            <el-input-number v-model="priceForm.tenant_image_price" :min="0" class="w-full" />
          </el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="priceDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitModelPrice">创建</el-button>
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
