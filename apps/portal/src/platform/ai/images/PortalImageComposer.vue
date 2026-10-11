<script setup lang="ts">
import { Delete, FullScreen } from "@element-plus/icons-vue";
import PortalImageReferenceTray from "./PortalImageReferenceTray.vue";
import { usePortalImageComposer, type PortalImageComposerProps } from "./usePortalImageComposer";

const props = defineProps<PortalImageComposerProps>();
const emit = defineEmits<{ preview: [src: string] }>();
const {
  sourceImageReferences,
  handleAddReferences,
  handleRemoveReference,
  handleMoveReference,
  insertReferenceToken,
  activeForm,
  promptPlaceholder,
  handleComposerKeydown,
  composerComposing,
  promptEditorExpanded,
  ui,
  operationOptions,
  modelOptionKey,
  modelOptionLabel,
  modelOptionMeta,
  maskImageFile,
  activeOutputCountMax,
  resolutionOptions,
  aspectRatioOptions,
  maskImageInputKey,
  onMaskImageChange,
  maskImageSummary,
  clearMaskImage,
  canSubmit,
  submit,
  submitIconComponent,
  insertExpandedReferenceToken,
  submitFromExpandedEditor
} = usePortalImageComposer(props);
</script>

<template>
  <footer class="composer">
    <div class="composer-box">
      <PortalImageReferenceTray
        class="composer-reference-tray"
        :references="sourceImageReferences"
        @add="handleAddReferences"
        @remove="handleRemoveReference"
        @move="handleMoveReference"
        @preview="emit('preview', $event)"
        @insert-reference="insertReferenceToken"
      />

      <div class="composer-input-shell">
        <el-input
          ref="composerInputRef"
          v-model="activeForm.prompt"
          class="composer-input"
          type="textarea"
          :autosize="{ minRows: 1, maxRows: 8 }"
          resize="none"
          :placeholder="promptPlaceholder"
          @keydown="handleComposerKeydown"
          @compositionstart="composerComposing = true"
          @compositionend="composerComposing = false"
        />
        <el-tooltip content="展开编辑" placement="top">
          <el-button
            class="composer-expand"
            circle
            text
            :icon="FullScreen"
            aria-label="展开编辑提示词"
            @click="promptEditorExpanded = true"
          />
        </el-tooltip>
      </div>

      <div class="composer-controls">
        <el-segmented
          v-model="ui.operation"
          size="small"
          :options="operationOptions"
        />
        <el-select v-model="activeForm.model" class="pill pill-model" size="small" filterable placeholder="选择模型">
          <el-option v-for="model in models" :key="modelOptionKey(model)" :label="modelOptionLabel(model)" :value="modelOptionKey(model)">
            <div class="model-option">
              <span>{{ model.model_code }}</span>
              <small>{{ modelOptionMeta(model) }}</small>
            </div>
          </el-option>
        </el-select>

        <el-popover placement="top" :width="320" trigger="click">
          <template #reference>
            <el-button size="small" class="pill-button" :class="{ 'pill-button--active': maskImageFile }">
              高级设置{{ maskImageFile ? " · 1" : "" }}
            </el-button>
          </template>
          <div class="advanced-settings-panel">
            <div class="advanced-settings-row">
              <span class="advanced-settings-panel__label">张数</span>
              <el-input-number
                v-model="activeForm.n"
                size="small"
                :min="1"
                :max="activeOutputCountMax"
                controls-position="right"
              />
            </div>

            <div class="advanced-settings-row">
              <span class="advanced-settings-panel__label">输出尺寸</span>
              <el-select v-model="activeForm.resolution" class="advanced-settings-resolution" size="small">
                <el-option v-for="resolution in resolutionOptions" :key="resolution.value" :label="resolution.label" :value="resolution.value" />
              </el-select>
            </div>

            <div v-if="activeForm.resolution !== 'auto'" class="advanced-settings-row">
              <span class="advanced-settings-panel__label">输出比例</span>
              <el-select v-model="activeForm.aspectRatio" class="advanced-settings-resolution" size="small">
                <el-option v-for="ratio in aspectRatioOptions" :key="ratio" :label="ratio" :value="ratio" />
              </el-select>
            </div>

            <div v-if="ui.operation === 'edit'" class="mask-panel">
              <span class="advanced-settings-panel__label">蒙版</span>
              <div class="mask-panel__control">
                <label class="mask-upload" :class="{ 'mask-upload--filled': maskImageFile }">
                  <input :key="maskImageInputKey" type="file" accept="image/*" @change="onMaskImageChange" />
                  <span :title="maskImageSummary">{{ maskImageSummary }}</span>
                </label>
                <el-button
                  v-if="maskImageFile"
                  text
                  type="danger"
                  :icon="Delete"
                  aria-label="移除蒙版"
                  @click="clearMaskImage"
                />
              </div>
            </div>
          </div>
        </el-popover>

        <div class="composer-spacer" />

        <el-button
          class="composer-send"
          type="primary"
          circle
          aria-label="生成图片"
          :loading="submitting"
          :disabled="!canSubmit"
          @click="submit"
        >
          <el-icon class="composer-send__icon">
            <component :is="submitIconComponent" />
          </el-icon>
        </el-button>
      </div>
    </div>
  </footer>

  <el-dialog v-model="promptEditorExpanded" title="编辑提示词" width="min(900px, calc(100vw - 32px))" append-to-body>
    <div class="expanded-editor">
      <PortalImageReferenceTray
        class="expanded-editor__references"
        :references="sourceImageReferences"
        @add="handleAddReferences"
        @remove="handleRemoveReference"
        @move="handleMoveReference"
        @preview="emit('preview', $event)"
        @insert-reference="insertExpandedReferenceToken"
      />
      <el-input
        ref="expandedPromptInputRef"
        v-model="activeForm.prompt"
        type="textarea"
        :rows="12"
        resize="vertical"
        :placeholder="promptPlaceholder"
      />
    </div>
    <template #footer>
      <el-button @click="promptEditorExpanded = false">完成</el-button>
      <el-button type="primary" :loading="submitting" :disabled="!canSubmit" @click="submitFromExpandedEditor">生成</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.composer {
  flex-shrink: 0;
  padding: 12px 20px 18px;
  border-top: 1px solid var(--ds-line);
  background: linear-gradient(0deg, var(--ds-panel), color-mix(in srgb, var(--ds-panel) 45%, var(--ds-paper)));
}

