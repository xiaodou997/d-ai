<script setup>
import { computed } from 'vue'
import { ElButton, ElInput, ElInputNumber, ElTag } from 'element-plus'
import { Delete, Plus } from '@element-plus/icons-vue'

const props = defineProps({
  modelValue: { type: Array, default: () => [] },
  // 'image' -> 积分/张; 'video' -> 积分/秒
  mode: { type: String, default: 'image' },
  presets: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:modelValue'])

const rows = computed(() => props.modelValue || [])
const priceLabel = computed(() => (props.mode === 'video' ? '积分 (每秒)' : '积分 (每张)'))

function pushRow(resolution) {
  emit('update:modelValue', [...rows.value, { resolution, price_credits: 0 }])
}

function updateRow(index, patch) {
  emit('update:modelValue', rows.value.map((r, i) => (i === index ? { ...r, ...patch } : r)))
}

function removeRow(index) {
  emit('update:modelValue', rows.value.filter((_, i) => i !== index))
}

function applyPreset(preset) {
  const existing = new Set(rows.value.map((r) => r.resolution))
  const additions = preset.resolutions
    .filter((res) => !existing.has(res))
    .map((res) => ({ resolution: res, price_credits: 0 }))
  if (additions.length === 0) return
  emit('update:modelValue', [...rows.value, ...additions])
}
</script>

<template>
  <div class="resolutions">
    <div v-if="presets.length" class="preset-row">
      <span class="preset-label">预设：</span>
      <el-tag
        v-for="preset in presets"
        :key="preset.label"
        type="info"
        effect="plain"
        class="preset-chip"
        @click="applyPreset(preset)"
      >
        + {{ preset.label }}
      </el-tag>
      <el-button link type="primary" :icon="Plus" @click="pushRow('')">自定义</el-button>
    </div>

    <div v-if="rows.length === 0" class="empty">点击预设或"自定义"添加分辨率档位</div>

    <div v-else class="table-wrap">
      <div class="row head">
        <span>分辨率</span>
        <span>{{ priceLabel }}</span>
        <span></span>
      </div>
      <div v-for="(row, idx) in rows" :key="idx" class="row">
        <el-input
          :model-value="row.resolution"
          placeholder="如 1024x1024 / 1080p"
          @update:model-value="(v) => updateRow(idx, { resolution: v })"
        />
        <el-input-number
          :model-value="row.price_credits"
          :min="0"
          :precision="0"
          :step="1"
          controls-position="right"
          @update:model-value="(v) => updateRow(idx, { price_credits: v || 0 })"
        />
        <el-button link type="danger" :icon="Delete" @click="removeRow(idx)" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.resolutions {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: 100%;
}

.preset-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.preset-label {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}

.preset-chip { cursor: pointer; }

.empty {
  padding: 14px;
  border: 1px dashed #cbd5e1;
  border-radius: 8px;
  color: #94a3b8;
  font-size: 12px;
  text-align: center;
}

.table-wrap {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 140px 40px;
  gap: 10px;
  align-items: center;
}

.row.head {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
  padding: 0 4px;
}

.row :deep(.el-input-number) { width: 100%; }
</style>
