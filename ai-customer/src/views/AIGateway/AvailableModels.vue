<script setup>
import { computed, onMounted, shallowRef } from 'vue'
import { Refresh, Loading, Box } from '@element-plus/icons-vue'
import { listUserModelGrants, formatCredits } from '@/api/aiGateway'

const loading = shallowRef(false)
const models = shallowRef([])
const filterCapability = shallowRef('')

const capabilityLabels = {
  chat: '文本对话',
  image: '生图',
  video: '视频',
  embedding: 'Embedding',
  audio_tts: '语音合成',
  audio_stt: '语音识别',
  rerank: '重排'
}

const capabilityColors = {
  chat: 'primary',
  image: 'success',
  video: 'warning',
  embedding: 'info',
  audio_tts: '',
  audio_stt: '',
  rerank: 'danger'
}

const allCapabilities = computed(() => {
  const caps = new Set(models.value.map((m) => m.capability_type).filter(Boolean))
  return [...caps]
})

const filteredModels = computed(() => {
  if (!filterCapability.value) return models.value
  return models.value.filter((m) => m.capability_type === filterCapability.value)
})

// formatCredits 已从 @/api/aiGateway 导入，后端 API 现在统一返回「积分」float

const isTokenBased = (cap) => ['chat', 'embedding', 'rerank'].includes(cap)
const isImageBased = (cap) => cap === 'image'
const isVideoBased = (cap) => cap === 'video'
const isAudioTTS = (cap) => cap === 'audio_tts'
const isAudioSTT = (cap) => cap === 'audio_stt'

const hasPrice = (model) => {
  if (isTokenBased(model.capability_type)) {
    return model.input_price_per_1m_credits > 0 || model.output_price_per_1m_credits > 0
  }
  if (isImageBased(model.capability_type)) {
    return Array.isArray(model.image_prices) && model.image_prices.length > 0
  }
  if (isVideoBased(model.capability_type)) {
    return Array.isArray(model.video_prices) && model.video_prices.length > 0
  }
  if (isAudioTTS(model.capability_type)) {
    return model.audio_tts_price_per_1m_chars_credits > 0
  }
  if (isAudioSTT(model.capability_type)) {
    return model.audio_stt_price_per_minute_credits > 0
  }
  return false
}

const fetchModels = async () => {
  loading.value = true
  try {
    const res = await listUserModelGrants()
    models.value = Array.isArray(res) ? res : []
  } finally {
    loading.value = false
  }
}

onMounted(fetchModels)
</script>

