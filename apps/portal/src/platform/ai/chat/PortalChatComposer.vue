<script setup lang="ts">
import { computed, shallowRef } from "vue";

import { formatMultiplier } from "../utils";
import type { PortalChatModelRecord } from "./types";

const draft = defineModel<string>({ default: "" });

const props = withDefaults(
  defineProps<{
    sending?: boolean;
    canSend?: boolean;
    clearIcon?: unknown;
    sendIcon?: unknown;
    hasActiveSession?: boolean;
    models?: PortalChatModelRecord[];
    loadingModels?: boolean;
    selectedModel?: string;
    selectedModelInfo?: PortalChatModelRecord | null;
    sourceLabel?: string;
  }>(),
  {
    sending: false,
    canSend: false,
    hasActiveSession: false,
    models: () => [],
    loadingModels: false,
    selectedModel: "",
    selectedModelInfo: null,
    sourceLabel: "网页对话"
  }
);

const emit = defineEmits<{
  (e: "send"): void;
  (e: "stop"): void;
  (e: "clear"): void;
  (e: "update:selectedModel", value: string): void;
}>();

const disabled = computed(() => !props.canSend);
const composerComposing = shallowRef(false);

function modelOptionKey(model: PortalChatModelRecord) {
  return `${model.group_id || ""}::${model.model_code}`;
}

function modelOptionLabel(model: PortalChatModelRecord) {
  const groupLabel = model.billing_group_label || model.group_name || model.group_id;
  return groupLabel ? `${model.model_code} · ${groupLabel}` : model.model_code;
}

function modelOptionMeta(model: PortalChatModelRecord) {
  const groupLabel = model.billing_group_label || model.group_name || model.group_id || "默认分组";
  const multiplier = typeof model.effective_user_multiplier === "number"
    ? `${formatMultiplier(model.effective_user_multiplier)}x`
    : "";
  return multiplier && !groupLabel.includes(multiplier) ? `${groupLabel} · ${multiplier}` : groupLabel;
}

function handleKeydown(event: KeyboardEvent) {
  if (event.isComposing || composerComposing.value || event.keyCode === 229) return;
  if (event.key === "Enter" && !event.shiftKey) {
    event.preventDefault();
    emit("send");
  }
}
</script>

<template>
  <footer class="composer">
    <div class="composer-box">
      <el-input
        v-model="draft"
        class="composer-input"
        type="textarea"
        :autosize="{ minRows: 1, maxRows: 8 }"
        resize="none"
        placeholder="输入消息，Enter 发送，Shift + Enter 换行"
        @keydown="handleKeydown"
        @compositionstart="composerComposing = true"
        @compositionend="composerComposing = false"
      />

      <div class="composer-controls">
        <el-select
          :model-value="selectedModel"
          class="pill pill-model"
          size="small"
          filterable
          :loading="loadingModels"
          :disabled="hasActiveSession"
          placeholder="选择模型"
          @update:model-value="emit('update:selectedModel', $event as string)"
        >
          <el-option
            v-for="model in models"
            :key="modelOptionKey(model)"
            :label="modelOptionLabel(model)"
            :value="modelOptionKey(model)"
          >
            <div class="model-option">
              <span>{{ model.model_code }}</span>
              <small>{{ modelOptionMeta(model) }}</small>
            </div>
          </el-option>
        </el-select>

        <div class="composer-spacer" />

        <el-button
          v-if="!sending"
          size="small"
          text
          :icon="clearIcon"
          @click="emit('clear')"
        >
          清空
        </el-button>

        <el-button
          v-if="sending"
          size="small"
          type="warning"
          @click="emit('stop')"
        >
          停止
        </el-button>
        <el-button
          v-else
          class="composer-send"
          type="primary"
          circle
          :disabled="disabled"
          @click="emit('send')"
        >
          <el-icon class="composer-send__icon">
            <component :is="sendIcon" />
          </el-icon>
        </el-button>
      </div>
    </div>
  </footer>
</template>

<style scoped>
.composer {
  flex-shrink: 0;
  padding: 12px 20px 16px;
  border-top: 1px solid var(--ds-line);
  background: linear-gradient(0deg, var(--ds-panel), color-mix(in srgb, var(--ds-panel) 45%, var(--ds-paper)));
}

.composer-box {
  max-width: 860px;
  margin: 0 auto;
  border: 1px solid var(--ds-line);
  border-radius: 16px;
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-panel);
  padding: 10px 14px 8px;
  transition: border-color 0.18s ease, box-shadow 0.18s ease;
}

.composer-box:focus-within {
  border-color: color-mix(in srgb, var(--ds-accent) 55%, var(--ds-line));
  box-shadow: 0 0 0 4px var(--ds-accent-soft), var(--ds-shadow-panel);
}

.composer-input :deep(.el-textarea__inner) {
  border: none;
  box-shadow: none;
  padding: 6px 6px 2px;
  background: transparent;
  font-size: 14.5px;
  line-height: 1.6;
  color: var(--ds-ink);
}

.composer-controls {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 8px;
}

.composer-spacer {
  flex: 1;
  min-width: 8px;
}

.pill {
  width: auto;
  min-width: 120px;
}

.pill-model {
  min-width: 168px;
  max-width: 240px;
}

.pill :deep(.el-select__wrapper) {
  border-radius: var(--ds-radius-pill);
}

.composer-send {
  flex-shrink: 0;
  width: 34px;
  height: 34px;
  font-size: 15px;
  font-weight: 700;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}

.composer-send__icon {
  font-size: 15px;
}

.composer-send:not(:disabled):hover {
  transform: scale(1.08);
  box-shadow: var(--ds-shadow-pop);
}


.model-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-width: 0;
}

.model-option span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.model-option small {
  flex-shrink: 0;
  color: var(--ds-muted);
}

</style>
