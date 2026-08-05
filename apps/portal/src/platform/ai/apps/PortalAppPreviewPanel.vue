<script setup lang="ts">
import { computed, reactive, shallowRef, watch } from "vue";

import PortalContentCard from "../../page/PortalContentCard.vue";
import type {
  PortalAppPreviewRequest,
  PortalAppPreviewResponse,
  PortalAppRecord,
  PortalAppRuntimeConfig,
  PortalAppType
} from "./types";
import { appAllowsAttachments, appRuntimeConfig } from "./contract";

const props = defineProps<{
  app: PortalAppRecord;
  variables: string[];
  canPreview: boolean;
  previewApp?: (appId: string, payload: PortalAppPreviewRequest) => Promise<PortalAppPreviewResponse>;
  notifyError?: (message: string) => void;
}>();

const loading = shallowRef(false);
const result = shallowRef<PortalAppPreviewResponse | null>(null);

const form = reactive({
  input: "",
  responseFormat: "b64_json" as "url" | "b64_json",
  outputCount: 1,
  attachmentsText: "",
  imagesText: ""
});

const variableValues = reactive<Record<string, string>>({});

const runtimeConfig = computed<PortalAppRuntimeConfig>(() => appRuntimeConfig(props.app));
const appType = computed(() => props.app.capability);
const isImageApp = computed(() => appType.value === "image_generation" || appType.value === "image_edit");
const allowsAttachments = computed(() => appAllowsAttachments(appType.value, runtimeConfig.value));
const outputCountMax = computed(() => {
  const config = runtimeConfig.value.image;
  if (!config) return 1;
  return config.allow_output_count_override ? config.max_output_count : config.default_output_count;
});
const outputCountLocked = computed(() => !runtimeConfig.value.image?.allow_output_count_override);
const helperText = computed(() => {
  if (!props.canPreview) {
    return "当前作用域暂不支持试运行。";
  }
  if (appType.value === "chat") {
    return "试运行不会创建会话，但会按真实链路执行、计费并进入用量记录。";
  }
  return "试运行不会落图片任务或作品，但会按真实链路执行、计费并进入用量记录。";
});
const resultImages = computed(() => result.value?.images ?? []);
const resultText = computed(() => result.value?.text || "");

watch(
  () => [props.app.id, runtimeConfig.value.image?.default_output_count] as const,
  () => {
    form.outputCount = runtimeConfig.value.image?.default_output_count ?? 1;
  },
  { immediate: true }
);

function ensureVariableKeys() {
  for (const name of props.variables) {
    if (!(name in variableValues)) {
      variableValues[name] = "";
    }
  }
  for (const key of Object.keys(variableValues)) {
    if (!props.variables.includes(key)) {
      delete variableValues[key];
    }
  }
}

ensureVariableKeys();
watch(() => props.variables, ensureVariableKeys, { deep: true });

function buildVariables() {
  const entries = Object.entries(variableValues)
    .map(([key, value]) => [key.trim(), value.trim()] as const)
    .filter(([key, value]) => key && value);
  return entries.length ? Object.fromEntries(entries) : undefined;
}

function parseLineList(text: string) {
  return text
    .split("\n")
    .map((item) => item.trim())
    .filter(Boolean);
}

function buildPayload(): PortalAppPreviewRequest {
  const variables = buildVariables();
  const payload: PortalAppPreviewRequest = {
    variables
  };
  payload.input = form.input.trim();
  if (appType.value === "chat") {
    if (allowsAttachments.value) {
      const attachments = parseLineList(form.attachmentsText).map((url) => ({ url }));
      if (attachments.length) payload.attachments = attachments;
    }
    return payload;
  }
  payload.response_format = form.responseFormat;
  payload.n = form.outputCount;
  if (appType.value === "image_edit") {
    const images = parseLineList(form.imagesText);
    if (images.length) payload.images = images;
  }
  return payload;
}

function firstError(): string | null {
  if (!props.canPreview || !props.previewApp) {
    return "当前作用域暂不支持试运行";
  }
  if (!form.input.trim()) {
    return "请输入 input";
  }
  if (appType.value === "image_edit" && parseLineList(form.imagesText).length === 0) {
    return "图生图至少需要一张参考图";
  }
  return null;
}

