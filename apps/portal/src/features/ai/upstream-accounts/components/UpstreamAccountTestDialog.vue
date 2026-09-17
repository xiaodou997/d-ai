<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, shallowRef, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { VideoPlay } from '@element-plus/icons-vue'

import { aiAdminApi } from '@/api/aiAdmin'
import type { AccountDTO, UpstreamAccountEndpointDTO, UpstreamAccountTestImage, UpstreamAccountTestResult, UpstreamModelBindingDTO } from '@/api/types/ai'
import { formatDuration } from '@/platform/ai/usage'
import { upstreamAPIFormatLabel } from '../constants'
import { accountHost } from '../presentation'
import { upstreamAccountStatusLabel, upstreamAccountStatusTagType } from './status'
import UpstreamImageTestUpload from './UpstreamImageTestUpload.vue'

const props = withDefaults(defineProps<{
  modelValue: boolean
  account?: AccountDTO | null
}>(), { account: null })

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  tested: [result: UpstreamAccountTestResult]
}>()

const models = ref<{ model_code: string; capability_type: string }[]>([])
const modelsLoading = shallowRef(false)
const form = reactive({ modelCode: '', apiFormat: '', prompt: '', imageEdit: false })
const image = shallowRef<UpstreamAccountTestImage | null>(null)
const testing = shallowRef(false)
const result = shallowRef<UpstreamAccountTestResult | null>(null)
const error = shallowRef('')
const elapsedSeconds = shallowRef(0)
let elapsedTimer: ReturnType<typeof setInterval> | undefined

const selectedBinding = computed(() => models.value.find((model) => model.model_code === form.modelCode))
const selectedCapability = computed(() => selectedBinding.value?.capability_type ?? '')
const isImage = computed(() => selectedCapability.value === 'image')
const endpoints = computed<UpstreamAccountEndpointDTO[]>(() => {
  const active = (props.account?.endpoints || []).filter((endpoint) => endpoint.status === 'active')
  if (selectedCapability.value === 'image') return active.filter((endpoint) => ['openai_images', 'gemini_generate'].includes(endpoint.api_format))
  if (selectedCapability.value === 'embedding') return active.filter((endpoint) => ['openai_embeddings', 'gemini_embeddings'].includes(endpoint.api_format))
  if (selectedCapability.value === 'chat') return active.filter((endpoint) => ['openai_chat', 'openai_responses', 'anthropic_messages', 'gemini_generate'].includes(endpoint.api_format))
  return []
})
const supportsImageEdit = computed(() => isImage.value && form.apiFormat === 'openai_images')
const canRun = computed(() => Boolean(form.modelCode && form.apiFormat) && (!form.imageEdit || Boolean(image.value)))

watch([() => form.modelCode, endpoints], () => {
  if (!endpoints.value.some((endpoint) => endpoint.api_format === form.apiFormat)) form.apiFormat = endpoints.value[0]?.api_format || ''
})
watch(supportsImageEdit, (supported) => { if (!supported) form.imageEdit = false })
watch(() => form.imageEdit, (edit) => { if (!edit) image.value = null })
watch(() => props.modelValue, async (open) => {
  if (!open || !props.account) return
  models.value = []
  Object.assign(form, { modelCode: '', apiFormat: '', prompt: '', imageEdit: false })
  image.value = null
  result.value = null
  error.value = ''
  elapsedSeconds.value = 0
  modelsLoading.value = true
  try {
    const data = await aiAdminApi.listAccountModelBindings(props.account.id)
    models.value = (data.items ?? []).map((binding: UpstreamModelBindingDTO) => ({
      model_code: binding.model_code,
      capability_type: binding.capability_type
    }))
    if (models.value.length) form.modelCode = models.value[0].model_code
  } catch (cause) {
    ElMessage.error(cause instanceof Error ? cause.message : '加载模型绑定失败')
  } finally {
    modelsLoading.value = false
  }
})

function modelLabel(model: { model_code: string; capability_type: string }) {
  const capability = model.capability_type === 'image' ? '生图' : model.capability_type === 'chat' ? '对话' : model.capability_type === 'embedding' ? '向量' : model.capability_type
  return `${model.model_code} · ${capability}`
}

function imageStreamLabel(value?: string) { return value === 'force_stream' ? '流式' : '非流式' }
function imageFormatLabel(value?: string) { return value === 'url' ? 'URL' : value === 'b64_json' ? 'Base64' : '—' }
function imageTransportLabel(value?: string) { return value === 'application/json' ? 'JSON 图片 URL' : value === 'multipart/form-data' ? 'Multipart 文件上传' : '—' }

