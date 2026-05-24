<script setup>
import { onMounted, reactive, shallowRef, computed } from 'vue'
import { Refresh, Delete } from '@element-plus/icons-vue'
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
import ResolutionPricingEditor from './components/ResolutionPricingEditor.vue'

const loading = shallowRef(false)
const models = shallowRef([])
const userPrices = shallowRef([])
const selectedModel = shallowRef(null)
const priceLoading = shallowRef(false)
const saving = shallowRef(false)

const publicPrice = shallowRef(null)
const tenantPrice = shallowRef(null)

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

const priceForm = reactive({
  input_price_per_1m_credits: 0,
  output_price_per_1m_credits: 0,
  image_prices: [],
  video_prices: [],
  audio_tts_price_per_1m_chars_credits: 0,
  audio_stt_price_per_minute_credits: 0
})

const hasPriceSet = computed(() => Boolean(tenantPrice.value))

const cap = computed(() => selectedModel.value?.capability_type || '')
const isChatLike = computed(() => ['chat', 'embedding', 'rerank'].includes(cap.value))
const isChat = computed(() => cap.value === 'chat')
const isImage = computed(() => cap.value === 'image')
const isVideo = computed(() => cap.value === 'video')
const isAudioTTS = computed(() => cap.value === 'audio_tts')
const isAudioSTT = computed(() => cap.value === 'audio_stt')

const fetchModels = async () => {
  loading.value = true
  try {
    const [grantsRes, pricesRes] = await Promise.all([
      listTenantModelGrants(),
      listTenantUserPrices()
    ])
    models.value = grantsRes || []
    userPrices.value = pricesRes || []
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
    const [pubRes, tenantRes] = await Promise.all([
      getModelPrice(model.model_id).catch(() => null),
      getTenantUserPrice(model.model_id).catch(() => null)
    ])
    publicPrice.value = pubRes
    tenantPrice.value = tenantRes

    const src = tenantRes || pubRes
    if (src) {
      priceForm.input_price_per_1m_credits = src.input_price_per_1m_credits || 0
      priceForm.output_price_per_1m_credits = src.output_price_per_1m_credits || 0
      priceForm.image_prices = Array.isArray(src.image_prices) ? src.image_prices : []
      priceForm.video_prices = Array.isArray(src.video_prices) ? src.video_prices : []
      priceForm.audio_tts_price_per_1m_chars_credits = src.audio_tts_price_per_1m_chars_credits || 0
      priceForm.audio_stt_price_per_minute_credits = src.audio_stt_price_per_minute_credits || 0
    }
  } finally {
    priceLoading.value = false
  }
}

const resetPriceForm = () => {
  Object.assign(priceForm, {
    input_price_per_1m_credits: 0,
    output_price_per_1m_credits: 0,
    image_prices: [],
    video_prices: [],
    audio_tts_price_per_1m_chars_credits: 0,
    audio_stt_price_per_minute_credits: 0
  })
}

