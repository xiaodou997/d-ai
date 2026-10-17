<script setup lang="ts">
import { computed, reactive, shallowRef, watch } from 'vue'
import { ElMessage } from 'element-plus'

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
  endpoint: EndpointDraft
  concurrency_limit: number | null
  price_book_id: string
  tenant_multiplier: number | null
}

const submitting = shallowRef(false)
const isEditing = computed(() => Boolean(props.account?.id))
const invalidEndpointCount = computed(() => isEditing.value && props.account?.endpoints?.length !== 1)
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
    endpoint: blankEndpoint(), concurrency_limit: null, price_book_id: activePriceBookId.value, tenant_multiplier: 1
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
    endpoint: account.endpoints?.length === 1 ? {
      ...account.endpoints[0],
      extra_headers: { ...account.endpoints[0]?.extra_headers as Record<string, unknown> }
    } : blankEndpoint(),
    concurrency_limit: account.concurrency_limit ?? null,
    price_book_id: account.price_book_id || '',
    tenant_multiplier: account.tenant_multiplier ?? null
  })
}

watch(() => props.modelValue, (open) => {
  if (!open) return
  resetForm()
}, { immediate: true })
watch(activePriceBookId, (id) => {
  if (props.modelValue && !isEditing.value && !form.price_book_id) form.price_book_id = id
})

function endpointPayload(endpoint: EndpointDraft): UpstreamAccountEndpointWriteRequest & { id?: string } {
  return {
    ...(endpoint.id ? { id: endpoint.id } : {}),
    api_format: endpoint.api_format,
    base_url: endpoint.base_url.trim(),
    path_override: endpoint.path_override?.trim() || undefined,
    auth_scheme: endpoint.auth_scheme || 'format_default',
    auth_header: endpoint.auth_scheme === 'custom_header' ? endpoint.auth_header?.trim() : undefined,
    extra_headers: endpoint.extra_headers && Object.keys(endpoint.extra_headers).length ? endpoint.extra_headers : undefined,
    status: endpoint.status || 'active'
  }
}

function validateEndpoint() {
  if (invalidEndpointCount.value) {
    ElMessage.warning('账号应对应一个端点，请刷新页面后重试')
    return false
  }
  const endpoint = form.endpoint
  if (!endpoint.base_url.trim()) {
    ElMessage.warning(`请填写 ${upstreamAPIFormatLabel(endpoint.api_format)} 的 Base URL`)
    return false
  }
  if (endpoint.auth_scheme === 'custom_header' && !endpoint.auth_header?.trim()) {
    ElMessage.warning('自定义认证方式必须填写请求头名称')
    return false
  }
  return true
}

function payload(): AccountWriteRequest {
  const value: AccountWriteRequest = {
    name: form.name.trim(),
    description: form.description.trim(),
    tenant_display_name: form.tenant_display_name.trim() || form.name.trim(),
    tenant_access_mode: form.tenant_access_mode,
    concurrency_limit: form.concurrency_limit ?? null,
    price_book_id: form.price_book_id || undefined,
    tenant_multiplier: form.tenant_multiplier ?? undefined
  }
  value.endpoints = [endpointPayload(form.endpoint)]
  if (form.api_key.trim()) value.api_key = form.api_key.trim()
  return value
}

