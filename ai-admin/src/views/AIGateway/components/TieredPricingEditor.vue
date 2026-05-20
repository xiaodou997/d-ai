<script setup>
import { computed } from 'vue'
import { ElButton, ElIcon, ElInputNumber } from 'element-plus'
import { Delete, Plus } from '@element-plus/icons-vue'

const props = defineProps({
  modelValue: {
    type: Array,
    default: () => []
  }
})
const emit = defineEmits(['update:modelValue'])

const tiers = computed({
  get: () => (props.modelValue.length ? props.modelValue : [emptyTier(null)]),
  set: (val) => emit('update:modelValue', val)
})

function emptyTier(upTo) {
  return {
    up_to: upTo,
    input_per_1m: 0,
    output_per_1m: 0,
    cache_write_per_1m: 0,
    cache_read_per_1m: 0,
    reasoning_per_1m: 0
  }
}

function updateTier(index, patch) {
  const next = tiers.value.map((t, i) => (i === index ? { ...t, ...patch } : t))
  emit('update:modelValue', next)
}

function addTier() {
  const next = [...tiers.value]
  const last = next[next.length - 1]
  // Insert a finite tier before the unbounded one
  const upTo = last?.up_to == null ? 200000 : Number(last.up_to) * 2
  next.splice(next.length - 1, 0, emptyTier(upTo))
  if (next[next.length - 1]?.up_to !== null) next.push(emptyTier(null))
  emit('update:modelValue', next)
}

function removeTier(index) {
  if (tiers.value.length <= 1) return
  const next = tiers.value.filter((_, i) => i !== index)
  // Ensure last tier remains unbounded
  if (next.length > 0) next[next.length - 1] = { ...next[next.length - 1], up_to: null }
  emit('update:modelValue', next)
}

const upperLabel = (upTo) => (upTo == null ? '无上限' : Number(upTo).toLocaleString())

const lowerBoundOf = (index) => {
  if (index === 0) return 0
  const prev = tiers.value[index - 1]
  return prev?.up_to == null ? 0 : Number(prev.up_to)
}

const rangeLabel = (index, tier) =>
  `${lowerBoundOf(index).toLocaleString()} ~ ${upperLabel(tier.up_to)}`
</script>

<template>
  <div class="tiered">
    <div v-for="(tier, idx) in tiers" :key="idx" class="tier-card">
      <div class="tier-head">
        <span class="tier-title">阶梯 {{ idx + 1 }}</span>
        <span class="tier-sub">{{ rangeLabel(idx, tier) }}</span>
        <div class="tier-bound">
          <span>上限</span>
          <el-input-number
            :model-value="tier.up_to"
            :min="1"
            :precision="0"
            :disabled="idx === tiers.length - 1"
            placeholder="∞"
            controls-position="right"
            @update:model-value="(v) => updateTier(idx, { up_to: idx === tiers.length - 1 ? null : v })"
          />
          <el-button
            v-if="tiers.length > 1"
            link
            type="danger"
            :icon="Delete"
            @click="removeTier(idx)"
          />
        </div>
      </div>
      <div class="tier-grid">
        <div class="tier-cell">
          <label>输入 (¥/M)</label>
          <el-input-number
            :model-value="tier.input_per_1m"
            :min="0"
            :precision="4"
            :step="0.1"
            controls-position="right"
            @update:model-value="(v) => updateTier(idx, { input_per_1m: v || 0 })"
          />
        </div>
        <div class="tier-cell">
          <label>输出 (¥/M)</label>
          <el-input-number
            :model-value="tier.output_per_1m"
            :min="0"
            :precision="4"
            :step="0.1"
            controls-position="right"
            @update:model-value="(v) => updateTier(idx, { output_per_1m: v || 0 })"
          />
        </div>
        <div class="tier-cell">
          <label>缓存写 (¥/M)</label>
          <el-input-number
            :model-value="tier.cache_write_per_1m"
            :min="0"
            :precision="4"
            :step="0.1"
            controls-position="right"
            @update:model-value="(v) => updateTier(idx, { cache_write_per_1m: v || 0 })"
          />
        </div>
        <div class="tier-cell">
          <label>缓存读 (¥/M)</label>
          <el-input-number
            :model-value="tier.cache_read_per_1m"
            :min="0"
            :precision="4"
            :step="0.1"
            controls-position="right"
            @update:model-value="(v) => updateTier(idx, { cache_read_per_1m: v || 0 })"
          />
        </div>
        <div class="tier-cell">
          <label>推理 (¥/M)</label>
          <el-input-number
            :model-value="tier.reasoning_per_1m"
            :min="0"
            :precision="4"
            :step="0.1"
            controls-position="right"
            @update:model-value="(v) => updateTier(idx, { reasoning_per_1m: v || 0 })"
          />
        </div>
      </div>
    </div>
    <el-button :icon="Plus" plain @click="addTier">添加价格阶梯</el-button>
  </div>
</template>

<style scoped>
.tiered {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.tier-card {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fafbfc;
  padding: 12px;
}

.tier-head {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
}

.tier-title {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

.tier-sub {
  color: #64748b;
  font-size: 12px;
}

.tier-bound {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
  color: #64748b;
  font-size: 12px;
}

.tier-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 10px;
}

.tier-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.tier-cell label {
  color: #475569;
  font-size: 11px;
  font-weight: 700;
}

.tier-cell :deep(.el-input-number) {
  width: 100%;
}
</style>
