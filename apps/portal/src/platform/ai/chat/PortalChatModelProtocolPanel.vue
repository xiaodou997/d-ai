<script setup lang="ts">
import { computed, shallowRef } from "vue";

import type { PortalChatModelRecord } from "./types";

const props = withDefaults(
  defineProps<{
    models?: PortalChatModelRecord[];
    loadingModels?: boolean;
    selectedModel?: string;
    protocolPolicy?: string;
    selectedProtocol?: string;
    temperature?: number;
    maxTokens?: number;
    showAdvanced?: boolean;
    selectedModelInfo?: PortalChatModelRecord | null;
    activeProtocolLabel?: string;
    protocolLabels: Record<string, string>;
    sourceLabel?: string;
    collapseOpenIcon?: unknown;
    collapseClosedIcon?: unknown;
    settingsIcon?: unknown;
    noteIcon?: unknown;
  }>(),
  {
    models: () => [],
    loadingModels: false,
    selectedModel: "",
    protocolPolicy: "auto",
    selectedProtocol: "",
    temperature: 0.7,
    maxTokens: 2048,
    showAdvanced: false,
    selectedModelInfo: null,
    activeProtocolLabel: "自动",
    sourceLabel: "网页对话"
  }
);

const emit = defineEmits<{
  (e: "update:selectedModel", value: string): void;
  (e: "update:protocolPolicy", value: string): void;
  (e: "update:selectedProtocol", value: string): void;
  (e: "update:temperature", value: number): void;
  (e: "update:maxTokens", value: number): void;
  (e: "update:showAdvanced", value: boolean): void;
}>();

const protocolOptions = computed(() =>
  (props.selectedModelInfo?.available_api_formats || []).map((protocol) => ({
    label: props.protocolLabels[protocol] || protocol,
    value: protocol
  }))
);

const panelOpen = shallowRef(true);

function modelOptionKey(model: PortalChatModelRecord) {
  return `${model.group_id || ""}::${model.model_code}`;
}

function modelOptionLabel(model: PortalChatModelRecord) {
  const groupLabel = model.billing_group_label || model.group_name || model.group_id;
  return groupLabel ? `${model.model_code} · ${groupLabel}` : model.model_code;
}
</script>

<template>
  <section class="model-panel">
    <header class="panel-head">
      <div class="panel-title">
        <span>模型与 API 格式</span>
        <strong>{{ selectedModelInfo ? modelOptionLabel(selectedModelInfo) : "未选择模型" }}</strong>
        <small>{{ activeProtocolLabel }} · {{ protocolPolicy === "auto" ? "自动" : "指定" }}</small>
      </div>
      <el-button
        link
        type="primary"
        :icon="panelOpen ? collapseOpenIcon : collapseClosedIcon"
        @click="panelOpen = !panelOpen"
      >
        {{ panelOpen ? "收起" : "展开" }}
      </el-button>
    </header>

    <div v-show="panelOpen" class="panel-body">
      <label class="field-label">模型</label>
      <el-select
        class="w-full"
        filterable
        :loading="loadingModels"
        :model-value="selectedModel"
        placeholder="选择模型"
        @update:model-value="emit('update:selectedModel', $event as string)"
      >
        <el-option v-for="model in models" :key="modelOptionKey(model)" :label="modelOptionLabel(model)" :value="modelOptionKey(model)">
          <div class="model-option">
            <span>{{ model.model_code }}</span>
            <small>{{ model.billing_group_label || model.group_name || model.group_id }} · Auto: {{ protocolLabels[model.default_api_format] || model.default_api_format }}</small>
          </div>
        </el-option>
      </el-select>

      <div class="protocol-card">
        <span>当前 API 格式</span>
        <strong>{{ activeProtocolLabel }}</strong>
        <small>{{ protocolPolicy === "auto" ? "后台自动选择" : "高级模式指定" }}</small>
      </div>

      <button class="advanced-toggle" type="button" @click="emit('update:showAdvanced', !showAdvanced)">
        <el-icon v-if="settingsIcon"><component :is="settingsIcon" /></el-icon>
        <span>高级设置</span>
      </button>

      <div v-show="showAdvanced" class="advanced-panel">
        <label class="field-label">API 格式策略</label>
        <el-segmented
          class="w-full"
          :model-value="protocolPolicy"
          :options="[
            { label: '自动', value: 'auto' },
            { label: '指定', value: 'manual' }
          ]"
          @update:model-value="emit('update:protocolPolicy', $event as string)"
        />
        <template v-if="protocolPolicy === 'manual'">
          <label class="field-label mt-3">指定 API 格式</label>
          <el-select
            class="w-full"
            :model-value="selectedProtocol"
            @update:model-value="emit('update:selectedProtocol', $event as string)"
          >
            <el-option v-for="item in protocolOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </template>
        <label class="field-label mt-3">Temperature</label>
        <el-slider
          :model-value="temperature"
          :min="0"
          :max="2"
          :step="0.1"
          @update:model-value="emit('update:temperature', $event as number)"
        />
        <!-- 最大输出不再作为用户可控项：由模型自行决定（对话与应用均不暴露）。 -->
      </div>

      <div class="source-note">
        <el-icon v-if="noteIcon"><component :is="noteIcon" /></el-icon>
        <span>{{ sourceLabel }}</span>
      </div>
    </div>
  </section>
</template>

<style scoped>
.model-panel {
  flex-shrink: 0;
  padding: 16px;
  background: var(--ds-white);
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-shell);
}

.panel-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.panel-title {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.panel-title span {
  color: var(--ds-ink-soft);
  font-size: 12px;
  font-weight: 900;
}

.panel-title strong {
  overflow: hidden;
  color: var(--ds-ink);
  font-size: 15px;
  font-weight: 900;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.panel-title small {
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 800;
}

.panel-body {
  margin-top: 14px;
}

.field-label {
  display: block;
  margin: 0 0 8px;
  color: var(--ds-ink-soft);
  font-size: 12px;
  font-weight: 800;
}

.mt-3 {
  margin-top: 12px;
}

.model-option {
  display: flex;
  justify-content: space-between;
  gap: 18px;
}

.model-option small {
  color: var(--ds-muted);
}

.protocol-card {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-top: 14px;
  padding: 12px;
  border: 1px solid color-mix(in srgb, var(--ds-accent) 30%, transparent);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-accent-soft);
}

.protocol-card span,
.protocol-card small {
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 800;
}

.protocol-card strong {
  color: var(--ds-accent-hover);
  font-size: 14px;
  font-weight: 900;
}

.advanced-toggle {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 16px 0 10px;
  padding: 10px 0;
  color: var(--ds-ink-soft);
  font-size: 13px;
  font-weight: 800;
  background: transparent;
  border: 0;
  cursor: pointer;
}

.advanced-panel {
  padding: 12px;
  background: var(--ds-panel-muted);
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
}

.source-note {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 16px;
  padding: 10px 12px;
  color: var(--ds-accent-hover);
  font-size: 12px;
  font-weight: 700;
  border-radius: var(--ds-radius-panel);
  background: var(--ds-accent-soft);
}
</style>
