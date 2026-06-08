<script setup>
import { onMounted, shallowRef } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { formatCredits, getMyEffectivePrices } from '@/api/aiGateway'

const loading = shallowRef(false)
const prices = shallowRef([])

const CAPABILITY_LABEL = {
  chat: '对话',
  image: '图像',
  video: '视频',
  embedding: '向量',
  audio_tts: '语音合成',
  audio_stt: '语音识别',
  rerank: '重排序'
}
const capabilityLabel = (v) => CAPABILITY_LABEL[v] || v

async function load() {
  loading.value = true
  try {
    const res = await getMyEffectivePrices()
    prices.value = Array.isArray(res) ? res : []
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="p-1">
    <header class="flex items-start justify-between mb-4">
      <div>
        <h2 class="text-xl font-extrabold text-slate-800 m-0">价格</h2>
        <p class="text-sm text-slate-500 mt-1.5 m-0">你调用各模型时按以下积分单价计费（已是你的最终价格）。</p>
      </div>
      <el-button :icon="Refresh" @click="load">刷新</el-button>
    </header>

    <el-card shadow="never">
      <el-table v-loading="loading" :data="prices" size="small" stripe>
        <el-table-column prop="model_code" label="模型" min-width="200" show-overflow-tooltip />
        <el-table-column label="能力" width="100">
          <template #default="{ row }">{{ capabilityLabel(row.capability_type) }}</template>
        </el-table-column>
        <el-table-column label="输入 积分/1M token" min-width="150">
          <template #default="{ row }">{{ formatCredits(row.input_per_1m_credits) }}</template>
        </el-table-column>
        <el-table-column label="输出 积分/1M token" min-width="150">
          <template #default="{ row }">{{ formatCredits(row.output_per_1m_credits) }}</template>
        </el-table-column>
        <el-table-column label="缓存读 积分/1M token" min-width="160">
          <template #default="{ row }">{{ formatCredits(row.cache_read_per_1m_credits) }}</template>
        </el-table-column>
        <template #empty>
          <span class="text-slate-400">暂无价格（你所属租户尚未配置售价）</span>
        </template>
      </el-table>
    </el-card>
  </div>
</template>
