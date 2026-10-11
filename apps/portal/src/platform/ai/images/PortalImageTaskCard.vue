<script setup lang="ts">
import { CircleClose, Coin, CopyDocument, Delete, Download, RefreshRight } from "@element-plus/icons-vue";
import type { PortalImageJobCard, PortalImagePreviewAsset } from "./imageJobPresentation";
import type { PortalImageJobRecord } from "./types";

defineProps<{ card: PortalImageJobCard; cancelling: boolean; deleting: boolean; formatUSD: (value: number | null | undefined) => string }>();
const emit = defineEmits<{
  preview: [src: string];
  download: [src: string];
  downloadAll: [images: PortalImagePreviewAsset[]];
  copy: [job: PortalImageJobRecord];
  retry: [job: PortalImageJobRecord];
  cancel: [job: PortalImageJobRecord];
  delete: [job: PortalImageJobRecord];
  togglePrompt: [job: PortalImageJobRecord];
}>();
</script>

<template>
  <article class="task-card" :class="{ active: card.isActive }">
    <div class="task-visual">
      <div v-if="card.isActive || card.awaitingDetails" class="skeleton-grid">
        <span />
      </div>
      <div v-else-if="card.images.length" class="thumb-grid">
        <button v-for="image in card.images" :key="image.preview" class="thumb" type="button" @click="emit('preview', image.preview)">
          <img :src="image.preview" alt="" loading="lazy" decoding="async" />
          <span class="thumb-action" @click.stop="emit('download', image.original)">下载</span>
        </button>
      </div>
      <div v-else class="visual-empty" :class="[`visual-empty--${card.visualTone}`]">
        <strong>{{ card.visualLabel }}</strong>
        <small v-if="card.visualHint">{{ card.visualHint }}</small>
      </div>
    </div>

    <div class="task-body">
      <div class="task-head">
        <span :class="card.statusClass">
          <i class="status-dot" />
          {{ card.statusLabel }}
        </span>
        <div class="task-quick-tools" role="toolbar" aria-label="任务操作">
          <el-tooltip :content="card.copyTooltip" placement="top">
            <span class="task-quick-tools__item">
              <el-button
                circle
                plain
                size="small"
                class="task-icon-button task-icon-button--copy"
                :icon="CopyDocument"
                :disabled="!card.canCopyPrompt"
                aria-label="复制提示词"
                @click="emit('copy', card.job)"
              />
            </span>
          </el-tooltip>
          <el-tooltip :content="card.downloadTooltip" placement="top">
            <span class="task-quick-tools__item">
              <el-button
                circle
                plain
                size="small"
                class="task-icon-button"
                :icon="Download"
                :disabled="!card.canDownload"
                aria-label="下载原图"
                @click="emit('downloadAll', card.images)"
              />
            </span>
          </el-tooltip>
          <el-tooltip :content="card.retryTooltip" placement="top">
            <span class="task-quick-tools__item">
              <el-button
                circle
                plain
                size="small"
                class="task-icon-button"
                :icon="RefreshRight"
                :disabled="!card.canRetry"
                aria-label="重试"
                @click="emit('retry', card.job)"
              />
            </span>
          </el-tooltip>
          <el-tooltip v-if="card.isActive" :content="card.cancelTooltip" placement="top">
            <span class="task-quick-tools__item">
              <el-button
                circle
                plain
                size="small"
                class="task-icon-button task-icon-button--cancel"
                :icon="CircleClose"
                :loading="cancelling"
                :disabled="!card.canCancel"
                aria-label="取消任务"
                @click="emit('cancel', card.job)"
              />
            </span>
          </el-tooltip>
          <el-tooltip v-else :content="card.deleteTooltip" placement="top">
            <span class="task-quick-tools__item">
              <el-button
                circle
                plain
                size="small"
                class="task-icon-button task-icon-button--delete"
                :icon="Delete"
                :loading="deleting"
                :disabled="!card.canDelete"
                aria-label="删除任务记录"
                @click="emit('delete', card.job)"
              />
            </span>
          </el-tooltip>
        </div>
      </div>

      <div class="task-prompt-wrap">
        <p
          class="task-prompt"
          :class="{
            'task-prompt--expanded': card.isPromptExpanded,
            'task-prompt--placeholder': !card.promptText
          }"
        >
          {{ card.promptText || "-" }}
        </p>
        <button
          v-if="card.showPromptToggle"
          type="button"
          class="task-prompt-toggle"
          @click="emit('togglePrompt', card.job)"
        >
          {{ card.isPromptExpanded ? "收起" : "展开" }}
        </button>
      </div>
      <div class="task-meta">
        <span class="meta-tag meta-tag--model">{{ card.job.model_code }}</span>
        <span class="meta-tag" :class="card.job.operation === 'edit' ? 'meta-tag--edit' : 'meta-tag--gen'">
          {{ card.job.operation === "edit" ? "参考图" : "文生图" }}
        </span>
        <span v-if="card.job.size" class="meta-tag meta-tag--size">{{ card.job.size }}</span>
        <span v-if="card.awaitingDetails || card.job.status === 'completed'" class="cost-chip">
          <el-icon><Coin /></el-icon>{{ card.awaitingDetails ? "费用待同步" : formatUSD(card.job.caller_charge_usd) }}
        </span>
        <span class="task-time">{{ card.createdAtText }}</span>
      </div>

      <template v-if="card.isActive">
        <div class="progress indeterminate"><i /></div>
        <div class="progress-note">
          <span>{{ card.awaitingDetails ? "任务已受理，等待状态同步" : card.job.status === "pending" ? "等待执行" : "模型推理中" }}</span>
          <span v-if="!card.awaitingDetails">已用 {{ card.elapsedText }}</span>
        </div>
      </template>
      <div v-else-if="card.isFailed" class="fail-box">{{ card.job.error_message || "任务未完成" }}</div>
    </div>
  </article>
