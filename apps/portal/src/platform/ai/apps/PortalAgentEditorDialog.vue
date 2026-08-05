<script setup lang="ts">
import { computed, reactive, watch } from "vue";

import {
  agentTypeLabel,
  normalizeRuntimeConfig,
  PORTAL_DEFAULT_IMAGE_ASPECT_RATIO,
  PORTAL_DEFAULT_IMAGE_OUTPUT_COUNT,
  PORTAL_DEFAULT_RESOLUTION,
  PORTAL_IMAGE_ASPECT_RATIOS,
  PORTAL_IMAGE_RESOLUTIONS,
  PORTAL_MAX_IMAGE_OUTPUT_COUNT
} from "./contract";
import type { PortalAppCreativity } from "./contract";
import { formatMultiplier } from "../utils";
import type {
  PortalAppModelRecord,
  PortalAppPromptRecord,
  PortalAppRecord,
  PortalAppRuntimeConfig,
  PortalAppScope,
  PortalAppTemplate,
  PortalAppType,
  PortalAppWriteInput
} from "./types";

const props = defineProps<{
  visible: boolean;
  loading: boolean;
  scope: PortalAppScope;
  template: PortalAppTemplate;
  app?: PortalAppRecord | null;
  prompts: PortalAppPromptRecord[];
  models: PortalAppModelRecord[];
  loadingModels?: boolean;
  modelSelectorEnabled: boolean;
}>();

const emit = defineEmits<{
  close: [];
  submit: [payload: PortalAppWriteInput];
}>();

const form = reactive({
  name: "",
  description: "",
  status: "active" as "active" | "disabled",
  capability: "chat" as PortalAppType,
  promptIds: [] as string[],
  groupId: "",
  modelCode: "",
  chatCreativity: "balanced" as PortalAppCreativity,
  chatAllowAttachments: false,
  imageResolution: PORTAL_DEFAULT_RESOLUTION,
  imageAspectRatio: PORTAL_DEFAULT_IMAGE_ASPECT_RATIO,
  imageDefaultOutputCount: PORTAL_DEFAULT_IMAGE_OUTPUT_COUNT,
  imageMaxOutputCount: PORTAL_DEFAULT_IMAGE_OUTPUT_COUNT,
  imageAllowOutputCountOverride: false
});

const title = computed(() => (props.app ? "编辑应用" : `创建${props.template.name}`));
const activePrompts = computed(() => props.prompts.filter((prompt) => prompt.status === "active"));
const needsPrompts = computed(() => props.template.promptStrategy !== "none");
const allowsMultiplePrompts = computed(() => props.template.promptStrategy === "bound_prompt_exact");
const activeModelCapability = computed(() => (form.capability === "chat" ? "chat" : "image"));
const activeModelOptions = computed(() =>
  props.models.filter((model) => normalizeModelCapability(model.capability_type) === activeModelCapability.value && model.status !== "disabled")
);
const selectedPrompts = computed(() => form.promptIds.map((id) => props.prompts.find((prompt) => prompt.id === id)).filter(Boolean) as PortalAppPromptRecord[]);
const selectedVariables = computed(() => [...new Set(selectedPrompts.value.flatMap((prompt) => prompt.variables || []))]);
const selectedModelKey = computed({
  get: () => (form.groupId || form.modelCode ? `${form.groupId}::${form.modelCode}` : ""),
  set: (value: string) => {
    const model = activeModelOptions.value.find((item) => modelOptionKey(item) === value);
    form.groupId = model?.group_id || "";
    form.modelCode = model?.model_code || "";
  }
});
const selectedModel = computed(() => activeModelOptions.value.find((model) => modelOptionKey(model) === selectedModelKey.value) || null);
const selectedModelOutputLimit = computed(() => {
  const configured = form.capability === "image_edit" ? selectedModel.value?.edit_max_output_count : selectedModel.value?.max_output_count;
  return Math.max(1, Math.min(PORTAL_MAX_IMAGE_OUTPUT_COUNT, configured || (props.modelSelectorEnabled ? 1 : PORTAL_MAX_IMAGE_OUTPUT_COUNT)));
});
const imageOutputControlsDisabled = computed(() => Boolean(props.modelSelectorEnabled && (props.loadingModels || !selectedModel.value)));
const promptCountValid = computed(() => {
  const count = form.promptIds.length;
  return count >= props.template.minPromptBindings && count <= props.template.maxPromptBindings;
});
const imageOutputCountValid = computed(() => {
  if (form.capability === "chat") return true;
  if (!Number.isInteger(form.imageDefaultOutputCount) || form.imageDefaultOutputCount < 1 || form.imageDefaultOutputCount > selectedModelOutputLimit.value) return false;
  if (!form.imageAllowOutputCountOverride) return true;
  return Number.isInteger(form.imageMaxOutputCount) && form.imageMaxOutputCount >= form.imageDefaultOutputCount && form.imageMaxOutputCount <= selectedModelOutputLimit.value;
});
const submitDisabled = computed(
  () =>
    !form.name.trim() ||
    !promptCountValid.value ||
    !imageOutputCountValid.value ||
    Boolean(props.modelSelectorEnabled && (props.loadingModels || !selectedModel.value)) ||
    !form.groupId.trim() ||
    !form.modelCode.trim()
);

