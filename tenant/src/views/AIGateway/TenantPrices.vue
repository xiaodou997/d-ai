<script setup>
import { onMounted, reactive, shallowRef, computed } from 'vue'
import { Refresh, Plus, Delete } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  listTenantModelGrants,
  listTenantUserPrices,
  getTenantUserPrice,
  upsertTenantUserPrice,
  deleteTenantUserPrice,
  getModelPrice,
  capabilityOptions,
  formatCredits
} from '@/api/aiGateway'

const loading = shallowRef(false)
const models = shallowRef([]) // 已授权模型
const userPrices = shallowRef([]) // 租户售价
const selectedModel = shallowRef(null)
const priceLoading = shallowRef(false)
const saving = shallowRef(false)

const publicPrice = shallowRef(null) // 平台公价（参考）
const tenantPrice = shallowRef(null) // 当前租户售价

const priceForm = reactive({
  input_price_per_1m: 0,
  output_price_per_1m: 0,
  image_size_prices: [],
  video_price_per_second: 0,
  audio_tts_price_per_1m_chars: 0,
  audio_stt_price_per_minute: 0
})

const hasPriceSet = computed(() => Boolean(tenantPrice.value))

const fetchModels = async () => {
  loading.value = true
  try {
    const [grantsRes, pricesRes] = await Promise.all([
      listTenantModelGrants(),
      listTenantUserPrices()
    ])
    models.value = grantsRes.data || []
    userPrices.value = pricesRes.data || []
  } finally {
    loading.value = false
  }
}

const selectModel = async (model) => {
  selectedModel.value = model
  priceLoading.value = true
  tenantPrice.value = null
  publicPrice.value = null
  resetPriceForm()

  try {
    // 并行获取平台公价和租户售价
    const [pubRes, tenantRes] = await Promise.all([
      getModelPrice(model.model_id).catch(() => ({ data: null })),
      getTenantUserPrice(model.model_id).catch(() => ({ data: null }))
    ])
    publicPrice.value = pubRes.data
    tenantPrice.value = tenantRes.data

    if (tenantRes.data) {
      // 已有租户售价，填充表单
      priceForm.input_price_per_1m = tenantRes.data.input_price_per_1m || 0
      priceForm.output_price_per_1m = tenantRes.data.output_price_per_1m || 0
      priceForm.video_price_per_second = tenantRes.data.video_price_per_second || 0
      priceForm.audio_tts_price_per_1m_chars = tenantRes.data.audio_tts_price_per_1m_chars || 0
      priceForm.audio_stt_price_per_minute = tenantRes.data.audio_stt_price_per_minute || 0
      priceForm.image_size_prices = parseSizePrices(tenantRes.data.image_size_prices)
    } else if (pubRes.data) {
      // 无租户售价，使用平台公价作为默认值
      priceForm.input_price_per_1m = pubRes.data.input_price_per_1m || 0
      priceForm.output_price_per_1m = pubRes.data.output_price_per_1m || 0
      priceForm.video_price_per_second = pubRes.data.video_price_per_second || 0
      priceForm.audio_tts_price_per_1m_chars = pubRes.data.audio_tts_price_per_1m_chars || 0
      priceForm.audio_stt_price_per_minute = pubRes.data.audio_stt_price_per_minute || 0
      priceForm.image_size_prices = parseSizePrices(pubRes.data.image_size_prices)
    }
  } finally {
    priceLoading.value = false
  }
}

const resetPriceForm = () => {
  Object.assign(priceForm, {
    input_price_per_1m: 0,
    output_price_per_1m: 0,
    image_size_prices: [],
    video_price_per_second: 0,
    audio_tts_price_per_1m_chars: 0,
    audio_stt_price_per_minute: 0
  })
}

const parseSizePrices = (raw) => {
  if (!raw) return []
  try {
    const obj = typeof raw === 'string' ? JSON.parse(raw) : raw
    return Object.entries(obj).map(([size, price]) => ({ size, price: Number(price) }))
  } catch {
    return []
  }
}

const serializeSizePrices = () => {
  const obj = {}
  for (const { size, price } of priceForm.image_size_prices) {
    if (size && size.trim()) {
      obj[size.trim()] = Number(price) || 0
    }
  }
  return obj
}

