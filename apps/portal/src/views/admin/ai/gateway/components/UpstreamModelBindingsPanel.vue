<script setup lang="ts">
import { computed, reactive, shallowRef, useTemplateRef, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { TableInstance } from 'element-plus'
import { Delete, Plus, Refresh } from '@element-plus/icons-vue'
import { aiAdminApi } from '../../../../api/aiAdmin'
import { useModelBindingBatchDelete } from '../../../../features/ai/upstream-model-bindings'
import {
  bindingFormatGroups,
  bindingFormatValue,
  capabilityOptions,
  DEFAULT_OPENAI_BINDING_PROTOCOL,
  OTHER_CAPABILITIES_VALUE,
  OTHER_CAPABILITY_TYPES,
  protocolOptions,
  statusOptions
} from '../constants'
import type {
  DiscoveredUpstreamModelDTO,
  UpstreamModelBindingDTO,
  UpstreamModelBindingWriteRequest
} from '../../../../types/ai'

interface ImportOption {
  id: string
  label: string
  capabilityType?: string
  apiFormat?: string
  exists: boolean
}

type BindingForm = Required<UpstreamModelBindingWriteRequest>

const props = withDefaults(defineProps<{
  targetKind: 'account' | 'pool'
  targetId: string
  defaultBindingProtocol?: string
  lockedApiFormat?: string
  title?: string
  description?: string
  emptyText?: string
  importButtonLabel?: string
  importDialogTitle?: string
  importAlertTitle?: string
}>(), {
  defaultBindingProtocol: DEFAULT_OPENAI_BINDING_PROTOCOL,
  lockedApiFormat: undefined,
  title: '显式模型绑定',
  description: '显式绑定直接落到 ai_upstream_models，可独立控制 API 格式和上游模型 ID。',
  emptyText: '暂无显式模型绑定。',
  importButtonLabel: '导入模型',
  importDialogTitle: '导入模型',
  importAlertTitle: '从预设或上游发现列表里勾选后创建显式绑定，导入协议按模型自动推断。'
})

const loading = shallowRef(false)
const bindings = shallowRef<UpstreamModelBindingDTO[]>([])
const bindingTableRef = useTemplateRef<TableInstance>('bindingTable')
const dialogVisible = shallowRef(false)
const editingBindingId = shallowRef('')
const submitting = shallowRef(false)
const importDialogVisible = shallowRef(false)
const importLoading = shallowRef(false)
const importItems = shallowRef<ImportOption[]>([])
const importSelected = shallowRef<string[]>([])
const importKeyword = shallowRef('')
const importCapabilityFilter = shallowRef('')
const importSourceProtocolFilter = shallowRef('')
const importOnlyAvailable = shallowRef(true)

const {
  clearSelection: clearBatchSelection,
  deleteSelection: deleteSelectedBindings,
  deleting: batchDeleting,
  selectedCount,
  setSelection: setBatchSelection
} = useModelBindingBatchDelete({
  targetKind: () => props.targetKind,
  targetId: () => props.targetId,
  reload: fetchBindings
})

const form = reactive<BindingForm>({
  model_code: '',
  capability_type: 'chat',
  api_format: props.defaultBindingProtocol,
  upstream_model_name: '',
  status: 'active',
  image_stream_mode: 'force_sync',
  image_edit_transport: 'multipart/form-data',
  image_upstream_response_format: '',
  image_max_output_count: 1,
  image_edit_max_output_count: 1
})
// comboTouched=true 后，模型标识变化不再触发自动推断覆盖用户已手选的 API 格式。
const comboTouched = shallowRef(false)
const inferringCapability = shallowRef(false)

const isEditing = computed(() => Boolean(editingBindingId.value))

// ── 能力类型 + API 格式合并选择器 ──────────────────────────────────────────────
// chat/image/embedding 这三个能力用一份"选一项即合法"的分组下拉；video/audio_tts/
// audio_stt/rerank 目前后端不限制协议，走"其它能力"入口展开原始双选兜底。
const otherCapabilityOptions = capabilityOptions.filter((item) => OTHER_CAPABILITY_TYPES.includes(item.value))
const isOtherCapability = computed(() => OTHER_CAPABILITY_TYPES.includes(form.capability_type))
const filteredBindingFormatGroups = computed(() => {
  if (!props.lockedApiFormat) return bindingFormatGroups
  return bindingFormatGroups
    .map((group) => ({ ...group, options: group.options.filter((o) => o.api_format === props.lockedApiFormat) }))
    .filter((group) => group.options.length > 0)
})
const bindingFormatModel = computed<string>({
  get() {
    if (isOtherCapability.value) return OTHER_CAPABILITIES_VALUE
    return bindingFormatValue(form.capability_type, form.api_format)
  },
  set(value: string) {
    comboTouched.value = true
    if (value === OTHER_CAPABILITIES_VALUE) {
      if (!isOtherCapability.value) {
        form.capability_type = OTHER_CAPABILITY_TYPES[0]
        form.api_format = protocolOptions[0].value
      }
      return
    }
    const option = filteredBindingFormatGroups.value.flatMap((group) => group.options).find((o) => o.value === value)
    if (!option) return
    form.capability_type = option.capability_type
    form.api_format = option.api_format
  }
})

function endpointProtocolForInfer(): string {
  switch (props.defaultBindingProtocol) {
    case 'anthropic_messages':
      return 'anthropic'
    case 'gemini_generate':
      return 'gemini'
    default:
      return 'openai_compatible'
  }
}

async function handleModelCodeBlur() {
  if (isEditing.value || comboTouched.value) return
  const modelCode = form.model_code.trim()
  if (!modelCode) return
  inferringCapability.value = true
  try {
    const result = await aiAdminApi.inferModelCapability(modelCode, endpointProtocolForInfer())
    if (comboTouched.value) return // 推断请求期间用户已手动选择，不覆盖
    form.capability_type = result.capability_type
    form.api_format = result.api_format
  } catch {
    // 推断只是默认值建议，失败静默忽略，不影响手动选择
  } finally {
    inferringCapability.value = false
  }
}
const filteredImportItems = computed(() => {
  const keyword = importKeyword.value.trim().toLowerCase()
  return importItems.value.filter((item) => {
    if (importOnlyAvailable.value && item.exists) return false
    if (importCapabilityFilter.value && item.capabilityType !== importCapabilityFilter.value) return false
    if (importSourceProtocolFilter.value && item.apiFormat !== importSourceProtocolFilter.value) return false
    if (!keyword) return true
    return [
      item.id,
      item.label,
      item.capabilityType || '',
      item.apiFormat || ''
    ].some((value) => value.toLowerCase().includes(keyword))
  })
})
const importableFilteredItems = computed(() => filteredImportItems.value.filter((item) => !item.exists))
const importSourceProtocolOptions = computed(() => {
  const protocols = new Set(importItems.value.map((item) => item.apiFormat).filter(Boolean) as string[])
  return [...protocols].sort().map((value) => ({ value, label: protocolLabel(value) }))
})
const importSelectedCount = computed(() => importSelected.value.length)
const bindingCountLabel = computed(() => `${bindings.value.length} 条显式绑定`)
const imageStreamModeOptions = computed(() => {
  const options = [
    { label: '非流式', value: 'force_sync' },
    { label: '流式', value: 'force_stream' }
  ]
  if (form.image_stream_mode === 'auto') {
    options.push({ label: '自动（旧配置）', value: 'auto' })
  }
  return options
})
const imageEditTransportOptions = [
  { label: 'Multipart 文件上传', value: 'multipart/form-data' },
  { label: 'JSON 图片 URL', value: 'application/json' }
]
const imageUpstreamResponseFormatOptions = [
  { label: '未设置（不传给上游）', value: '' },
  { label: 'URL', value: 'url' },
  { label: 'Base64', value: 'b64_json' }
]

function protocolLabel(value: string) {
  return protocolOptions.find((item) => item.value === value)?.label || value || '-'
}

function capabilityLabel(value: string) {
  return capabilityOptions.find((item) => item.value === value)?.label || value || '-'
}

function imageStreamModeLabel(value?: string) {
  switch (value) {
    case 'force_stream': return '流式'
    case 'force_sync': return '非流式'
    case 'auto': return '自动（旧配置）'
    default: return value || '非流式'
  }
}

function imageEditTransportLabel(value?: string) {
  switch (value) {
    case 'application/json': return 'JSON 图片 URL'
    case 'multipart/form-data': return 'Multipart 文件上传'
    default: return value || 'Multipart 文件上传'
  }
}

function imageUpstreamResponseFormatLabel(value?: string) {
  switch (value) {
    case 'url': return 'URL'
    case 'b64_json': return 'Base64'
    default: return '未设置'
  }
}

function imagePolicySummary(row: UpstreamModelBindingDTO) {
  return [
    imageStreamModeLabel(row.image_stream_mode),
    imageEditTransportLabel(row.image_edit_transport),
    `上游返回 ${imageUpstreamResponseFormatLabel(row.image_upstream_response_format)}`,
    `文生图 ${row.image_max_output_count || 1} 张`,
    `图生图 ${row.image_edit_max_output_count || 1} 张`
  ].join(' / ')
}

async function toggleBindingStatus(row: UpstreamModelBindingDTO) {
  const next = row.status === 'active' ? 'disabled' : 'active'
  const payload: UpstreamModelBindingWriteRequest = {
    model_code: row.model_code,
    capability_type: row.capability_type,
    api_format: row.api_format,
    upstream_model_name: row.upstream_model_name || undefined,
    status: next,
    image_stream_mode: row.image_stream_mode || 'force_sync',
    image_edit_transport: row.image_edit_transport || 'multipart/form-data',
    image_upstream_response_format: row.image_upstream_response_format || '',
    image_max_output_count: row.image_max_output_count || 1,
    image_edit_max_output_count: row.image_edit_max_output_count || 1
  }
  try {
    if (props.targetKind === 'account') {
      await aiAdminApi.updateAccountModelBinding(props.targetId, row.id, payload)
    } else {
      await aiAdminApi.updatePoolModelBinding(props.targetId, row.id, payload)
    }
    ElMessage.success('状态已更新')
    await fetchBindings()
  } catch (error: any) {
    ElMessage.error(error?.message || '更新失败')
  }
}

function isImportSelected(id: string) {
  return importSelected.value.includes(id)
}

function toggleImportItem(item: ImportOption, checked: boolean) {
  if (item.exists) return
  if (checked) {
    if (!importSelected.value.includes(item.id)) {
      importSelected.value = [...importSelected.value, item.id]
    }
    return
  }
  importSelected.value = importSelected.value.filter((id) => id !== item.id)
}

function selectFilteredImportItems() {
  const next = new Set(importSelected.value)
  for (const item of importableFilteredItems.value) {
    next.add(item.id)
  }
  importSelected.value = [...next]
}

function clearImportSelected() {
  importSelected.value = []
}

function resetForm() {
  editingBindingId.value = ''
  comboTouched.value = false
  inferringCapability.value = false
  form.model_code = ''
  form.capability_type = 'chat'
  form.api_format = props.defaultBindingProtocol
  form.upstream_model_name = ''
  form.status = 'active'
  form.image_stream_mode = 'force_sync'
  form.image_edit_transport = 'multipart/form-data'
  form.image_upstream_response_format = ''
  form.image_max_output_count = 1
  form.image_edit_max_output_count = 1
}

function openCreate() {
  resetForm()
  dialogVisible.value = true
}

function openEdit(row: UpstreamModelBindingDTO) {
  editingBindingId.value = row.id
  comboTouched.value = true // 编辑态不触发 model_code blur 自动推断覆盖
  inferringCapability.value = false
  form.model_code = row.model_code
  form.capability_type = row.capability_type
  form.api_format = row.api_format
  form.upstream_model_name = row.upstream_model_name
  form.status = row.status
  form.image_stream_mode = row.image_stream_mode || 'force_sync'
  form.image_edit_transport = row.image_edit_transport || 'multipart/form-data'
  form.image_upstream_response_format = row.image_upstream_response_format || ''
  form.image_max_output_count = row.image_max_output_count || 1
  form.image_edit_max_output_count = row.image_edit_max_output_count || 1
  dialogVisible.value = true
}

function buildPayload(): UpstreamModelBindingWriteRequest {
  return {
    model_code: form.model_code.trim(),
    capability_type: form.capability_type,
    api_format: form.api_format,
    upstream_model_name: form.upstream_model_name.trim() || undefined,
    status: form.status,
    image_stream_mode: form.image_stream_mode,
    image_edit_transport: form.image_edit_transport,
    image_upstream_response_format: form.image_upstream_response_format,
    image_max_output_count: form.image_max_output_count,
    image_edit_max_output_count: form.image_edit_max_output_count
  }
}

async function fetchBindings() {
  clearBindingSelection()
  if (!props.targetId) {
    bindings.value = []
    return
  }
  loading.value = true
  try {
    const res = props.targetKind === 'account'
      ? await aiAdminApi.listAccountModelBindings(props.targetId)
      : await aiAdminApi.listPoolModelBindings(props.targetId)
    bindings.value = res.items || []
  } catch (error: any) {
    ElMessage.error(error?.message || '加载显式模型绑定失败')
  } finally {
    loading.value = false
  }
}

function handleBindingSelectionChange(rows: UpstreamModelBindingDTO[]) {
  setBatchSelection(rows)
}

function clearBindingSelection() {
  bindingTableRef.value?.clearSelection()
  clearBatchSelection()
}

async function submit() {
  if (!form.model_code.trim()) {
    ElMessage.warning('请填写模型标识')
    return
  }
  submitting.value = true
  try {
    if (props.targetKind === 'account') {
      if (isEditing.value) {
        await aiAdminApi.updateAccountModelBinding(props.targetId, editingBindingId.value, buildPayload())
      } else {
        await aiAdminApi.createAccountModelBinding(props.targetId, buildPayload())
      }
    } else if (isEditing.value) {
      await aiAdminApi.updatePoolModelBinding(props.targetId, editingBindingId.value, buildPayload())
    } else {
      await aiAdminApi.createPoolModelBinding(props.targetId, buildPayload())
    }
    ElMessage.success(isEditing.value ? '显式绑定已更新' : '显式绑定已创建')
    dialogVisible.value = false
    await fetchBindings()
  } catch (error: any) {
    ElMessage.error(error?.message || '保存失败')
  } finally {
    submitting.value = false
  }
}

async function remove(row: UpstreamModelBindingDTO) {
  try {
    await ElMessageBox.confirm(`删除显式绑定「${row.model_code} -> ${row.upstream_model_name}」？`, '确认删除', { type: 'warning' })
  } catch {
    return
  }
  try {
    if (props.targetKind === 'account') {
      await aiAdminApi.deleteAccountModelBinding(props.targetId, row.id)
    } else {
      await aiAdminApi.deletePoolModelBinding(props.targetId, row.id)
    }
    ElMessage.success('已删除')
    await fetchBindings()
  } catch (error: any) {
    ElMessage.error(error?.message || '删除失败')
  }
}

function mapAccountImportItem(item: DiscoveredUpstreamModelDTO): ImportOption {
  return {
    id: item.id,
    label: item.name || item.id,
    capabilityType: item.capability_type,
    apiFormat: item.api_format,
    exists: item.exists
  }
}

async function openImport() {
  if (!props.targetId) {
    return
  }
  importDialogVisible.value = true
  importSelected.value = []
  importKeyword.value = ''
  importCapabilityFilter.value = ''
  importSourceProtocolFilter.value = ''
  importOnlyAvailable.value = true
  importItems.value = []
  importLoading.value = true
  try {
    if (props.targetKind === 'account') {
      const res = await aiAdminApi.fetchAccountUpstreamModels(props.targetId)
      importItems.value = (res.items || []).map(mapAccountImportItem)
    } else {
      const res = await aiAdminApi.getPoolAvailableModels(props.targetId)
      const existing = new Set(bindings.value.map((item) => item.model_code))
      importItems.value = (res.models || []).map((modelCode: string) => ({
        id: modelCode,
        label: modelCode,
        exists: existing.has(modelCode)
      }))
    }
  } catch (error: any) {
    ElMessage.error(error?.message || '加载可导入模型失败')
  } finally {
    importLoading.value = false
  }
}

async function importSelectedModels() {
  if (!importSelected.value.length) {
    ElMessage.warning('请选择要导入的模型')
    return
  }
  try {
    const res = props.targetKind === 'account'
      ? await aiAdminApi.importAccountUpstreamModels(props.targetId, {
        models: importSelected.value
      })
      : await aiAdminApi.importPoolAvailableModels(props.targetId, { models: importSelected.value })
    ElMessage.success(`已创建 ${res.created?.length || 0} 个显式绑定，跳过 ${res.skipped?.length || 0} 个`)
    importDialogVisible.value = false
    await fetchBindings()
  } catch (error: any) {
    ElMessage.error(error?.message || '导入失败')
  }
}

watch(
  () => props.targetId,
  async () => {
    dialogVisible.value = false
    importDialogVisible.value = false
    await fetchBindings()
  },
  { immediate: true }
)
</script>

<template>
  <section class="panel">
    <div class="panel-header">
      <div class="panel-copy">
        <span class="panel-eyebrow">模型声明</span>
        <h3 class="panel-title">{{ title }}</h3>
        <p class="panel-description">{{ description }}</p>
      </div>
      <div class="header-actions">
        <el-button size="small" :icon="Refresh" @click="openImport">{{ importButtonLabel }}</el-button>
        <el-button type="primary" size="small" :icon="Plus" @click="openCreate">新增绑定</el-button>
      </div>
    </div>

    <div class="panel-table-shell">
      <div class="panel-table-meta">
        <div>
          <p class="panel-table-label">当前绑定</p>
          <p class="panel-table-value">{{ bindingCountLabel }}</p>
        </div>
        <p class="panel-table-hint">每条绑定单独声明对外模型、上游格式与调度权重。</p>
      </div>

      <div v-if="selectedCount" class="batch-action-bar">
        <div class="batch-action-copy">
          <span class="batch-action-count">已选 {{ selectedCount }} 条</span>
          <span class="batch-action-hint">删除后，相关模型将停止通过当前上游路由。</span>
        </div>
        <div class="batch-action-buttons">
          <el-button size="small" :disabled="batchDeleting" @click="clearBindingSelection">取消选择</el-button>
          <el-button
            data-test="batch-delete-bindings"
            type="danger"
            size="small"
            :icon="Delete"
            :loading="batchDeleting"
            @click="deleteSelectedBindings"
          >
            删除所选
          </el-button>
        </div>
      </div>

      <el-table
        ref="bindingTable"
        v-loading="loading"
        :data="bindings"
        row-key="id"
        border
        stripe
        class="w-full binding-table"
        @selection-change="handleBindingSelectionChange"
      >
        <el-table-column type="selection" width="48" />
        <el-table-column prop="model_code" label="模型标识" min-width="180" show-overflow-tooltip />
        <el-table-column label="能力" width="110">
          <template #default="{ row }">{{ capabilityLabel(row.capability_type) }}</template>
        </el-table-column>
        <el-table-column label="API 格式" min-width="170" show-overflow-tooltip>
          <template #default="{ row }">{{ protocolLabel(row.api_format) }}</template>
        </el-table-column>
        <el-table-column prop="upstream_model_name" label="上游模型 ID" min-width="180" show-overflow-tooltip />
        <el-table-column label="生图策略" min-width="210" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.capability_type === 'image'">
              {{ imagePolicySummary(row) }}
            </span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-switch
              :model-value="row.status === 'active'"
              inline-prompt
              active-text="启用"
              inactive-text="停用"
              size="small"
              @change="toggleBindingStatus(row)"
            />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
        <template #empty><span class="empty-text">{{ emptyText }}</span></template>
      </el-table>
    </div>

    <el-dialog v-model="dialogVisible" :title="isEditing ? '编辑显式绑定' : '新增显式绑定'" width="620px">
      <el-form label-width="128px">
        <el-form-item label="模型标识" required>
          <el-input v-model="form.model_code" placeholder="如 gpt-5.4 / claude-opus-4-1" @blur="handleModelCodeBlur" />
          <p v-if="inferringCapability" class="field-hint">正在按模型标识推断能力与 API 格式…</p>
        </el-form-item>
        <el-form-item label="API 格式" required>
          <el-select v-model="bindingFormatModel" class="w-full">
            <el-option-group v-for="group in filteredBindingFormatGroups" :key="group.capability_type" :label="group.label">
              <el-option v-for="opt in group.options" :key="opt.value" :label="opt.label" :value="opt.value" />
            </el-option-group>
            <el-option v-if="!lockedApiFormat" :value="OTHER_CAPABILITIES_VALUE" label="其它能力（视频/语音合成/语音识别/重排）" />
          </el-select>
          <p class="field-hint">输入模型标识后会自动带出建议组合，仍可在此改选其它合法组合。</p>
        </el-form-item>
        <template v-if="isOtherCapability">
          <el-form-item label="能力类型" required>
            <el-select v-model="form.capability_type" class="w-full">
              <el-option v-for="item in otherCapabilityOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="API 格式" required>
            <el-select v-model="form.api_format" class="w-full">
              <el-option v-for="item in protocolOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
        </template>
        <el-form-item label="上游模型 ID">
          <el-input v-model="form.upstream_model_name" placeholder="留空时默认同模型标识" />
        </el-form-item>
        <template v-if="form.capability_type === 'image'">
          <el-form-item label="生图流式策略">
            <el-select v-model="form.image_stream_mode" class="w-full">
              <el-option v-for="item in imageStreamModeOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="图生图传输方式">
            <el-select v-model="form.image_edit_transport" class="w-full">
              <el-option v-for="item in imageEditTransportOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="上游图片返回格式">
            <el-select v-model="form.image_upstream_response_format" class="w-full">
              <el-option v-for="item in imageUpstreamResponseFormatOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
            <p class="field-hint">未设置时不向上游发送 response_format。</p>
          </el-form-item>
          <el-form-item label="文生图最大张数">
            <el-input-number v-model="form.image_max_output_count" :min="1" :max="10" controls-position="right" />
          </el-form-item>
          <el-form-item label="图生图最大张数">
            <el-input-number v-model="form.image_edit_max_output_count" :min="1" :max="10" controls-position="right" />
          </el-form-item>
        </template>
        <el-form-item label="状态">
          <el-radio-group v-model="form.status">
            <el-radio v-for="item in statusOptions" :key="item.value" :value="item.value">{{ item.label }}</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="importDialogVisible" :title="importDialogTitle" width="760px">
      <div v-loading="importLoading">
        <el-alert type="info" :closable="false" class="mb-2" :title="importAlertTitle" />
        <div class="import-toolbar">
          <el-input v-model="importKeyword" clearable placeholder="搜索模型名 / 能力 / API 格式" />
          <el-select v-model="importCapabilityFilter" clearable placeholder="全部能力">
            <el-option v-for="item in capabilityOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
          <el-select v-model="importSourceProtocolFilter" clearable placeholder="全部来源格式">
            <el-option v-for="item in importSourceProtocolOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </div>
        <div class="import-options-bar">
          <el-checkbox v-model="importOnlyAvailable">只看未导入</el-checkbox>
          <span class="import-count">已选 {{ importSelectedCount }} / 当前可选 {{ importableFilteredItems.length }}</span>
          <div class="import-actions">
            <el-button size="small" @click="selectFilteredImportItems">全选当前筛选</el-button>
            <el-button size="small" @click="clearImportSelected">清空选择</el-button>
          </div>
        </div>
        <div class="import-list">
          <div
            v-for="item in filteredImportItems"
            :key="item.id"
            class="import-row"
            :class="{ disabled: item.exists, selected: isImportSelected(item.id) }"
            @click="toggleImportItem(item, !isImportSelected(item.id))"
          >
            <el-checkbox
              :model-value="isImportSelected(item.id)"
              :disabled="item.exists"
              @update:model-value="(checked: any) => toggleImportItem(item, Boolean(checked))"
              @click.stop
            />
            <span class="import-row-main">
              <span class="import-model">{{ item.id }}</span>
              <span class="import-meta">
                <el-tag v-if="item.capabilityType" size="small" effect="plain">{{ capabilityLabel(item.capabilityType) }}</el-tag>
                <el-tag v-if="item.apiFormat" size="small" type="info" effect="plain">{{ protocolLabel(item.apiFormat) }}</el-tag>
                <el-tag v-if="item.exists" size="small" type="warning" effect="plain">已存在</el-tag>
              </span>
            </span>
          </div>
          <el-empty v-if="!filteredImportItems.length && !importLoading" description="没有匹配的模型" :image-size="60" />
        </div>
      </div>
      <template #footer>
        <el-button @click="importDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="importSelectedModels">导入所选</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.panel {
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 20px;
  border: 1px solid #d8e1eb;
  border-radius: 24px;
  background:
    radial-gradient(circle at top left, rgba(14, 165, 233, 0.12), transparent 28%),
    linear-gradient(180deg, #ffffff 0%, #f8fbff 100%);
  box-shadow: 0 20px 50px rgba(15, 23, 42, 0.08);
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 20px;
}

.panel-copy {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-width: 720px;
}

.panel-eyebrow {
  display: inline-flex;
  width: fit-content;
  padding: 4px 10px;
  border-radius: 999px;
  background: rgba(15, 118, 110, 0.10);
  color: #0f766e;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.12em;
}

.panel-title {
  margin: 0;
  color: #0f172a;
  font-size: 22px;
  font-weight: 900;
  line-height: 1.2;
}

.panel-description {
  margin: 0;
  max-width: 680px;
  color: #64748b;
  font-size: 14px;
  line-height: 1.65;
}

.header-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
  align-self: center;
}