async function handlePreview() {
  const error = firstError();
  if (error) {
    props.notifyError?.(error);
    return;
  }
  loading.value = true;
  result.value = null;
  try {
    result.value = await props.previewApp!(props.app.id, buildPayload());
  } catch (err) {
    props.notifyError?.((err as Error).message || "试运行失败");
  } finally {
    loading.value = false;
  }
}

function imageSrc(image: NonNullable<PortalAppPreviewResponse["images"]>[number]) {
  if (image.url) return image.url;
  if (image.b64_json) return `data:image/png;base64,${image.b64_json}`;
  return "";
}

function responseFormatLabel(appType: PortalAppType) {
  return appType === "image_generation" || appType === "image_edit" ? "返回格式" : "";
}
</script>

<template>
  <PortalContentCard title="试运行" :description="helperText">
    <div class="preview-panel">
      <div class="preview-panel__form">
        <div class="preview-panel__group">
          <label class="preview-panel__label">input</label>
          <textarea
            v-model="form.input"
            class="preview-panel__textarea"
            :placeholder="app.prompt_strategy === 'bound_prompt_exact' ? '例如：请结合 {{客户背景}} 完成本次任务' : '输入一次性测试内容'"
          />
        </div>

        <div v-if="variables.length" class="preview-panel__group">
          <label class="preview-panel__label">variables</label>
          <div class="preview-panel__variables">
            <div v-for="name in variables" :key="name" class="preview-panel__variable-row">
              <span class="preview-panel__variable-name">{{ name }}</span>
              <input v-model="variableValues[name]" class="preview-panel__input" :placeholder="`${name} 的值`" />
            </div>
          </div>
        </div>

        <div v-if="appType === 'chat' && allowsAttachments" class="preview-panel__group">
          <label class="preview-panel__label">attachments</label>
          <textarea
            v-model="form.attachmentsText"
            class="preview-panel__textarea preview-panel__textarea--compact"
            placeholder="每行一个直连 URL；默认按文件处理，图片后缀会自动识别为 image"
          />
        </div>

        <div v-if="appType === 'image_edit'" class="preview-panel__group">
          <label class="preview-panel__label">images</label>
          <textarea
            v-model="form.imagesText"
            class="preview-panel__textarea preview-panel__textarea--compact"
            placeholder="每行一张参考图 URL 或 base64 data URL"
          />
        </div>

        <div v-if="isImageApp" class="preview-panel__inline">
          <div class="preview-panel__group preview-panel__group--inline">
            <label class="preview-panel__label">{{ responseFormatLabel(appType) }}</label>
            <div class="preview-panel__segmented">
              <button
                type="button"
                class="preview-panel__segmented-btn"
                :class="{ 'is-active': form.responseFormat === 'b64_json' }"
                @click="form.responseFormat = 'b64_json'"
              >
                Base64
              </button>
              <button
                type="button"
                class="preview-panel__segmented-btn"
                :class="{ 'is-active': form.responseFormat === 'url' }"
                @click="form.responseFormat = 'url'"
              >
                URL
              </button>
            </div>
          </div>
          <div class="preview-panel__group preview-panel__group--inline">
            <label class="preview-panel__label">输出张数</label>
            <input
              v-model.number="form.outputCount"
              class="preview-panel__input preview-panel__number"
              type="number"
              min="1"
              :max="outputCountMax"
              :disabled="outputCountLocked"
            />
          </div>
        </div>

        <div class="preview-panel__actions">
          <button class="preview-panel__submit" type="button" :disabled="loading || !canPreview" @click="handlePreview">
            {{ loading ? "执行中..." : "测试" }}
          </button>
        </div>
      </div>

      <div class="preview-panel__result">
        <div v-if="result" class="preview-panel__result-card">
          <div class="preview-panel__result-meta">
            <span class="preview-panel__chip">{{ result.type }}</span>
            <span v-if="result.request_id" class="preview-panel__request-id">{{ result.request_id }}</span>
          </div>

          <pre v-if="result.type === 'chat'" class="preview-panel__text">{{ resultText }}</pre>

          <div v-else class="preview-panel__images">
            <div v-if="resultImages.length" class="preview-panel__image-grid">
              <figure v-for="(image, index) in resultImages" :key="index" class="preview-panel__image-card">
                <img v-if="imageSrc(image)" :src="imageSrc(image)" alt="" />
                <div v-else class="preview-panel__image-empty">无可展示图片</div>
              </figure>
            </div>
            <div v-else class="preview-panel__empty">本次没有返回图片。</div>
          </div>

          <pre v-if="result.usage" class="preview-panel__usage">{{ JSON.stringify(result.usage, null, 2) }}</pre>
        </div>

        <div v-else class="preview-panel__empty">
          这里展示试运行结果。不会创建会话或作品，但会进入真实计费与用量链路。
        </div>
      </div>
    </div>
  </PortalContentCard>