</template>

<style scoped>
.task-card {
  display: grid;
  grid-template-columns: 104px minmax(0, 1fr);
  gap: 16px;
  border: 1px solid transparent;
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  padding: 14px 16px;
  box-shadow: var(--ds-shadow-sm);
  transition: box-shadow 0.18s ease, transform 0.18s ease, border-color 0.18s ease;
}

.task-card:hover {
  border-color: var(--ds-line);
  box-shadow: var(--ds-shadow-panel);
  transform: translateY(-1px);
}

.task-card.active {
  border-color: color-mix(in srgb, var(--ds-accent) 38%, transparent);
  box-shadow: var(--ds-shadow-focus-sm);
}

.task-visual {
  width: 104px;
}

.task-body {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.task-head,
.progress-note {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.task-status {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border-radius: var(--ds-radius-pill);
  padding: 2px 9px;
  font-size: 12px;
  font-weight: 700;
}

.task-status.run {
  color: var(--ds-info);
  background: var(--ds-info-soft);
}

.task-status.queue {
  color: var(--ds-warning);
  background: var(--ds-warning-soft);
}

.task-status.done {
  color: var(--ds-positive);
  background: var(--ds-positive-soft);
}

.task-status.fail {
  color: var(--ds-danger);
  background: var(--ds-danger-soft);
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: var(--ds-radius-circle);
  background: currentColor;
}

.task-time {
  margin-left: auto;
  flex-shrink: 0;
  color: var(--ds-faint);
  font-size: 12px;
}

.progress-note {
  color: var(--ds-faint);
  font-size: 12px;
}

.task-prompt {
  margin: 9px 0 7px;
  overflow: hidden;
  color: var(--ds-ink-soft);
  font-size: 13.5px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-word;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.task-prompt-wrap {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
}

.task-prompt--expanded {
  display: block;
  overflow: visible;
  -webkit-line-clamp: unset;
}

.task-prompt--placeholder {
  color: var(--ds-faint);
}

.task-prompt-toggle {
  margin: -2px 0 7px;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--ds-accent-hover);
  font-size: 12px;
  font-weight: 600;
  line-height: 1.4;
  cursor: pointer;
  transition: color 0.15s ease, opacity 0.15s ease;
}

.task-prompt-toggle:hover {
  color: var(--ds-accent);
}

.task-prompt-toggle:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--ds-accent) 45%, transparent);
  outline-offset: 2px;
  border-radius: var(--ds-radius-xs);
}

.task-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.meta-tag {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: var(--ds-radius-pill);
  font-size: 11px;
  font-weight: 600;
  line-height: 1.6;
  white-space: nowrap;
}

.meta-tag--model {
  color: var(--ds-info);
  background: var(--ds-info-soft);
}

.meta-tag--gen {
  color: var(--ds-positive);
  background: var(--ds-positive-soft);
}

.meta-tag--edit {
  color: var(--ds-warning);
  background: var(--ds-warning-soft);
}

.meta-tag--size {
  color: var(--ds-ink-soft);
  background: var(--ds-panel-muted);
  font-variant-numeric: tabular-nums;
}

.cost-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 9px;
  border-radius: var(--ds-radius-pill);
  border: 1px solid color-mix(in srgb, var(--ds-accent) 22%, transparent);
  color: var(--ds-accent);
  background: var(--ds-accent-soft);
  font-size: 12px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.cost-chip .el-icon {
  font-size: 13px;
}

.progress {
  height: 5px;
  margin: 12px 0 6px;
  overflow: hidden;
  border-radius: var(--ds-radius-pill);
  background: var(--ds-panel-muted);
}

.progress i {
  display: block;
  height: 100%;
  border-radius: var(--ds-radius-inherit);
  background: var(--ds-accent);
}