const addSizePrice = () => {
  priceForm.image_size_prices.push({ size: '', price: 0 })
}

const removeSizePrice = (idx) => {
  priceForm.image_size_prices.splice(idx, 1)
}

const savePrice = async () => {
  if (!selectedModel.value) return
  saving.value = true
  try {
    await upsertTenantUserPrice(selectedModel.value.model_id, {
      input_price_per_1m: priceForm.input_price_per_1m,
      output_price_per_1m: priceForm.output_price_per_1m,
      image_size_prices: serializeSizePrices(),
      video_price_per_second: priceForm.video_price_per_second,
      audio_tts_price_per_1m_chars: priceForm.audio_tts_price_per_1m_chars,
      audio_stt_price_per_minute: priceForm.audio_stt_price_per_minute
    })
    ElMessage.success('售价已保存')
    await fetchModels()
    // 重新加载当前模型的售价
    const tenantRes = await getTenantUserPrice(selectedModel.value.model_id).catch(() => ({ data: null }))
    tenantPrice.value = tenantRes.data
  } finally {
    saving.value = false
  }
}

const deletePrice = async () => {
  if (!selectedModel.value || !tenantPrice.value) return
  try {
    await ElMessageBox.confirm('确定删除此模型的售价设置？删除后将使用平台公价扣费', '提示', {
      confirmButtonText: '确定删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await deleteTenantUserPrice(selectedModel.value.model_id)
    ElMessage.success('售价已删除')
    tenantPrice.value = null
    resetPriceForm()
    await fetchModels()
  } catch {}
}

const capabilityLabel = (value) =>
  capabilityOptions.find((o) => o.value === value)?.label || value || '-'

const statusTagType = (status) => {
  const map = { active: 'success', inactive: 'warning', disabled: 'danger' }
  return map[status] || 'info'
}

const getUserPriceForModel = (modelId) => {
  return userPrices.value.find((p) => p.model_id === modelId)
}

onMounted(fetchModels)
</script>

<template>
  <div class="page-container">
    <!-- Header -->
    <header class="page-header">
      <div class="page-title">
        <p class="eyebrow">Tenant Pricing</p>
        <h1>租户定价</h1>
        <p>设置租户对外售价，未设置的模型将使用平台公价扣费用户积分</p>
      </div>
      <el-button :icon="Refresh" @click="fetchModels" :loading="loading">刷新</el-button>
    </header>

    <!-- Content -->
    <main class="page-main flex gap-6 min-h-0">
      <!-- Left: Model List -->
      <section class="model-list-panel flex-1 min-w-0">
        <el-table :data="models" v-loading="loading" stripe highlight-current-row @current-change="selectModel">
          <el-table-column prop="model_code" label="模型编码" min-width="140" />
          <el-table-column prop="display_name" label="显示名称" min-width="160" />
          <el-table-column label="能力类型" min-width="100">
            <template #default="{ row }">
              <el-tag size="small">{{ capabilityLabel(row.capability_type) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="售价状态" min-width="100">
            <template #default="{ row }">
              <template v-if="getUserPriceForModel(row.model_id)">
                <el-tag type="success" size="small">已设置</el-tag>
              </template>
              <template v-else>
                <el-tag type="info" size="small">未设置</el-tag>
              </template>
            </template>
          </el-table-column>
        </el-table>
      </section>

      <!-- Right: Price Editor -->
      <section class="price-editor-panel w-96" v-if="selectedModel">
        <el-card shadow="never" class="editor-card" v-loading="priceLoading">
          <template #header>
            <div class="card-header">
              <span class="font-bold">{{ selectedModel.display_name }}</span>
              <el-tag size="small" :type="statusTagType(selectedModel.status)">
                {{ selectedModel.status }}
              </el-tag>
            </div>
          </template>

          <p class="text-xs text-slate-500 mb-4">
            模型编码: {{ selectedModel.model_code }} · {{ capabilityLabel(selectedModel.capability_type) }}
          </p>

          <!-- Platform Price Reference -->
          <el-divider content-position="left">平台公价（参考）</el-divider>
          <div v-if="publicPrice" class="reference-section">
            <div class="ref-row">
              <span>输入</span>
              <span>{{ formatCredits(publicPrice.input_price_per_1m) }} 积分/M</span>
            </div>
            <div class="ref-row">
              <span>输出</span>
              <span>{{ formatCredits(publicPrice.output_price_per_1m) }} 积分/M</span>
            </div>
          </div>
          <p v-else class="text-slate-400 text-xs">暂无平台定价</p>

          <!-- Tenant Price Form -->
          <el-divider content-position="left">
            <span>租户售价</span>
            <el-tag v-if="hasPriceSet" type="success" size="small" class="ml-2">已设置</el-tag>
          </el-divider>

          <el-form label-position="top" size="small">
            <div class="grid grid-cols-2 gap-4">
              <el-form-item label="输入价格（积分/百万token）">
                <el-input-number v-model="priceForm.input_price_per_1m" :min="0" :precision="0" class="w-full" />
              </el-form-item>
              <el-form-item label="输出价格（积分/百万token）">
                <el-input-number v-model="priceForm.output_price_per_1m" :min="0" :precision="0" class="w-full" />
              </el-form-item>
            </div>

            <el-form-item label="图片尺寸价格">
              <div class="size-price-list">
                <div v-for="(item, idx) in priceForm.image_size_prices" :key="idx" class="size-price-row">
                  <el-input v-model="item.size" placeholder="尺寸如 1024x1024" class="flex-1" />
                  <el-input-number v-model="item.price" :min="0" :precision="0" class="w-24" />
                  <el-button :icon="Delete" link type="danger" @click="removeSizePrice(idx)" />
                </div>
                <el-button :icon="Plus" link type="primary" @click="addSizePrice">添加尺寸</el-button>
              </div>
            </el-form-item>

            <div class="grid grid-cols-2 gap-4">
              <el-form-item label="视频价格（积分/秒）">
                <el-input-number v-model="priceForm.video_price_per_second" :min="0" :precision="0" class="w-full" />
              </el-form-item>
              <el-form-item label="语音合成（积分/百万字符）">
                <el-input-number v-model="priceForm.audio_tts_price_per_1m_chars" :min="0" :precision="0" class="w-full" />
              </el-form-item>
            </div>
            <el-form-item label="语音识别（积分/分钟）">
              <el-input-number v-model="priceForm.audio_stt_price_per_minute" :min="0" :precision="0" class="w-full" />
            </el-form-item>
          </el-form>

          <!-- Actions -->
          <div class="action-bar">
            <el-button type="primary" @click="savePrice" :loading="saving">保存售价</el-button>
            <el-button v-if="hasPriceSet" type="danger" plain @click="deletePrice">删除售价</el-button>
          </div>

          <el-divider />

          <p class="text-xs text-slate-400">
            租户售价用于扣减用户积分。如未设置，将使用平台公价扣减用户积分。
          </p>
        </el-card>
      </section>

      <!-- Empty State -->
      <section class="price-editor-panel w-96 flex items-center justify-center" v-else>
        <p class="text-slate-400 text-sm">选择左侧模型设置售价</p>
      </section>
    </main>
  </div>
</template>

<style scoped>
.page-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: #f5f7fa;
}

.page-header {
  padding: 16px 24px;
  background: #ffffff;
  border-bottom: 1px solid #e4e7ed;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.page-title {
  display: flex;
  flex-direction: column;
}

.eyebrow {
  margin: 0 0 4px;
  color: #64748b;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0;
  text-transform: uppercase;
}

.page-title h1 {
  margin: 0;
  color: #111827;
  font-size: 22px;
  font-weight: 900;
}

.page-title p {
  margin: 4px 0 0;
  color: #64748b;
  font-size: 13px;
}

.page-main {
  padding: 24px;
  flex: 1;
  min-height: 0;
}

.model-list-panel {
  background: #ffffff;
  border-radius: 8px;
  padding: 16px;
}

.price-editor-panel {
  background: #ffffff;
  border-radius: 8px;
  padding: 16px;
}

.editor-card {
  height: 100%;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.reference-section {
  padding: 8px 0;
}

.ref-row {
  display: flex;
  justify-content: space-between;
  padding: 4px 0;
  font-size: 13px;
  color: #909399;
}

.size-price-list {
  width: 100%;
}

.size-price-row {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
  align-items: center;
}

.action-bar {
  margin-top: 16px;
  display: flex;
  gap: 12px;
}
</style>