.panel-table-shell {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 16px;
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.88);
  backdrop-filter: blur(10px);
}

.panel-table-meta {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  padding-bottom: 2px;
}

.panel-table-label {
  margin: 0 0 4px;
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
}

.panel-table-value {
  margin: 0;
  color: #0f172a;
  font-size: 18px;
  font-weight: 800;
}

.panel-table-hint {
  margin: 0;
  max-width: 360px;
  color: #94a3b8;
  font-size: 12px;
  line-height: 1.5;
}

.batch-action-bar {
  display: flex;
  min-height: 46px;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 9px 12px;
  border: 1px solid color-mix(in srgb, var(--ds-danger) 22%, transparent);
  border-left: 3px solid var(--ds-danger);
  border-radius: var(--ds-radius-control);
  background: var(--ds-danger-soft);
}

.batch-action-copy,
.batch-action-buttons {
  display: flex;
  align-items: center;
  gap: 10px;
}

.batch-action-count {
  color: var(--ds-danger);
  font-size: 13px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.batch-action-hint {
  color: var(--ds-muted);
  font-size: 12px;
}

.w-full {
  width: 100%;
}

.mb-2 {
  margin-bottom: 8px;
}

.field-hint {
  margin: 6px 0 0;
  color: #94a3b8;
  font-size: 12px;
  line-height: 1.4;
}

.import-toolbar {
  display: grid;
  grid-template-columns: minmax(220px, 1fr) 140px 170px;
  gap: 10px;
  margin-bottom: 10px;
}

.import-options-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
  color: #64748b;
  font-size: 13px;
}

