<script setup lang="ts">
import { computed, shallowRef, useTemplateRef } from 'vue'
import { ImagePlus, Trash2 } from 'lucide-vue-next'

export interface UpstreamImageTestValue {
  filename: string
  mime_type: 'image/png' | 'image/jpeg' | 'image/webp'
  b64_json: string
}

const MAX_IMAGE_MIB = 32
const MAX_IMAGE_BYTES = MAX_IMAGE_MIB * 1024 * 1024
const SUPPORTED_IMAGE_TYPES = new Set<UpstreamImageTestValue['mime_type']>([
  'image/png',
  'image/jpeg',
  'image/webp'
])

const image = defineModel<UpstreamImageTestValue | null>({ default: null })
const fileInput = useTemplateRef<HTMLInputElement>('fileInput')
const reading = shallowRef(false)
const validationError = shallowRef('')

const previewSource = computed(() => image.value
  ? `data:${image.value.mime_type};base64,${image.value.b64_json}`
  : '')
const selectedSize = computed(() => image.value
  ? formatBytes(decodedBase64Size(image.value.b64_json))
  : '')

function chooseImage() {
  fileInput.value?.click()
}

async function handleFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return

  const mimeType = normalizedImageType(file.type)
  if (!mimeType || !SUPPORTED_IMAGE_TYPES.has(mimeType)) {
    validationError.value = '仅支持 PNG、JPEG 或 WebP 图片'
    return
  }
  if (file.size > MAX_IMAGE_BYTES) {
    validationError.value = `图片不能超过 ${MAX_IMAGE_MIB} MiB`
    return
  }

  reading.value = true
  validationError.value = ''
  try {
    image.value = {
      filename: file.name,
      mime_type: mimeType,
      b64_json: await readFileAsBase64(file)
    }
  } catch {
    validationError.value = '图片读取失败，请重新选择'
  } finally {
    reading.value = false
  }
}

function clearImage() {
  image.value = null
  validationError.value = ''
}

function normalizedImageType(value: string): UpstreamImageTestValue['mime_type'] | '' {
  const normalized = value.trim().toLowerCase()
  if (normalized === 'image/jpg') return 'image/jpeg'
  return SUPPORTED_IMAGE_TYPES.has(normalized as UpstreamImageTestValue['mime_type'])
    ? normalized as UpstreamImageTestValue['mime_type']
    : ''
}

async function readFileAsBase64(file: File): Promise<string> {
  const bytes = new Uint8Array(await file.arrayBuffer())
  const chunks: string[] = []
  const chunkSize = 32 * 1024
  for (let offset = 0; offset < bytes.length; offset += chunkSize) {
    chunks.push(String.fromCharCode(...bytes.subarray(offset, offset + chunkSize)))
  }
  return btoa(chunks.join(''))
}

function decodedBase64Size(value: string) {
  const padding = value.endsWith('==') ? 2 : value.endsWith('=') ? 1 : 0
  return Math.max(0, Math.floor(value.length * 3 / 4) - padding)
}

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`
}
</script>

<template>
  <div class="image-test-upload">
    <input
      ref="fileInput"
      type="file"
      accept="image/png,image/jpeg,image/webp"
      class="image-test-upload__input"
      @change="handleFileSelected"
    />

    <div v-if="image" class="image-test-upload__selected">
      <img :src="previewSource" alt="参考图片预览" class="image-test-upload__preview" />
      <div class="image-test-upload__meta">
        <strong class="image-test-upload__name">{{ image.filename }}</strong>
        <span>{{ image.mime_type }} · {{ selectedSize }}</span>
        <el-button plain :loading="reading" @click="chooseImage">
          <ImagePlus :size="16" aria-hidden="true" />
          更换图片
        </el-button>
      </div>
      <el-button
        text
        class="image-test-upload__remove"
        aria-label="移除图片"
        title="移除图片"
        @click="clearImage"
      >
        <Trash2 :size="17" aria-hidden="true" />
      </el-button>
    </div>

    <div v-else class="image-test-upload__empty">
      <el-button type="primary" plain :loading="reading" @click="chooseImage">
        <ImagePlus :size="16" aria-hidden="true" />
        选择图片
      </el-button>
      <span>PNG、JPEG、WebP · 最大 {{ MAX_IMAGE_MIB }} MiB</span>
    </div>

    <p v-if="validationError" class="image-test-upload__error" role="alert">{{ validationError }}</p>
  </div>
</template>

<style scoped>
.image-test-upload {
  width: 100%;
}

.image-test-upload__input {
  display: none;
}

.image-test-upload__empty {
  min-height: 72px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  border: 1px dashed var(--el-border-color);
  border-radius: 8px;
  background: var(--el-fill-color-lighter);
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.image-test-upload__selected {
  min-height: 112px;
  display: grid;
  grid-template-columns: 88px minmax(0, 1fr) 32px;
  align-items: center;
  gap: 12px;
  padding: 12px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  background: var(--el-fill-color-lighter);
}

.image-test-upload__preview {
  width: 88px;
  height: 88px;
  object-fit: cover;
  border-radius: 6px;
  border: 1px solid var(--el-border-color);
  background: var(--el-bg-color);
}

.image-test-upload__meta {
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.image-test-upload__name {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--el-text-color-primary);
  font-size: 13px;
}

.image-test-upload__remove {
  width: 32px;
  height: 32px;
  padding: 0;
  color: var(--el-text-color-secondary);
}

.image-test-upload__error {
  margin: 6px 0 0;
  color: var(--el-color-danger);
  font-size: 12px;
  line-height: 1.4;
}

@media (max-width: 560px) {
  .image-test-upload__empty {
    align-items: flex-start;
    flex-direction: column;
  }

  .image-test-upload__selected {
    grid-template-columns: 72px minmax(0, 1fr) 32px;
  }

  .image-test-upload__preview {
    width: 72px;
    height: 72px;
  }
}
</style>