async function submit() {
  if (!form.name.trim()) { ElMessage.warning('请填写账号名称'); return }
  if (!isEditing.value && !form.api_key.trim()) { ElMessage.warning('请填写上游 API key'); return }
  if (!validateEndpoint()) return
  if (props.account?.status === 'active' && form.endpoint.status !== 'active') {
    ElMessage.warning('启用账号的请求端点必须启用')
    return
  }
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
    class="upstream-connection-dialog"
    :model-value="modelValue"
    :title="isEditing ? '编辑上游连接' : '新增上游账号'"
    width="min(760px, 94vw)"
    top="5vh"
    :close-on-click-modal="false"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="connection-form-scroll">
      <el-alert v-if="invalidEndpointCount" title="账号应对应一个端点，请刷新页面后重试" type="warning" :closable="false" />
      <el-form label-width="130px">
        <el-form-item label="名称" required><el-input v-model="form.name" placeholder="如 OpenAI 官方 / 某中转" /></el-form-item>
        <el-form-item label="描述"><el-input v-model="form.description" type="textarea" :rows="2" maxlength="200" show-word-limit placeholder="给租户展示的一句话说明（可选）" /></el-form-item>
        <el-form-item label="展示名称" required><el-input v-model="form.tenant_display_name" placeholder="租户目录中显示的名称" /></el-form-item>
        <el-form-item label="租户专属">
          <el-switch v-model="form.tenant_access_mode" active-value="restricted" inactive-value="public" active-text="开启" inactive-text="关闭" />
        </el-form-item>
        <el-form-item label="API Key" :required="!isEditing">
          <el-input v-model="form.api_key" type="password" show-password :placeholder="isEditing ? '留空不改；密文存储' : '输入上游 API Key（密文存储）'" />
          <span class="hint">此账号对应一个端点；地址、协议与密钥一起保存。</span>
        </el-form-item>

        <div class="endpoint-drafts">
          <div class="endpoint-drafts__head">
            <div><strong>请求端点</strong><p>一个账号对应一个端点。需要其他地址或 API 格式时，新增上游账号。</p></div>
          </div>
          <div class="endpoint-draft">
            <el-form-item label="API 格式" required>
              <el-select v-model="form.endpoint.api_format" class="w-full">
                <el-option v-for="option in upstreamAPIFormatOptions" :key="option.value" :label="option.label" :value="option.value" />
              </el-select>
            </el-form-item>
            <el-form-item label="Base URL" required><el-input v-model="form.endpoint.base_url" placeholder="https://api.example.com" /></el-form-item>
            <el-form-item label="路径覆盖"><el-input v-model="form.endpoint.path_override" placeholder="留空使用该 API 格式的默认路径" /></el-form-item>
            <el-form-item label="认证方式">
              <el-select v-model="form.endpoint.auth_scheme" class="w-full"><el-option v-for="option in endpointAuthSchemeOptions" :key="option.value" :label="option.label" :value="option.value" /></el-select>
            </el-form-item>
            <el-form-item v-if="form.endpoint.auth_scheme === 'custom_header'" label="认证请求头" required><el-input v-model="form.endpoint.auth_header" placeholder="如 X-API-Key" /></el-form-item>
            <el-form-item label="状态"><el-switch v-model="form.endpoint.status" active-value="active" inactive-value="disabled" active-text="启用" inactive-text="停用" /></el-form-item>
            <el-form-item label="附加请求头"><KeyValueEditor v-model="form.endpoint.extra_headers" /><span v-if="isEditing" class="hint">敏感值保持 ***REDACTED*** 可保留原配置。</span></el-form-item>
          </div>
        </div>

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
    </div>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="submitting" :disabled="invalidEndpointCount" @click="submit">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.connection-form-scroll { max-height: 70vh; overflow-y: auto; padding-right: 4px; }
.hint { color: var(--ds-faint); font-size: 12px; margin-left: 8px; }
.endpoint-drafts { display: flex; flex-direction: column; gap: 12px; margin: 8px 0 18px; }
.endpoint-drafts__head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.endpoint-drafts__head p { margin: 4px 0 0; color: var(--ds-muted); font-size: 12px; }
.endpoint-draft { padding: 14px 14px 2px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-control); background: var(--ds-panel-muted); }
@media (max-width: 600px) {
  .connection-form-scroll :deep(.el-form-item) { display: block; }
  .connection-form-scroll :deep(.el-form-item__label) { width: auto !important; justify-content: flex-start; padding: 0; }
  .connection-form-scroll :deep(.el-form-item__content) { margin-left: 0 !important; }
  .connection-form-scroll :deep(.kv .row) { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) 28px; gap: 6px; }
  .endpoint-drafts__head { flex-wrap: wrap; }
}
</style>