.composer-box {
  box-sizing: border-box;
  width: min(100%, var(--studio-content-width));
  margin: 0 auto;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-xl);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-panel);
  padding: 12px 14px 10px;
  transition: border-color 0.18s ease, box-shadow 0.18s ease;
}

.composer-box:focus-within {
  border-color: color-mix(in srgb, var(--ds-accent) 55%, var(--ds-line));
  box-shadow: var(--ds-shadow-focus-panel);
}

.composer-reference-tray {
  margin-bottom: 8px;
}

.composer-input-shell {
  position: relative;
}

.composer-input :deep(.el-textarea__inner) {
  border: none;
  box-shadow: none;
  padding: 6px 42px 2px 6px;
  background: transparent;
  font-size: 14.5px;
  line-height: 1.6;
  color: var(--ds-ink);
}

.composer-expand {
  position: absolute;
  top: 2px;
  right: 2px;
  width: 30px;
  min-width: 30px;
  height: 30px;
  color: var(--ds-muted);
}

.composer-expand:hover {
  color: var(--ds-accent);
  background: var(--ds-accent-soft);
}

.composer-controls {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 8px;
}

.composer-controls :deep(.el-segmented) {
  --el-segmented-item-selected-bg-color: var(--ds-accent);
  --el-segmented-item-selected-color: var(--ds-white);
  border-radius: var(--ds-radius-pill);
  padding: 2px;
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
  max-width: 232px;
}

.pill :deep(.el-select__wrapper),
.pill-button {
  border-radius: var(--ds-radius-pill);
}

.pill-button--active {
  border-color: color-mix(in srgb, var(--ds-accent) 55%, var(--ds-line));
  color: var(--ds-accent);
  background: var(--ds-accent-soft);
}

.composer-send {
  flex-shrink: 0;
  width: 36px;
  height: 36px;
  font-size: 16px;
  font-weight: 700;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}

.composer-send__icon {
  font-size: 16px;
}

.composer-send:not(:disabled):hover {
  transform: scale(1.08);
  box-shadow: var(--ds-shadow-pop);
}

.advanced-settings-panel,
.mask-panel {
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.advanced-settings-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.advanced-settings-panel__label {
  color: var(--ds-ink-soft);
  font-size: 12px;
  font-weight: 700;
}

.advanced-settings-row :deep(.el-input-number) {
  width: 108px;
}

.advanced-settings-resolution {
  width: 168px;
}

.mask-panel__control {
  display: flex;
  align-items: center;
  gap: 6px;
}

.mask-upload {
  position: relative;
  overflow: hidden;
  display: flex;
  align-items: center;
  min-width: 0;
  flex: 1;
  height: 34px;
  padding: 0 11px;
  border: 1px dashed var(--ds-line-strong);
  border-radius: var(--ds-radius-sm);
  color: var(--ds-muted);
  font-size: 12px;
  cursor: pointer;
}

.mask-upload--filled {
  border-style: solid;
  border-color: var(--ds-accent);
  color: var(--ds-accent);
}

.mask-upload input {
  position: absolute;
  inset: 0;
  opacity: 0;
  cursor: pointer;
}

.mask-upload span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.expanded-editor {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.expanded-editor__references {
  min-width: 0;
}

.expanded-editor :deep(.el-textarea__inner) {
  font-size: 14px;
  line-height: 1.7;
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

@media (max-width: 560px) {

  .composer {
    padding: 10px 10px 12px;
  }

  .composer-box {
    padding: 10px;
  }

  .composer-controls {
    gap: 6px;
  }

  .pill-model {
    max-width: 100%;
    flex: 1 1 170px;
  }
}
</style>