function startElapsedTimer() {
  if (elapsedTimer) clearInterval(elapsedTimer)
  const startedAt = Date.now()
  elapsedSeconds.value = 0
  elapsedTimer = setInterval(() => { elapsedSeconds.value = Math.floor((Date.now() - startedAt) / 1000) }, 1000)
}

function stopElapsedTimer() {
  if (elapsedTimer) clearInterval(elapsedTimer)
  elapsedTimer = undefined
}

async function run() {
  if (testing.value || !props.account || !form.modelCode) return
  if (form.imageEdit && !image.value) { ElMessage.warning('请选择图片编辑测试使用的参考图片'); return }
  testing.value = true
  result.value = null
  error.value = ''
  startElapsedTimer()
  try {
    result.value = await aiAdminApi.testUpstreamAccount(props.account.id, {
      model_code: form.modelCode,
      api_format: form.apiFormat,
      prompt: form.prompt.trim() || undefined,
      image_edit: isImage.value && form.imageEdit,
      image: isImage.value && form.imageEdit ? image.value || undefined : undefined
    })
    if (!result.value.ok) error.value = result.value.error || '上游未返回可用结果'
    emit('tested', result.value)
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '测试请求失败'
  } finally {
    stopElapsedTimer()
    testing.value = false
  }
}

onBeforeUnmount(stopElapsedTimer)
</script>

<template>
  <el-dialog
    :model-value="modelValue"
    title="测试账号连通"
    width="620px"
    :close-on-click-modal="!testing"
    :close-on-press-escape="!testing"
    :show-close="!testing"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div v-if="account" class="test-dialog">
      <div class="test-account-head">
        <el-icon class="test-account-icon"><VideoPlay /></el-icon>
        <div class="test-account-meta"><strong>{{ account.name }}</strong><span>直连上游账号测试</span></div>
        <el-tag :type="upstreamAccountStatusTagType(account.status)" size="small" effect="light">{{ upstreamAccountStatusLabel(account.status) }}</el-tag>
      </div>

      <el-form label-position="top">
        <el-form-item label="选择测试模型">
          <el-select v-model="form.modelCode" :loading="modelsLoading" placeholder="选择该账号下的显式绑定模型" style="width: 100%">
            <el-option v-for="model in models" :key="model.model_code" :label="modelLabel(model)" :value="model.model_code" />
          </el-select>
          <p v-if="!modelsLoading && !models.length" class="test-empty-hint">该账号暂无显式模型绑定，请先在「模型绑定」中发现或添加模型。</p>
        </el-form-item>
        <el-form-item label="选择请求端点" required>
          <el-select v-model="form.apiFormat" placeholder="选择与模型能力兼容的请求格式" style="width: 100%">
            <el-option v-for="endpoint in endpoints" :key="endpoint.id" :label="`${upstreamAPIFormatLabel(endpoint.api_format)} · ${accountHost(endpoint.base_url)}`" :value="endpoint.api_format" />
          </el-select>
          <p v-if="form.modelCode && !endpoints.length" class="test-empty-hint">当前账号没有与该模型能力兼容的启用端点。</p>
        </el-form-item>
        <el-form-item :label="isImage ? '生图提示词' : selectedCapability === 'embedding' ? '向量输入文本' : '对话提示词'">
          <el-input
            v-model="form.prompt"
            type="textarea"
            :rows="3"
            :placeholder="isImage ? 'Generate a cute orange cat astronaut sticker on a clean pastel background.' : selectedCapability === 'embedding' ? 'A short sentence for embedding validation.' : '留空使用默认连通性测试内容；也可输入实际业务内容。'"
          />
          <p class="test-hint">测试会等待上游返回完整的完成或失败结果，上游可能计费。</p>
        </el-form-item>
        <el-form-item v-if="isImage" label="测试类型">
          <el-radio-group v-model="form.imageEdit"><el-radio :value="false">图片生成</el-radio><el-radio :value="true" :disabled="!supportsImageEdit">图片编辑</el-radio></el-radio-group>
        </el-form-item>
        <el-form-item v-if="isImage && form.imageEdit" label="参考图片" required><UpstreamImageTestUpload v-model="image" /></el-form-item>
      </el-form>

      <div class="test-console">
        <template v-if="testing">
          <span class="test-line test-warn">⟳ 等待上游完整结果 · 已等待 {{ elapsedSeconds }} 秒</span>
          <span class="test-line test-muted">模型：{{ form.modelCode }} · 请求已发出，暂时无法判断上游处理阶段</span>
        </template>
        <template v-else-if="result">
          <strong class="test-line" :class="result.ok ? 'test-ok' : 'test-err'">{{ result.ok ? '✓ 测试成功' : '✗ 测试失败' }}</strong>
          <span v-if="error" class="test-line test-err">{{ error }}</span>
          <span class="test-line test-muted">HTTP {{ result.http_status }} · {{ formatDuration(result.latency_ms) }} · {{ result.api_format }}</span>
          <span class="test-line test-muted">上游模型 ID：{{ result.upstream_model }}</span>
          <span v-if="isImage" class="test-line test-muted">上游请求：{{ imageStreamLabel(result.image_stream_mode) }}<template v-if="form.imageEdit"> · {{ imageTransportLabel(result.image_edit_transport) }}</template> · 返回格式：{{ imageFormatLabel(result.image_upstream_response_format) }}</span>
          <span v-if="isImage && result.actual_image_format" class="test-line test-muted">上游响应：{{ imageFormatLabel(result.actual_image_format) }}</span>
          <span v-if="result.total_tokens" class="test-line test-muted">tokens：in {{ result.prompt_tokens ?? 0 }} / out {{ result.output_tokens ?? 0 }} / total {{ result.total_tokens }}</span>
          <span v-if="result.response_content_type" class="test-line test-muted">响应类型：{{ result.response_content_type }}</span>
          <span v-if="result.upstream_request_id" class="test-line test-muted">上游请求 ID：{{ result.upstream_request_id }}</span>
          <template v-if="result.ok && !isImage && result.reply_text"><span class="test-line test-muted">回复：</span><span class="test-line test-reply">{{ result.reply_text }}</span></template>
          <div v-if="result.ok && isImage" class="test-image-wrap">
            <img v-if="result.image_b64" :src="`data:${result.image_mime || 'image/png'};base64,${result.image_b64}`" alt="生图测试结果" class="test-image" />
            <img v-else-if="result.image_url" :src="result.image_url" alt="生图测试结果" class="test-image" />
          </div>
        </template>
        <span v-else-if="error" class="test-line test-err">{{ error }}</span>
        <span v-else class="test-line test-muted">尚未测试。选择模型后点击“开始测试”。</span>
      </div>
    </div>
    <template #footer>
      <el-button :disabled="testing" @click="emit('update:modelValue', false)">关闭</el-button>
      <el-button type="primary" :loading="testing" :disabled="testing || !canRun" :icon="VideoPlay" @click="run">{{ testing ? `已等待 ${elapsedSeconds} 秒` : '开始测试' }}</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.test-dialog { display: flex; flex-direction: column; gap: 16px; }