</template>

<style scoped>
.preview-panel {
  display: grid;
  grid-template-columns: minmax(0, 360px) minmax(0, 1fr);
  gap: 16px;
}

.preview-panel__form,
.preview-panel__result {
  min-width: 0;
}

.preview-panel__form {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.preview-panel__group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.preview-panel__group--inline {
  min-width: 0;
}

.preview-panel__inline {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.preview-panel__label {
  color: var(--ds-faint);
  font-size: 12px;
  font-weight: 700;
}

.preview-panel__textarea,
.preview-panel__input {
  width: 100%;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel);
  color: var(--ds-ink);
  font: inherit;
}

.preview-panel__number {
  width: 92px;
}

.preview-panel__textarea {
  min-height: 132px;
  padding: 12px 14px;
  line-height: 1.7;
  resize: vertical;
}

.preview-panel__textarea--compact {
  min-height: 88px;
}

.preview-panel__input {
  padding: 10px 12px;
  line-height: 1.5;
}

.preview-panel__variables {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.preview-panel__variable-row {
  display: grid;
  grid-template-columns: 120px minmax(0, 1fr);
  gap: 10px;
  align-items: center;
}

.preview-panel__variable-name {
  color: var(--ds-muted);
  font-size: 12px;
  font-weight: 700;
}

.preview-panel__segmented {
  display: inline-flex;
  padding: 3px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
}

.preview-panel__segmented-btn {
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--ds-muted);
  padding: 8px 12px;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
}

.preview-panel__segmented-btn.is-active {
  background: var(--ds-panel);
  color: var(--ds-ink);
  box-shadow: var(--ds-shadow-sm);
}

.preview-panel__actions {
  display: flex;
  justify-content: flex-start;
}

.preview-panel__submit {
  border: 0;
  border-radius: var(--ds-radius-control);
  background: var(--ds-accent);
  color: #fff;
  padding: 11px 16px;
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
}

.preview-panel__submit:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.preview-panel__result-card,
.preview-panel__empty {
  min-height: 100%;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel-muted);
  padding: 14px;
}

.preview-panel__result-card {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.preview-panel__result-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.preview-panel__chip {
  padding: 6px 10px;
  border-radius: var(--ds-radius-control);
  background: color-mix(in srgb, var(--ds-accent) 10%, var(--ds-panel));
  color: var(--ds-accent-hover);
  font-size: 12px;
  font-weight: 700;
}

.preview-panel__request-id {
  color: var(--ds-faint);
  font-size: 12px;
}

.preview-panel__text,
.preview-panel__usage {
  margin: 0;
  padding: 12px 14px;
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel);
  color: var(--ds-ink);
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.7;
}

.preview-panel__usage {
  font-size: 12px;
  color: var(--ds-muted);
}

.preview-panel__images {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.preview-panel__image-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
}

.preview-panel__image-card {
  margin: 0;
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  aspect-ratio: 1 / 1;
}

.preview-panel__image-card img,
.preview-panel__image-empty {
  width: 100%;
  height: 100%;
}

.preview-panel__image-card img {
  display: block;
  object-fit: cover;
}

.preview-panel__image-empty,
.preview-panel__empty {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.7;
  text-align: center;
}

@media (max-width: 960px) {
  .preview-panel {
    grid-template-columns: minmax(0, 1fr);
  }

  .preview-panel__variable-row {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