.progress.indeterminate i {
  width: 40%;
  background: linear-gradient(90deg, transparent, var(--ds-accent), transparent);
  animation: indeterminate 1.25s ease-in-out infinite;
}

.skeleton-grid,
.thumb-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(47px, 1fr));
  gap: 5px;
}

.visual-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 3px;
  aspect-ratio: 1;
  border: 1px dashed var(--ds-line-strong);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
  color: var(--ds-faint);
  font-size: 11px;
}

.visual-empty strong {
  font-size: 12px;
  font-weight: 700;
  line-height: 1.35;
}

.visual-empty small {
  max-width: 90px;
  line-height: 1.4;
  text-align: center;
}

.visual-empty--fail {
  border-color: color-mix(in srgb, var(--ds-danger) 30%, var(--ds-line));
  background: var(--ds-danger-soft);
  color: var(--ds-danger);
}

.visual-empty--success {
  border-color: color-mix(in srgb, var(--ds-positive) 28%, var(--ds-line));
  background: var(--ds-positive-soft);
  color: color-mix(in srgb, var(--ds-positive) 72%, var(--ds-ink));
}

.skeleton-grid span,
.thumb {
  aspect-ratio: 1;
  border-radius: var(--ds-radius-control);
}

.skeleton-grid span {
  background: linear-gradient(100deg, var(--ds-panel-muted) 30%, var(--ds-surface-soft) 50%, var(--ds-panel-muted) 70%);
  background-size: 200% 100%;
  animation: shimmer 1.4s infinite;
}

.thumb {
  position: relative;
  overflow: hidden;
  border: 1px solid var(--ds-line);
  background: var(--ds-panel-muted);
  cursor: pointer;
  transition: transform 0.15s ease;
}

.thumb:hover {
  transform: scale(1.03);
}

.thumb img {
  width: 100%;
  height: 100%;
  display: block;
  object-fit: cover;
}

.thumb-action {
  position: absolute;
  right: 5px;
  bottom: 5px;
  border-radius: var(--ds-radius-sm);
  background: color-mix(in srgb, var(--ds-ink) 78%, transparent);
  color: var(--ds-white);
  font-size: 11px;
  padding: 2px 6px;
  opacity: 0;
  transition: opacity 0.15s ease;
}

.thumb:hover .thumb-action {
  opacity: 1;
}

.task-quick-tools {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
  padding: 4px;
  border: 1px solid color-mix(in srgb, var(--ds-line-strong) 55%, transparent);
  border-radius: var(--ds-radius-pill);
  background: color-mix(in srgb, var(--ds-panel-muted) 72%, var(--ds-white));
  box-shadow: var(--ds-shadow-inset-highlight);
  opacity: 0.55;
  transition: opacity 0.18s ease;
}

.task-card:hover .task-quick-tools,
.task-card.active .task-quick-tools {
  opacity: 1;
}

.task-quick-tools__item {
  display: inline-flex;
}

.task-icon-button {
  width: 30px;
  min-width: 30px;
  height: 30px;
  border-color: transparent;
  background: transparent;
  color: var(--ds-muted);
  box-shadow: none;
  transition: transform 0.15s ease, background-color 0.15s ease, color 0.15s ease;
}

.task-icon-button:hover:not(:disabled) {
  transform: translateY(-1px);
  background: color-mix(in srgb, var(--ds-panel) 80%, var(--ds-white));
  color: var(--ds-ink);
}

.task-icon-button--copy {
  color: var(--ds-accent-hover);
}

.task-icon-button--copy:hover:not(:disabled) {
  background: color-mix(in srgb, var(--ds-accent-soft) 82%, var(--ds-white));
  color: var(--ds-accent-hover);
}

.task-icon-button--cancel,
.task-icon-button--delete {
  color: color-mix(in srgb, var(--ds-danger) 74%, var(--ds-ink));
}

.task-icon-button--cancel:hover:not(:disabled),
.task-icon-button--delete:hover:not(:disabled) {
  background: color-mix(in srgb, var(--ds-danger-soft) 80%, var(--ds-white));
  color: var(--ds-danger);
}

.task-icon-button:disabled {
  background: transparent;
  color: var(--ds-faint);
  opacity: 1;
}

.fail-box {
  margin: 8px 0;
  border-radius: var(--ds-radius-control);
  background: var(--ds-danger-soft);
  color: color-mix(in srgb, var(--ds-danger) 72%, var(--ds-black));
  padding: 8px 11px;
  font-size: 12px;
  line-height: 1.5;
}

@keyframes indeterminate {
  from {
    margin-left: -40%;
  }
  to {
    margin-left: 100%;
  }
}

@keyframes shimmer {
  from {
    background-position: 200% 0;
  }
  to {
    background-position: -200% 0;
  }
}

@media (max-width: 560px) {

  .task-card {
    grid-template-columns: 1fr;
  }

  .task-visual {
    width: 160px;
  }
}
</style>