.test-account-head { display: flex; align-items: center; gap: 12px; padding: 12px 14px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-panel); background: var(--ds-panel-muted); }
.test-account-icon { display: flex; flex: none; align-items: center; justify-content: center; width: 40px; height: 40px; border-radius: var(--ds-radius-panel); background: var(--ds-positive); color: var(--ds-accent-contrast); font-size: 18px; }
.test-account-meta { display: flex; flex: 1; min-width: 0; flex-direction: column; }
.test-account-meta span,
.test-hint,
.test-empty-hint { color: var(--ds-muted); font-size: 12px; }
.test-hint,
.test-empty-hint { margin: 6px 0 0; }
.test-empty-hint { color: var(--ds-warning); }
.test-console { --console-bg: color-mix(in srgb, var(--ds-ink) 94%, var(--ds-panel)); --console-fg: color-mix(in srgb, var(--ds-panel) 80%, var(--ds-ink)); --console-ok: color-mix(in srgb, var(--ds-positive) 65%, var(--ds-panel)); --console-err: color-mix(in srgb, var(--ds-danger) 60%, var(--ds-panel)); --console-warn: color-mix(in srgb, var(--ds-warning) 70%, var(--ds-panel)); --console-muted: var(--ds-faint); --console-reply: var(--ds-line); display: flex; overflow: auto; max-height: 320px; flex-direction: column; padding: 14px 16px; border-radius: var(--ds-radius-panel); background: var(--console-bg); color: var(--console-fg); font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 13px; line-height: 1.7; }
.test-line { white-space: pre-wrap; word-break: break-word; }
.test-ok { color: var(--console-ok); }
.test-err { color: var(--console-err); }
.test-warn { color: var(--console-warn); }
.test-muted { color: var(--console-muted); }
.test-reply { color: var(--console-reply); }
.test-image-wrap { margin-top: 10px; }
.test-image { max-width: 100%; max-height: 240px; border: 1px solid color-mix(in srgb, var(--ds-faint) 30%, transparent); border-radius: var(--ds-radius-control); }
</style>