watch(
  () => [props.visible, props.app?.id, props.template.id] as const,
  ([visible]) => {
    if (!visible) return;
    form.name = props.app?.name || "";
    form.description = props.app?.description || "";
    form.status = props.app?.status || "active";
    form.capability = props.app?.capability || props.template.defaultCapability;
    form.promptIds = props.app?.prompt_bindings?.map((binding) => binding.prompt_id) || [];
    form.groupId = props.app?.group_id || "";
    form.modelCode = props.app?.model_code || "";
    applyRuntimeConfig(normalizeRuntimeConfig(form.capability, props.app?.runtime_config as Record<string, unknown> | undefined));
    ensureCompatibleModelSelection();
  },
  { immediate: true }
);

watch(() => form.capability, ensureCompatibleModelSelection);
watch(activeModelOptions, ensureCompatibleModelSelection);

function normalizeModelCapability(value: string) {
  return value === "image_generation" || value === "image_edit" ? "image" : value;
}

function modelOptionKey(model: Pick<PortalAppModelRecord, "group_id" | "model_code">) {
  return `${model.group_id || ""}::${model.model_code}`;
}

function modelGroupLabel(model: PortalAppModelRecord) {
  return model.billing_group_label || model.group_name || model.group_id || "默认分组";
}

function modelOptionLabel(model: PortalAppModelRecord) {
  const multiplier = typeof model.effective_user_multiplier === "number" ? ` · ${formatMultiplier(model.effective_user_multiplier)}x` : "";
  return `${model.model_code} · ${modelGroupLabel(model)}${multiplier}`;
}

function placeholderLabel(name: string) {
  return `{{${name}}}`;
}

function ensureCompatibleModelSelection() {
  if (!props.visible || !props.modelSelectorEnabled || selectedModel.value) return;
  const model = activeModelOptions.value[0];
  form.groupId = model?.group_id || "";
  form.modelCode = model?.model_code || "";
}

function applyRuntimeConfig(config: PortalAppRuntimeConfig) {
  form.chatCreativity = config.chat?.creativity ?? "balanced";
  form.chatAllowAttachments = Boolean(config.chat?.allow_attachments);
  form.imageResolution = config.image?.resolution ?? PORTAL_DEFAULT_RESOLUTION;
  form.imageAspectRatio = config.image?.aspect_ratio ?? PORTAL_DEFAULT_IMAGE_ASPECT_RATIO;
  form.imageDefaultOutputCount = config.image?.default_output_count ?? PORTAL_DEFAULT_IMAGE_OUTPUT_COUNT;
  form.imageMaxOutputCount = config.image?.max_output_count ?? form.imageDefaultOutputCount;
  form.imageAllowOutputCountOverride = Boolean(config.image?.allow_output_count_override);
}

function collectRuntimeConfig(): PortalAppRuntimeConfig {
  if (form.capability === "chat") {
    return { chat: { creativity: form.chatCreativity, allow_attachments: form.chatAllowAttachments } };
  }
  return {
    image: {
      resolution: form.imageResolution,
      aspect_ratio: form.imageAspectRatio,
      default_output_count: form.imageDefaultOutputCount,
      max_output_count: form.imageAllowOutputCountOverride ? form.imageMaxOutputCount : form.imageDefaultOutputCount,
      allow_output_count_override: form.imageAllowOutputCountOverride
    }
  };
}

function handleSubmit() {
  if (submitDisabled.value) return;
  emit("submit", {
    ...(props.app ? {} : { template_id: props.template.id }),
    name: form.name.trim(),
    description: form.description.trim(),
    status: form.status,
    capability: form.capability,
    prompt_strategy: props.template.promptStrategy,
    prompt_ids: [...form.promptIds],
    group_id: form.groupId.trim(),
    model_code: form.modelCode.trim(),
    runtime_config: collectRuntimeConfig()
  });
}
</script>