<template>
  <div class="space-y-6">
    <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 class="text-2xl font-black text-slate-800 tracking-tight">可用模型</h1>
          <p class="text-slate-400 text-sm font-medium mt-1">当前账号可调用的 AI 模型列表</p>
        </div>
        <el-button type="primary" class="rounded-2xl! font-bold" :loading="loading" @click="fetchModels">
          <template #icon><el-icon><Refresh /></el-icon></template>
          刷新
        </el-button>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft p-6">
      <div class="flex items-center gap-2 flex-wrap mb-4">
        <span class="text-xs font-bold text-slate-400 mr-1">按类型筛选：</span>
        <el-tag
          :type="filterCapability === '' ? 'primary' : 'info'"
          class="cursor-pointer"
          :effect="filterCapability === '' ? 'dark' : 'plain'"
          @click="filterCapability = ''"
        >
          全部 ({{ models.length }})
        </el-tag>
        <el-tag
          v-for="cap in allCapabilities"
          :key="cap"
          :type="filterCapability === cap ? (capabilityColors[cap] || 'primary') : 'info'"
          :effect="filterCapability === cap ? 'dark' : 'plain'"
          class="cursor-pointer"
          @click="filterCapability = cap"
        >
          {{ capabilityLabels[cap] || cap }}
        </el-tag>
      </div>

      <div v-if="loading" class="flex items-center justify-center py-16">
        <el-icon class="text-slate-300 animate-spin" :size="36"><Loading /></el-icon>
      </div>

      <div v-else-if="filteredModels.length === 0" class="flex flex-col items-center justify-center py-16 text-slate-400">
        <el-icon :size="48"><Box /></el-icon>
        <p class="mt-4 text-sm">暂无可用模型</p>
      </div>

      <div v-else class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
        <div
          v-for="model in filteredModels"
          :key="model.id"
          class="group p-4 rounded-xl border border-slate-100 hover:border-primary-200 hover:shadow-md transition-all duration-200 bg-white"
        >
          <!-- 模型名称 + 类型标签 -->
          <div class="flex items-center justify-between mb-2">
            <h3 class="font-bold text-sm text-slate-800 truncate min-w-0 flex-1 mr-2">{{ model.model_code }}</h3>
            <el-tag
              :type="capabilityColors[model.capability_type] || 'primary'"
              size="small"
              class="shrink-0"
            >
              {{ capabilityLabels[model.capability_type] || model.capability_type }}
            </el-tag>
          </div>

          <!-- 价格区域 -->
          <div v-if="hasPrice(model)" class="space-y-1">
            <!-- Token 计费模型 -->
            <template v-if="isTokenBased(model.capability_type)">
              <div class="flex items-baseline justify-between text-xs">
                <span class="text-slate-400">输入</span>
                <span class="font-semibold text-slate-700">{{ formatCredits(model.input_price_per_1m_credits) }}<span class="text-slate-400 font-normal ml-0.5">积分/M</span></span>
              </div>
              <div class="flex items-baseline justify-between text-xs">
                <span class="text-slate-400">输出</span>
                <span class="font-semibold text-slate-700">{{ formatCredits(model.output_price_per_1m_credits) }}<span class="text-slate-400 font-normal ml-0.5">积分/M</span></span>
              </div>
            </template>
            <!-- 图片计费模型 -->
            <template v-if="isImageBased(model.capability_type) && Array.isArray(model.image_prices)">
              <div v-for="(entry, idx) in model.image_prices" :key="idx" class="flex items-baseline justify-between text-xs">
                <span class="text-slate-400">{{ entry.resolution }}</span>
                <span class="font-semibold text-slate-700">{{ formatCredits(entry.price_credits) }}<span class="text-slate-400 font-normal ml-0.5">积分/张</span></span>
              </div>
            </template>
            <!-- 视频计费模型 -->
            <template v-if="isVideoBased(model.capability_type) && Array.isArray(model.video_prices)">
              <div v-for="(entry, idx) in model.video_prices" :key="idx" class="flex items-baseline justify-between text-xs">
                <span class="text-slate-400">{{ entry.resolution }}</span>
                <span class="font-semibold text-slate-700">{{ formatCredits(entry.price_credits) }}<span class="text-slate-400 font-normal ml-0.5">积分/秒</span></span>
              </div>
            </template>
            <!-- 语音合成 -->
            <template v-if="isAudioTTS(model.capability_type)">
              <div class="flex items-baseline justify-between text-xs">
                <span class="text-slate-400">TTS</span>
                <span class="font-semibold text-slate-700">{{ formatCredits(model.audio_tts_price_per_1m_chars_credits) }}<span class="text-slate-400 font-normal ml-0.5">积分/M字符</span></span>
              </div>
            </template>
            <!-- 语音识别 -->
            <template v-if="isAudioSTT(model.capability_type)">
              <div class="flex items-baseline justify-between text-xs">
                <span class="text-slate-400">STT</span>
                <span class="font-semibold text-slate-700">{{ formatCredits(model.audio_stt_price_per_minute_credits) }}<span class="text-slate-400 font-normal ml-0.5">积分/分钟</span></span>
              </div>
            </template>
          </div>
          <div v-else class="text-xs text-slate-300 text-center py-1">
            暂未定价
          </div>

        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.animate-spin {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
