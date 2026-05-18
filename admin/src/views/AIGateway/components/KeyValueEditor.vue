<script setup>
import { ref, watch } from 'vue'
import { ElButton, ElInput } from 'element-plus'
import { Delete, Plus } from '@element-plus/icons-vue'

const props = defineProps({
  modelValue: { type: Object, default: () => ({}) }
})
const emit = defineEmits(['update:modelValue'])

// Local rows state. Empty-key rows are preserved while the user is typing;
// they only get filtered out on emit so they don't pollute the parent object.
const rows = ref(rowsFromObject(props.modelValue))

// Re-seed local state only when the parent object identity changes from outside
// (e.g. opening the dialog for a different deployment). Avoid feedback loops by
// not re-syncing on identity changes we caused ourselves — we compare contents.
watch(
  () => props.modelValue,
  (next) => {
    const incoming = rowsFromObject(next)
    if (sameRows(incoming, rows.value.filter((r) => r.key))) return
    rows.value = incoming
  }
)

function rowsFromObject(src) {
  if (!src || typeof src !== 'object') return []
  return Object.entries(src).map(([key, value]) => ({
    key,
    value: stringifyValue(value)
  }))
}

function sameRows(a, b) {
  if (a.length !== b.length) return false
  for (let i = 0; i < a.length; i += 1) {
    if (a[i].key !== b[i].key || a[i].value !== b[i].value) return false
  }
  return true
}

function stringifyValue(v) {
  if (v == null) return ''
  if (typeof v === 'boolean' || typeof v === 'number') return String(v)
  if (typeof v === 'string') return v
  try {
    return JSON.stringify(v)
  } catch {
    return String(v)
  }
}

function inferValue(raw) {
  if (raw === '') return ''
  if (raw === 'true') return true
  if (raw === 'false') return false
  if (raw === 'null') return null
  if (/^-?\d+(\.\d+)?$/.test(raw)) {
    const n = Number(raw)
    if (Number.isFinite(n)) return n
  }
  return raw
}

function emitObject() {
  const obj = {}
  for (const { key, value } of rows.value) {
    if (!key) continue
    obj[key] = inferValue(value)
  }
  emit('update:modelValue', obj)
}

function updateRow(index, patch) {
  rows.value[index] = { ...rows.value[index], ...patch }
  emitObject()
}

function addRow() {
  rows.value.push({ key: '', value: '' })
}

function removeRow(index) {
  rows.value.splice(index, 1)
  emitObject()
}
</script>

<template>
  <div class="kv">
    <div v-if="rows.length === 0" class="empty">无上游覆盖参数</div>
    <div v-else class="rows">
      <div class="row head">
        <span>参数名</span>
        <span>参数值</span>
        <span></span>
      </div>
      <div v-for="(row, idx) in rows" :key="idx" class="row">
        <el-input
          :model-value="row.key"
          placeholder="如 temperature"
          @update:model-value="(v) => updateRow(idx, { key: v })"
        />
        <el-input
          :model-value="row.value"
          placeholder="如 0.7 / true / json"
          @update:model-value="(v) => updateRow(idx, { value: v })"
        />
        <el-button link type="danger" :icon="Delete" @click="removeRow(idx)" />
      </div>
    </div>
    <el-button :icon="Plus" plain size="small" @click="addRow">添加参数</el-button>
  </div>
</template>

<style scoped>
.kv {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.rows {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.row {
  display: grid;
  grid-template-columns: 200px minmax(0, 1fr) 40px;
  gap: 10px;
  align-items: center;
}

.row.head {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
  padding: 0 4px;
}

.empty {
  padding: 10px;
  border: 1px dashed #cbd5e1;
  border-radius: 6px;
  color: #94a3b8;
  font-size: 12px;
  text-align: center;
}
</style>