const savePrice = async () => {
  if (!selectedModel.value) return
  saving.value = true
  try {
    await upsertTenantUserPrice(selectedModel.value.model_id, {
      input_price_per_1m_credits: priceForm.input_price_per_1m_credits,
      output_price_per_1m_credits: priceForm.output_price_per_1m_credits,
      image_prices: priceForm.image_prices,
      video_prices: priceForm.video_prices,
      audio_tts_price_per_1m_chars_credits: priceForm.audio_tts_price_per_1m_chars_credits,
      audio_stt_price_per_minute_credits: priceForm.audio_stt_price_per_minute_credits
    })
    ElMessage.success('售价已保存')
    await fetchModels()
    tenantPrice.value = await getTenantUserPrice(selectedModel.value.model_id).catch(() => null)
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

const getUserPriceForModel = (modelId) =>
  userPrices.value.find((p) => p.model_id === modelId)

onMounted(fetchModels)
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div class="page-title">
        <p class="eyebrow">Tenant Pricing</p>
        <h1>租户定价</h1>
        <p>设置租户对外售价，未设置的模型将使用平台公价扣费用户积分</p>
      </div>
      <el-button :icon="Refresh" @click="fetchModels" :loading="loading">刷新</el-button>
    </header>

    <main class="page-main flex gap-6 min-h-0">
      <!-- 左：模型列表 -->
      <section class="model-list-panel flex-1 min-w-0">
        <el-table :data="models" v-loading="loading" stripe highlight-current-row @current-change="selectModel">
          <el-table-column prop="model_code" label="模型编码" min-width="200" />
          <el-table-column label="能力类型" min-width="100">
            <template #default="{ row }">
              <el-tag size="small">{{ capabilityLabel(row.capability_type) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="售价状态" min-width="100">
            <template #default="{ row }">
              <el-tag :type="getUserPriceForModel(row.model_id) ? 'success' : 'info'" size="small">
                {{ getUserPriceForModel(row.model_id) ? '已设置' : '未设置' }}
              </el-tag>
            </template>
          </el-table-column>
        </el-table>
      </section>

      <!-- 右：价格编辑器 -->
      <section class="price-editor-panel w-[420px]" v-if="selectedModel">
        <el-card shadow="never" class="editor-card" v-loading="priceLoading">
          <template #header>
            <div class="card-header">
              <span class="font-bold">{{ selectedModel.model_code }}</span>
              <el-tag size="small" :type="statusTagType(selectedModel.status)">
                {{ selectedModel.status }}
              </el-tag>
            </div>
          </template>

          <p class="text-xs text-slate-500 mb-4">
            {{ selectedModel.model_code }} · {{ capabilityLabel(selectedModel.capability_type) }}
          </p>

          <!-- 平台公价参考 -->
          <el-divider content-position="left">平台公价（参考）</el-divider>
          <div v-if="publicPrice" class="reference-section">
            <template v-if="isChatLike">
              <div class="ref-row">
                <span>输入</span>
                <span>{{ formatCredits(publicPrice.input_price_per_1m_credits) }} 积分/M tokens</span>
              </div>
              <div v-if="isChat" class="ref-row">
                <span>输出</span>
                <span>{{ formatCredits(publicPrice.output_price_per_1m_credits) }} 积分/M tokens</span>
              </div>
            </template>
            <template v-if="isImage">
              <div v-for="(entry, idx) in (publicPrice.image_prices || [])" :key="idx" class="ref-row">
                <span>{{ entry.resolution }}</span>
                <span>{{ formatCredits(entry.price_credits) }} 积分/张</span>
              </div>
              <p v-if="!(publicPrice.image_prices?.length)" class="text-slate-400 text-xs">暂无尺寸定价</p>
            </template>
            <template v-if="isVideo">
              <div v-for="(entry, idx) in (publicPrice.video_prices || [])" :key="idx" class="ref-row">
                <span>{{ entry.resolution }}</span>
                <span>{{ formatCredits(entry.price_credits) }} 积分/秒</span>
              </div>
              <p v-if="!(publicPrice.video_prices?.length)" class="text-slate-400 text-xs">暂无分辨率定价</p>
            </template>
            <div v-if="isAudioTTS" class="ref-row">
              <span>语音合成</span>
              <span>{{ formatCredits(publicPrice.audio_tts_price_per_1m_chars_credits) }} 积分/M 字符</span>
            </div>
            <div v-if="isAudioSTT" class="ref-row">
              <span>语音识别</span>
              <span>{{ formatCredits(publicPrice.audio_stt_price_per_minute_credits) }} 积分/分钟</span>
            </div>
          </div>
          <p v-else class="text-slate-400 text-xs mb-4">暂无平台定价</p>

          <!-- 租户售价表单 -->
          <el-divider content-position="left">
            <span>租户售价</span>
            <el-tag v-if="hasPriceSet" type="success" size="small" class="ml-2">已设置</el-tag>
          </el-divider>

          <el-form label-position="top" size="small">
            <!-- chat / embedding / rerank -->
            <template v-if="isChatLike">
              <div class="grid grid-cols-2 gap-4">
                <el-form-item label="输入（积分/百万 token）">
                  <el-input-number v-model="priceForm.input_price_per_1m_credits" :min="0" :precision="4" :step="0.0001" class="w-full" />
                </el-form-item>
                <el-form-item v-if="isChat" label="输出（积分/百万 token）">
                  <el-input-number v-model="priceForm.output_price_per_1m_credits" :min="0" :precision="4" :step="0.0001" class="w-full" />
                </el-form-item>
              </div>
            </template>

            <!-- image -->
            <el-form-item v-if="isImage" label="尺寸定价（积分/张）">
              <ResolutionPricingEditor v-model="priceForm.image_prices" mode="image" :presets="imagePresets" />
            </el-form-item>

            <!-- video -->
            <el-form-item v-if="isVideo" label="分辨率定价（积分/秒）">
              <ResolutionPricingEditor v-model="priceForm.video_prices" mode="video" :presets="videoPresets" />
            </el-form-item>

            <!-- audio TTS -->
            <el-form-item v-if="isAudioTTS" label="语音合成（积分/百万字符）">
              <el-input-number v-model="priceForm.audio_tts_price_per_1m_chars_credits" :min="0" :precision="4" :step="0.0001" class="w-full" />
            </el-form-item>

            <!-- audio STT -->
            <el-form-item v-if="isAudioSTT" label="语音识别（积分/分钟）">
              <el-input-number v-model="priceForm.audio_stt_price_per_minute_credits" :min="0" :precision="4" :step="0.0001" class="w-full" />
            </el-form-item>
          </el-form>

          <div class="action-bar">
            <el-button type="primary" @click="savePrice" :loading="saving">保存售价</el-button>
            <el-button v-if="hasPriceSet" type="danger" plain @click="deletePrice" :icon="Delete">删除售价</el-button>
          </div>

          <el-divider />
          <p class="text-xs text-slate-400">
            租户售价用于扣减用户积分。如未设置，将使用平台公价扣减用户积分。
          </p>
        </el-card>
      </section>

      <section class="price-editor-panel w-[420px] flex items-center justify-center" v-else>
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
  gap: 24px;
  padding: 24px;
}

.page-header {
  flex-shrink: 0;
  background: #fff;
  border: 1px solid #f1f5f9;
  border-radius: 16px;
  padding: 24px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  box-shadow: 0 4px 16px rgba(15, 23, 42, 0.04);
}

.page-title { display: flex; flex-direction: column; }
.eyebrow {
  margin: 0 0 4px;
  color: #64748b;
  font-size: 11px;
  font-weight: 900;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.page-title h1 { margin: 0; color: #0f172a; font-size: 22px; font-weight: 900; }
.page-title p  { margin: 4px 0 0; color: #64748b; font-size: 13px; }

.page-main {
  flex: 1;
  min-height: 0;
  display: flex;
  gap: 24px;
}

.model-list-panel {
  background: #fff;
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  box-shadow: 0 4px 16px rgba(15, 23, 42, 0.04);
  overflow: hidden;
}

.price-editor-panel {
  background: #fff;
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  box-shadow: 0 4px 16px rgba(15, 23, 42, 0.04);
  padding: 20px;
  overflow-y: auto;
}

.editor-card { border: none !important; box-shadow: none !important; }
:deep(.editor-card .el-card__header) { padding: 0 0 14px; border-bottom: 1px solid #f1f5f9; }
:deep(.editor-card .el-card__body)   { padding: 0; }

.card-header { display: flex; justify-content: space-between; align-items: center; }

.reference-section {
  background: #f8fafc;
  border-radius: 10px;
  padding: 10px 14px;
  margin-bottom: 4px;
}
.ref-row {
  display: flex;
  justify-content: space-between;
  padding: 5px 0;
  font-size: 13px;
}
.ref-row span:first-child { color: #64748b; }
.ref-row span:last-child  { color: #0f172a; font-weight: 600; }

.action-bar {
  margin-top: 16px;
  display: flex;
  gap: 12px;
}

:deep(.el-table__header th) {
  background: #f8fafc !important;
  color: #64748b;
  font-size: 11px;
  font-weight: 900;
  text-transform: uppercase;
}
</style>
