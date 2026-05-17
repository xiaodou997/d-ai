<script setup>
import { computed, onMounted, shallowRef } from 'vue'
import { Refresh, Loading, Box } from '@element-plus/icons-vue'
import { listUserModelGrants } from '@/api/aiGateway'

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

const formatDate = (ts) => {
  if (!ts) return '—'
  return new Date(ts).toLocaleDateString('zh-CN')
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

      <div v-else class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        <div
          v-for="model in filteredModels"
          :key="model.id"
          class="group p-5 rounded-xl border border-slate-100 hover:border-primary-200 hover:shadow-md transition-all duration-200 bg-white"
        >
          <div class="flex items-start justify-between mb-3">
            <div class="flex-1 min-w-0">
              <h3 class="font-bold text-slate-800 truncate">{{ model.display_name }}</h3>
              <p class="text-xs text-slate-400 font-mono mt-0.5 truncate">{{ model.model_code }}</p>
            </div>
            <el-tag
              :type="capabilityColors[model.capability_type] || 'primary'"
              size="small"
              class="ml-2 shrink-0"
            >
              {{ capabilityLabels[model.capability_type] || model.capability_type }}
            </el-tag>
          </div>

          <div class="space-y-1.5 text-xs text-slate-500">
            <div v-if="model.context_window" class="flex items-center justify-between">
              <span>上下文窗口</span>
              <span class="font-semibold text-slate-700">{{ (model.context_window).toLocaleString() }} tokens</span>
            </div>
            <div v-if="model.default_max_output_tokens" class="flex items-center justify-between">
              <span>默认最大输出</span>
              <span class="font-semibold text-slate-700">{{ (model.default_max_output_tokens).toLocaleString() }} tokens</span>
            </div>
            <div v-if="model.max_output_tokens" class="flex items-center justify-between">
              <span>最大输出上限</span>
              <span class="font-semibold text-slate-700">{{ (model.max_output_tokens).toLocaleString() }} tokens</span>
            </div>
            <div class="flex items-center justify-between">
              <span>授权时间</span>
              <span class="font-semibold text-slate-600">{{ formatDate(model.granted_at) }}</span>
            </div>
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