<template>
  <el-dialog :model-value="visible" :title="title" width="min(780px, 94vw)" @close="emit('close')">
    <el-form label-position="top">
      <div class="form-grid">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" placeholder="例如：售前咨询助手" />
        </el-form-item>
        <el-form-item label="状态">
          <el-switch v-model="form.status" active-value="active" inactive-value="disabled" active-text="启用" inactive-text="停用" />
        </el-form-item>
      </div>

      <el-form-item label="描述">
        <el-input v-model="form.description" type="textarea" :rows="2" placeholder="说明应用用途与调用边界" />
      </el-form-item>

      <el-form-item v-if="template.allowedCapabilities.length > 1" label="执行能力">
        <el-segmented v-model="form.capability" :options="template.allowedCapabilities.map((value) => ({ value, label: agentTypeLabel(value) }))" />
      </el-form-item>
      <el-form-item v-else label="执行能力">
        <el-tag>{{ agentTypeLabel(form.capability) }}</el-tag>
      </el-form-item>

      <el-form-item v-if="needsPrompts" :label="allowsMultiplePrompts ? '绑定提示词' : '主提示词'" required>
        <el-select
          v-model="form.promptIds"
          class="w-full"
          filterable
          multiple
          collapse-tags
          :multiple-limit="allowsMultiplePrompts ? template.maxPromptBindings : 1"
          :placeholder="allowsMultiplePrompts ? '选择允许动态匹配的提示词' : '选择一条提示词'"
        >
          <el-option v-for="prompt in activePrompts" :key="prompt.id" :label="prompt.name" :value="prompt.id" />
        </el-select>
      </el-form-item>

      <el-form-item v-if="needsPrompts && selectedPrompts.length" :label="allowsMultiplePrompts ? '可用占位符' : '调用变量'">
        <div class="contract-tags">
          <template v-if="allowsMultiplePrompts">
            <el-tag v-for="prompt in selectedPrompts" :key="prompt.id" size="small">{{ placeholderLabel(prompt.name) }}</el-tag>
          </template>
          <template v-else-if="selectedVariables.length">
            <el-tag v-for="variable in selectedVariables" :key="variable" size="small">{{ variable }}</el-tag>
          </template>
          <span v-else class="muted-text">无需 variables</span>
        </div>
      </el-form-item>

      <el-form-item v-if="modelSelectorEnabled" label="模型" required>
        <el-select v-model="selectedModelKey" class="w-full" filterable :loading="loadingModels" :placeholder="`选择${agentTypeLabel(form.capability)}模型`">
          <el-option v-for="model in activeModelOptions" :key="modelOptionKey(model)" :label="modelOptionLabel(model)" :value="modelOptionKey(model)" />
        </el-select>
      </el-form-item>
      <div v-else class="form-grid">
        <el-form-item label="模型编码" required><el-input v-model="form.modelCode" /></el-form-item>
        <el-form-item label="分组 ID" required><el-input v-model="form.groupId" /></el-form-item>
      </div>

      <template v-if="form.capability === 'chat'">
        <!-- 创造性不再对外暴露:沿用应用原值,新建走 balanced 默认 -->
        <el-form-item label="是否支持附件">
          <el-switch v-model="form.chatAllowAttachments" />
        </el-form-item>
      </template>

      <template v-else>
        <div class="form-grid">
          <el-form-item label="输出尺寸">
            <el-select v-model="form.imageResolution" class="w-full">
              <el-option v-for="resolution in PORTAL_IMAGE_RESOLUTIONS" :key="resolution.value" :label="resolution.label" :value="resolution.value" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="form.imageResolution !== 'auto'" label="输出比例">
            <el-select v-model="form.imageAspectRatio" class="w-full" allow-create filterable>
              <el-option v-for="ratio in PORTAL_IMAGE_ASPECT_RATIOS" :key="ratio" :label="ratio" :value="ratio" />
            </el-select>
          </el-form-item>
        </div>
        <div class="form-grid">
          <el-form-item label="默认张数">
            <el-input-number v-model="form.imageDefaultOutputCount" :min="1" :max="selectedModelOutputLimit" :disabled="imageOutputControlsDisabled" />
          </el-form-item>
          <el-form-item label="调用方可调整">
            <el-switch v-model="form.imageAllowOutputCountOverride" :disabled="imageOutputControlsDisabled" />
          </el-form-item>
          <el-form-item v-if="form.imageAllowOutputCountOverride" label="最大张数">
            <el-input-number v-model="form.imageMaxOutputCount" :min="form.imageDefaultOutputCount" :max="selectedModelOutputLimit" :disabled="imageOutputControlsDisabled" />
          </el-form-item>
        </div>
      </template>
    </el-form>

    <template #footer>
      <el-button @click="emit('close')">取消</el-button>
      <el-button type="primary" :loading="loading" :disabled="submitDisabled" @click="handleSubmit">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.w-full {
  width: 100%;
}

.contract-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.muted-text {
  color: var(--ds-muted);
  font-size: 13px;
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: minmax(0, 1fr);
    gap: 0;
  }
}
</style>
