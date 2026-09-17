<script setup lang="ts">
import { computed, reactive, shallowRef, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'

import { aiAdminApi } from '@/api/aiAdmin'
import type { AccountDTO, AccountWriteRequest, UpstreamAccountEndpointWriteRequest } from '@/api/types/ai'
import { DsNumberInput } from '@/shared/ui'
import type { PriceBookRecord } from '@/features/ai/price-books/pricingTypes'
import { firstActivePriceBookId } from '@/features/ai/price-books/priceBookSelection'
import { endpointAuthSchemeOptions, upstreamAPIFormatLabel, upstreamAPIFormatOptions } from '../constants'
import KeyValueEditor from './KeyValueEditor.vue'

const props = withDefaults(defineProps<{
  modelValue: boolean
  account?: AccountDTO | null
  priceBooks?: PriceBookRecord[]
}>(), {
  account: null,
  priceBooks: () => []
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  saved: [account: AccountDTO]
}>()

type EndpointDraft = Omit<UpstreamAccountEndpointWriteRequest, 'extra_headers'> & {
  id?: string
  extra_headers?: Record<string, unknown>
}

interface AccountForm {
  name: string
  description: string
  tenant_display_name: string
  tenant_access_mode: 'public' | 'restricted'
  api_key: string
  endpoints: EndpointDraft[]
  concurrency_limit: number | null
  price_book_id: string
  tenant_multiplier: number | null
}

const submitting = shallowRef(false)
const isEditing = computed(() => Boolean(props.account?.id))
const activePriceBookId = computed(() => firstActivePriceBookId(props.priceBooks))

function blankEndpoint(): EndpointDraft {
  return {
    api_format: 'openai_responses',
    base_url: '',
    path_override: '',
    auth_scheme: 'format_default',
    auth_header: '',
    extra_headers: {},
    status: 'active'
  }
}

function blankAccount(): AccountForm {
  return {
    name: '', description: '', tenant_display_name: '', tenant_access_mode: 'public', api_key: '',
    endpoints: [blankEndpoint()], concurrency_limit: null, price_book_id: activePriceBookId.value, tenant_multiplier: 1
  }
}

const form = reactive<AccountForm>(blankAccount())

function resetForm() {
  const account = props.account
  if (!account) {
    Object.assign(form, blankAccount())
    return
  }
  Object.assign(form, blankAccount(), {
    name: account.name,
    description: account.description || '',
    tenant_display_name: account.tenant_display_name || account.name,
    tenant_access_mode: account.tenant_access_mode || 'public',
    api_key: '',
    endpoints: [],
    concurrency_limit: account.concurrency_limit ?? null,
    price_book_id: account.price_book_id || '',
    tenant_multiplier: account.tenant_multiplier ?? null
  })
}

watch(() => props.modelValue, (open) => { if (open) resetForm() })
watch(activePriceBookId, (id) => {
  if (props.modelValue && !isEditing.value && !form.price_book_id) form.price_book_id = id
})

function endpointPayload(endpoint: EndpointDraft): UpstreamAccountEndpointWriteRequest {
  return {
    api_format: endpoint.api_format,
    base_url: endpoint.base_url.trim(),
    path_override: endpoint.path_override?.trim() || undefined,
    auth_scheme: endpoint.auth_scheme || 'format_default',
    auth_header: endpoint.auth_scheme === 'custom_header' ? endpoint.auth_header?.trim() : undefined,
    extra_headers: endpoint.extra_headers && Object.keys(endpoint.extra_headers).length ? endpoint.extra_headers : undefined,
    status: endpoint.status || 'active'
  }
}

function validateEndpoints() {
  if (!form.endpoints.length) {
    ElMessage.warning('至少配置一个请求端点')
    return false
  }
  const formats = new Set<string>()
  for (const endpoint of form.endpoints) {
    if (!endpoint.base_url.trim()) {
      ElMessage.warning(`请填写 ${upstreamAPIFormatLabel(endpoint.api_format)} 的 Base URL`)
      return false
    }
    if (formats.has(endpoint.api_format)) {
      ElMessage.warning(`API 格式不能重复：${upstreamAPIFormatLabel(endpoint.api_format)}`)
      return false
    }
    if (endpoint.auth_scheme === 'custom_header' && !endpoint.auth_header?.trim()) {
      ElMessage.warning('自定义认证方式必须填写请求头名称')
      return false
    }
    formats.add(endpoint.api_format)
  }
  return true
}

function addEndpoint() {
  const used = new Set(form.endpoints.map((endpoint) => endpoint.api_format))
  const next = upstreamAPIFormatOptions.find((option) => !used.has(option.value))
  if (!next) {
    ElMessage.info('所有 API 格式都已配置')
    return
  }
  form.endpoints.push({ ...blankEndpoint(), api_format: next.value })
}

function removeEndpoint(index: number) {
  if (form.endpoints.length <= 1) {
    ElMessage.warning('账号至少需要一个请求端点')
    return
  }
  form.endpoints.splice(index, 1)
}

function formatDisabled(format: string, index: number) {
  return form.endpoints.some((endpoint, candidateIndex) => candidateIndex !== index && endpoint.api_format === format)
}

function payload(): AccountWriteRequest {
  const value: AccountWriteRequest = {
    name: form.name.trim(),
    description: form.description.trim() || undefined,
    tenant_display_name: form.tenant_display_name.trim() || form.name.trim(),
    tenant_access_mode: form.tenant_access_mode,
    concurrency_limit: form.concurrency_limit ?? null,
    price_book_id: form.price_book_id || undefined,
    tenant_multiplier: form.tenant_multiplier ?? undefined
  }
  if (!isEditing.value) value.endpoints = form.endpoints.map(endpointPayload)
  if (form.api_key.trim()) value.api_key = form.api_key.trim()
  return value
}

async function submit() {
  if (!form.name.trim()) { ElMessage.warning('请填写账号名称'); return }
  if (!isEditing.value && !form.api_key.trim()) { ElMessage.warning('请填写上游 API key'); return }
  if (!isEditing.value && !validateEndpoints()) return
  submitting.value = true
  try {
    const saved = isEditing.value
      ? await aiAdminApi.updateUpstreamAccount(props.account!.id, payload())
      : await aiAdminApi.createUpstreamAccount(payload())
    ElMessage.success(isEditing.value ? '账号已更新' : '账号已创建')
    emit('update:modelValue', false)
    emit('saved', saved)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存失败')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <el-dialog
    :model-value="modelValue"
    :title="isEditing ? '编辑上游账号' : '新增上游账号'"
    width="640px"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <el-form label-width="130px">
      <el-form-item label="名称" required><el-input v-model="form.name" placeholder="如 OpenAI 官方 / 某中转" /></el-form-item>
      <el-form-item label="描述"><el-input v-model="form.description" type="textarea" :rows="2" maxlength="200" show-word-limit placeholder="给租户展示的一句话说明（可选）" /></el-form-item>
      <el-form-item label="展示名称" required><el-input v-model="form.tenant_display_name" placeholder="租户目录中显示的名称" /></el-form-item>
      <el-form-item label="租户专属">
        <el-switch v-model="form.tenant_access_mode" active-value="restricted" inactive-value="public" active-text="开启" inactive-text="关闭" />
      </el-form-item>
      <el-form-item label="API Key" :required="!isEditing">
        <el-input v-model="form.api_key" type="password" show-password :placeholder="isEditing ? '留空不改；密文存储' : '输入上游 API Key（密文存储）'" />
      </el-form-item>

      <div v-if="!isEditing" class="endpoint-drafts">
        <div class="endpoint-drafts__head">
          <div><strong>请求端点</strong><p>一个账号可支持多种 API 格式；同一种格式不能重复配置。</p></div>
          <el-button size="small" :icon="Plus" @click="addEndpoint">添加格式</el-button>
        </div>
        <div v-for="(endpoint, index) in form.endpoints" :key="index" class="endpoint-draft">
          <div class="endpoint-draft__title">
            <span>端点 {{ index + 1 }}</span>
            <el-button link type="danger" :disabled="form.endpoints.length <= 1" @click="removeEndpoint(index)">移除</el-button>
          </div>
          <el-form-item label="API 格式" required>
            <el-select v-model="endpoint.api_format" class="w-full">
              <el-option v-for="option in upstreamAPIFormatOptions" :key="option.value" :label="option.label" :value="option.value" :disabled="formatDisabled(option.value, index)" />
            </el-select>
          </el-form-item>
          <el-form-item label="Base URL" required><el-input v-model="endpoint.base_url" placeholder="https://api.example.com" /></el-form-item>
          <el-form-item label="路径覆盖"><el-input v-model="endpoint.path_override" placeholder="留空使用该 API 格式的默认路径" /></el-form-item>
          <el-form-item label="认证方式">
            <el-select v-model="endpoint.auth_scheme" class="w-full"><el-option v-for="option in endpointAuthSchemeOptions" :key="option.value" :label="option.label" :value="option.value" /></el-select>
          </el-form-item>
          <el-form-item v-if="endpoint.auth_scheme === 'custom_header'" label="认证请求头" required><el-input v-model="endpoint.auth_header" placeholder="如 X-API-Key" /></el-form-item>
          <el-form-item label="附加请求头"><KeyValueEditor v-model="endpoint.extra_headers" /></el-form-item>
        </div>
      </div>
      <el-alert v-else type="info" :closable="false" title="请求端点在账号详情页单独管理，修改账号资料不会覆盖端点配置。" />

      <el-form-item label="最大并发数">
        <DsNumberInput v-model="form.concurrency_limit" :min="1" :step="1" />
        <span class="hint">留空表示不限制；指该账号同时在飞的上游请求数上限。</span>
      </el-form-item>
      <el-form-item label="价格表">
        <el-select v-model="form.price_book_id" clearable class="w-full" :placeholder="activePriceBookId ? '该账号成本基准' : '暂无启用价格表'">
          <el-option v-for="book in priceBooks" :key="book.id" :label="book.name" :value="book.id" />
        </el-select>
      </el-form-item>
      <el-form-item label="租户倍率">
        <DsNumberInput v-model="form.tenant_multiplier" :min="0" :step="0.1" :precision="4" />
        <span class="hint">租户扣费 = 价格表 USD × 默认倍率；默认 1</span>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="submit">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.hint { color: var(--ds-faint); font-size: 12px; margin-left: 8px; }
.endpoint-drafts { display: flex; flex-direction: column; gap: 12px; margin: 8px 0 18px; }
.endpoint-drafts__head,
.endpoint-draft__title { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.endpoint-drafts__head p { margin: 4px 0 0; color: var(--ds-muted); font-size: 12px; }
.endpoint-draft { padding: 14px 14px 2px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-control); background: var(--ds-panel-muted); }
.endpoint-draft__title { margin-bottom: 10px; color: var(--ds-ink); font-weight: 700; }
</style>
