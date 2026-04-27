<script setup>
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { ElMessage } from 'element-plus'
import { Edit, Plus, Refresh, Switch } from '@element-plus/icons-vue'
import {
  checkProviderEndpointHealth,
  centsToYuan,
  createProviderModelPrice,
  createProvider,
  createProviderEndpoint,
  formatTimestamp,
  formatYuan,
  listProviderModelPrices,
  listProviderEndpoints,
  listProviders,
  nowTimestamp,
  protocolOptions,
  statusOptions,
  updateProvider,
  updateProviderEndpoint,
  updateProviderEndpointStatus,
  updateProviderModelPrice,
  updateProviderModelPriceStatus,
  updateProviderStatus,
  yuanToCents
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

const providerForm = reactive({
  code: '',
  name: '',
  provider_type: 'custom',
  protocol_type: 'openai_chat_completions',
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
  status: 'active'
})

const selectedProvider = computed(() =>
  providers.value.find((item) => item.id === selectedProviderId.value)
)

const isEditingProvider = computed(() => Boolean(editingProviderId.value))
const isEditingEndpoint = computed(() => Boolean(editingEndpointId.value))
const isEditingPrice = computed(() => Boolean(editingPriceId.value))

const endpointOptions = computed(() =>
  endpoints.value.map((item) => ({
    label: item.name,
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
    endpoint_id: '',
    upstream_model: '',
    capability_type: 'chat',
    currency: 'CNY_CREDITS',
    input_cost_per_1m: 0,
    output_cost_per_1m: 0,
    request_cost: 0,
    image_cost: 0,
    video_cost_per_second: 0,
    status: 'active'
  })
}

const resetProviderForm = () => {
  editingProviderId.value = ''
  Object.assign(providerForm, {
    code: '',
    name: '',
    provider_type: 'custom',
    protocol_type: 'openai_chat_completions',
    is_custom: true,
    status: 'active'
  })
}

const resetEndpointForm = () => {
  editingEndpointId.value = ''
  Object.assign(endpointForm, {
    name: '',
    base_url: '',
    protocol_type: selectedProvider.value?.protocol_type || 'openai_chat_completions',
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
    protocol_type: row.protocol_type,
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
    input_cost_per_1m: centsToYuan(row.input_cost_per_1m),
    output_cost_per_1m: centsToYuan(row.output_cost_per_1m),
    request_cost: centsToYuan(row.request_cost),
    image_cost: centsToYuan(row.image_cost),
    video_cost_per_second: centsToYuan(row.video_cost_per_second),
    status: row.status
  })
}

const fetchProviders = async () => {
  loading.value = true
  try {
    providers.value = await listProviders()
    if (!selectedProviderId.value && providers.value.length > 0) {
      selectedProviderId.value = providers.value[0].id
      await fetchSelectedProviderDetail()
    }
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

const openProviderEditDialog = (row) => {
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
    input_cost_per_1m: yuanToCents(priceForm.input_cost_per_1m),
    output_cost_per_1m: yuanToCents(priceForm.output_cost_per_1m),
    request_cost: yuanToCents(priceForm.request_cost),
    image_cost: yuanToCents(priceForm.image_cost),
    video_cost_per_second: yuanToCents(priceForm.video_cost_per_second),
    endpoint_id: priceForm.endpoint_id || '',
    effective_from: nowTimestamp()
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
  <div class="grid grid-cols-1 xl:grid-cols-[420px_minmax(0,1fr)] gap-4">
    <section class="panel">
      <div class="section-head">
        <div>
          <h3>服务商</h3>
          <p>厂商名称、协议和启停状态</p>
        </div>
        <div class="flex gap-2">
          <el-button :icon="Refresh" circle @click="fetchProviders" />
          <el-button type="primary" :icon="Plus" @click="openProviderDialog">新增</el-button>
        </div>
      </div>

      <el-table
        v-loading="loading"
        :data="providers"
        border
        stripe
        highlight-current-row
        class="w-full"
        @current-change="selectProvider"
      >
        <el-table-column label="厂商" min-width="180">
          <template #default="{ row }">
            <button class="provider-link" type="button" @click="selectProvider(row)">
              <span>{{ row.name }}</span>
              <small>{{ row.code }}</small>
            </button>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="130" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" :icon="Edit" @click="openProviderEditDialog(row)">编辑</el-button>
            <el-button link :type="row.status === 'active' ? 'warning' : 'success'" @click="toggleProvider(row)">
              {{ row.status === 'active' ? '禁用' : '启用' }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <section class="panel">
      <div class="section-head">
        <div>
          <h3>接入点</h3>
          <p>{{ selectedProvider?.name || '请选择服务商' }}</p>
        </div>
        <el-button type="primary" :icon="Plus" @click="openEndpointDialog">新增接入点</el-button>
      </div>

      <el-table v-loading="endpointLoading" :data="endpoints" border stripe class="w-full">
        <el-table-column prop="name" label="名称" min-width="160" />
        <el-table-column prop="base_url" label="Base URL" min-width="260" show-overflow-tooltip />
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

    <section class="panel xl:col-span-2">
      <div class="section-head">
        <div>
          <h3>Provider 成本价</h3>
          <p>{{ selectedProvider?.name || '请选择服务商' }}</p>
        </div>
        <div class="flex gap-2">
          <el-button :icon="Refresh" circle @click="fetchProviderPrices" />
          <el-button type="primary" :icon="Plus" @click="openPriceDialog">新增成本价</el-button>
        </div>
      </div>

      <el-table v-loading="priceLoading" :data="providerPrices" border stripe class="w-full">
        <el-table-column prop="upstream_model" label="上游模型" min-width="160" />
        <el-table-column prop="capability_type" label="能力" width="90" />
        <el-table-column prop="endpoint_id" label="接入点" min-width="220" show-overflow-tooltip />
        <el-table-column label="输入/1M(元)" width="130" align="right">
          <template #default="{ row }">{{ formatYuan(row.input_cost_per_1m) }}</template>
        </el-table-column>
        <el-table-column label="输出/1M(元)" width="130" align="right">
          <template #default="{ row }">{{ formatYuan(row.output_cost_per_1m) }}</template>
        </el-table-column>
        <el-table-column label="请求(元)" width="100" align="right">
          <template #default="{ row }">{{ formatYuan(row.request_cost) }}</template>
        </el-table-column>
        <el-table-column label="单图(元)" width="100" align="right">
          <template #default="{ row }">{{ formatYuan(row.image_cost) }}</template>
        </el-table-column>
        <el-table-column label="视频/秒(元)" width="120" align="right">
          <template #default="{ row }">{{ formatYuan(row.video_cost_per_second) }}</template>
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
            <el-input v-model="providerForm.provider_type" placeholder="custom" />
          </el-form-item>
          <el-form-item label="请求协议">
            <el-select v-model="providerForm.protocol_type" class="w-full">
              <el-option v-for="item in protocolOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
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
            <el-input-number v-model="endpointForm.weight" :min="0" class="w-full" />
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
          <el-form-item label="输入成本/1M(元)">
            <el-input-number v-model="priceForm.input_cost_per_1m" :min="0" class="w-full" />
          </el-form-item>
          <el-form-item label="输出成本/1M(元)">
            <el-input-number v-model="priceForm.output_cost_per_1m" :min="0" class="w-full" />
          </el-form-item>
          <el-form-item label="请求成本(元)">
            <el-input-number v-model="priceForm.request_cost" :min="0" class="w-full" />
          </el-form-item>
          <el-form-item label="单图成本(元)">
            <el-input-number v-model="priceForm.image_cost" :min="0" class="w-full" />
          </el-form-item>
          <el-form-item label="视频每秒成本(元)">
            <el-input-number v-model="priceForm.video_cost_per_second" :min="0" class="w-full" />
          </el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="priceDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitProviderPrice">{{ isEditingPrice ? '保存' : '创建' }}</el-button>
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

.provider-link {
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

.provider-link span {
  font-weight: 800;
}

.provider-link small {
  color: #94a3b8;
  font-size: 11px;
}
</style>