.import-count {
  font-variant-numeric: tabular-nums;
}

.import-actions {
  display: flex;
  gap: 8px;
  margin-left: auto;
}

.import-list {
  display: grid;
  max-height: 420px;
  overflow-y: auto;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
}

.import-row {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  min-height: 54px;
  padding: 9px 12px;
  cursor: pointer;
  border-bottom: 1px solid #f1f5f9;
  background: #ffffff;
}

.import-row:last-child {
  border-bottom: 0;
}

.import-row:hover {
  background: #f8fafc;
}

.import-row.selected {
  background: #f5f3ff;
}

.import-row.disabled {
  cursor: not-allowed;
  color: #94a3b8;
  background: #f8fafc;
}

.import-row-main {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.import-model {
  min-width: 0;
  overflow: hidden;
  color: #0f172a;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.import-meta {
  display: flex;
  flex: 0 0 auto;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 6px;
}

.empty-text {
  color: #94a3b8;
}

@media (max-width: 720px) {
  .panel {
    padding: 16px;
    border-radius: 18px;
  }

  .panel-header,
  .panel-table-meta {
    align-items: stretch;
    flex-direction: column;
  }

  .panel-title {
    font-size: 18px;
  }

  .import-toolbar {
    grid-template-columns: 1fr;
  }

  .import-options-bar,
  .import-row-main,
  .batch-action-bar,
  .batch-action-copy {
    align-items: flex-start;
    flex-direction: column;
  }

  .batch-action-buttons {
    width: 100%;
    justify-content: flex-end;
  }

  .import-actions {
    margin-left: 0;
  }
}
</style>
