<script setup lang="ts">
import { ArrowUp, CircleClose, Coin, CopyDocument, Delete, Download, FullScreen, RefreshRight } from "@element-plus/icons-vue";
import { computed, nextTick, onMounted, reactive, shallowRef, useTemplateRef, watch } from "vue";
import type { InputInstance } from "element-plus";

import {
  PORTAL_DEFAULT_IMAGE_ASPECT_RATIO,
  PORTAL_IMAGE_ASPECT_RATIOS,
  PORTAL_IMAGE_RESOLUTIONS
} from "./contract";
import { formatMultiplier } from "../utils";

import PortalImageReferenceTray from "./PortalImageReferenceTray.vue";
import { resolveOpenAIImageSize } from "./openaiImageSizing";
import { usePortalImageReferences } from "./usePortalImageReferences";
import { PORTAL_IMAGE_ACTIVE_STATUSES, usePortalImageTaskQueue } from "./usePortalImageTaskQueue";
import type { PortalImageApi, PortalImageJobRecord, PortalImageModelRecord } from "./types";
import type { PortalImageReferenceMove } from "./usePortalImageReferences";

const props = withDefaults(
  defineProps<{
    api: PortalImageApi;
    formatUSD: (value: number | null | undefined) => string;
    usageMessage: string;
    refreshIcon?: unknown;
    submitIcon?: unknown;
    assetBaseUrl?: string;
    pollIntervalMs?: number;
    confirmDelete?: (job: PortalImageJobRecord) => boolean | Promise<boolean>;
    notifySuccess?: (message: string) => void;
    notifyError?: (message: string) => void;
  }>(),
  {
    usageMessage: "消耗会计入当前用量",
    pollIntervalMs: 20_000
  }
);

interface PortalImagePreviewAsset {
  preview: string;
  original: string;
}

interface PortalImageJobCard {
  job: PortalImageJobRecord;
  images: PortalImagePreviewAsset[];
  isActive: boolean;
  isFailed: boolean;
  isPromptExpanded: boolean;
  promptText: string;
  showPromptToggle: boolean;
  canCopyPrompt: boolean;
  canDownload: boolean;
  canRetry: boolean;
  canCancel: boolean;
  canDelete: boolean;
  copyTooltip: string;
  downloadTooltip: string;
  retryTooltip: string;
  cancelTooltip: string;
  deleteTooltip: string;
  statusLabel: string;
  statusClass: string;
  createdAtText: string;
  elapsedText: string;
  visualLabel: string;
  visualHint: string;
  visualTone: "fail" | "success" | "neutral";
}

const models = shallowRef<PortalImageModelRecord[]>([]);
const jobs = shallowRef<PortalImageJobRecord[]>([]);
const loading = shallowRef(false);
const submitting = shallowRef(false);
const cancellingJobId = shallowRef("");
const deletingJobId = shallowRef("");
const activeFilter = shallowRef<"all" | "active" | "completed" | "failed">("all");
const previewImage = shallowRef("");
const promptEditorExpanded = shallowRef(false);
const resultStreamRef = shallowRef<HTMLElement | null>(null);
const maskImageInputKey = shallowRef(0);
const composerComposing = shallowRef(false);
const expandedPromptState = shallowRef<Record<string, boolean>>({});
const composerInputRef = useTemplateRef<InputInstance>("composerInputRef");
const expandedPromptInputRef = useTemplateRef<InputInstance>("expandedPromptInputRef");
const {
  references: sourceImageReferences,
  mask: maskImageReference,
  addReferences,
  removeReference,
  moveReference,
  setMask,
  resetReferences
} = usePortalImageReferences();

const ui = reactive({
  operation: "generation" as "generation" | "edit"
});

const generationForm = reactive({
  model: "",
  prompt: "",
  n: 1,
  resolution: "auto",
  aspectRatio: PORTAL_DEFAULT_IMAGE_ASPECT_RATIO
});

const editForm = reactive({
  model: "",
  prompt: "",
  n: 1,
  resolution: "auto",
  aspectRatio: PORTAL_DEFAULT_IMAGE_ASPECT_RATIO
});

const resolutionOptions = PORTAL_IMAGE_RESOLUTIONS;
const aspectRatioOptions = PORTAL_IMAGE_ASPECT_RATIOS;
const activeStatuses = PORTAL_IMAGE_ACTIVE_STATUSES;

const selectedGenerationModel = computed(() => models.value.find((item) => modelOptionKey(item) === generationForm.model) || null);
const selectedEditModel = computed(() => models.value.find((item) => modelOptionKey(item) === editForm.model) || null);

// —— 供底部 Composer 统一取用（免去 generation/edit 双写模板）——
const activeForm = computed(() => (ui.operation === "generation" ? generationForm : editForm));
const activeOutputCountMax = computed(() => {
  const model = ui.operation === "generation" ? selectedGenerationModel.value : selectedEditModel.value;
  return ui.operation === "generation" ? model?.max_output_count || 1 : model?.edit_max_output_count || 1;
});
const submitIconComponent = computed(() => props.submitIcon || ArrowUp);
const sourceImageFiles = computed(() => sourceImageReferences.value.map((item) => item.file));
const maskImageFile = computed(() => maskImageReference.value?.file || null);
const maskImageSummary = computed(() => (maskImageFile.value ? maskImageFile.value.name : "蒙版（可选）"));
const operationOptions = computed(() => [
  { label: "文生图", value: "generation", disabled: sourceImageReferences.value.length > 0 },
  { label: "参考图", value: "edit" }
]);

const canSubmit = computed(() => {
  if (submitting.value) return false;
  if (ui.operation === "generation") {
    return Boolean(selectedGenerationModel.value && generationForm.prompt.trim());
  }
  if (sourceImageFiles.value.length === 0) return false;
  return Boolean(selectedEditModel.value && editForm.prompt.trim());
});

const promptPlaceholder = computed(() => {
  return ui.operation === "edit" ? "描述你希望修改成什么样" : "描述你希望生成的画面";
});

watch([selectedGenerationModel, selectedEditModel], () => clampActiveOutputCount());
watch(() => ui.operation, () => {
  clampActiveOutputCount();
});

function clampActiveOutputCount() {
  activeForm.value.n = Math.max(1, Math.min(activeOutputCountMax.value, activeForm.value.n || 1));
}

const queueCounts = computed(() => ({
  all: jobs.value.length,
  active: jobs.value.filter((job) => activeStatuses.has(job.status)).length,
  completed: jobs.value.filter((job) => job.status === "completed").length,
  failed: jobs.value.filter((job) => job.status === "failed").length
}));

const filterOptions = computed(() => [
  { label: `全部 ${queueCounts.value.all}`, value: "all" },
  { label: `进行中 ${queueCounts.value.active}`, value: "active" },
  { label: `完成 ${queueCounts.value.completed}`, value: "completed" },
  { label: `失败 ${queueCounts.value.failed}`, value: "failed" }
]);

// 结果流按时间正序：最新任务在底部、贴近输入框（聊天式）
const visibleJobs = computed(() => {
  const filtered = jobs.value.filter((job) => {
    if (activeFilter.value === "active") return activeStatuses.has(job.status);
    if (activeFilter.value === "completed") return job.status === "completed";
    if (activeFilter.value === "failed") return job.status === "failed";
    return true;
  });
  return [...filtered].sort((a, b) => (a.created_at || 0) - (b.created_at || 0));
});
const visibleJobCards = computed<PortalImageJobCard[]>(() => visibleJobs.value.map((job) => buildJobCard(job)));

const previewVisible = computed({
  get: () => Boolean(previewImage.value),
  set: (value: boolean) => {
    if (!value) previewImage.value = "";
  }
});

watch(
  () => ui.operation,
  (operation, previousOperation) => {
    if (operation === "edit" && previousOperation === "generation" && !editForm.prompt.trim()) {
      editForm.prompt = generationForm.prompt;
    }
  }
);
watch(
  visibleJobCards,
  (items) => {
    const keep = new Set(items.map((item) => item.job.id));
    const next = Object.fromEntries(
      Object.entries(expandedPromptState.value).filter(([jobId, expanded]) => keep.has(jobId) && expanded)
    );
    if (Object.keys(next).length !== Object.keys(expandedPromptState.value).length) {
      expandedPromptState.value = next;
    }
  },
  { immediate: true }
);

function modelOptionKey(model: PortalImageModelRecord) {
  return `${model.group_id || ""}::${model.model_code}`;
}

function modelGroupLabel(model: PortalImageModelRecord) {
  return model.billing_group_label || model.group_name || model.group_id || "默认分组";
}

function modelMultiplierLabel(model: PortalImageModelRecord) {
  return typeof model.effective_user_multiplier === "number" ? `${formatMultiplier(model.effective_user_multiplier)}x` : "";
}

function modelOptionMeta(model: PortalImageModelRecord) {
  const groupLabel = modelGroupLabel(model);
  const multiplierLabel = modelMultiplierLabel(model);
  return multiplierLabel && !groupLabel.includes(multiplierLabel) ? `${groupLabel} · ${multiplierLabel}` : groupLabel;
}

function modelOptionLabel(model: PortalImageModelRecord) {
  const suffix = modelOptionMeta(model);
  return suffix ? `${model.model_code} · ${suffix}` : model.model_code;
}

async function fetchModels() {
  models.value = await props.api.listModels();
  if (!generationForm.model && models.value.length > 0) generationForm.model = modelOptionKey(models.value[0]);
  if (!editForm.model && models.value.length > 0) editForm.model = modelOptionKey(models.value[0]);
}

async function fetchJobs() {
  jobs.value = await props.api.listJobs();
}

async function refreshAll() {
  loading.value = true;
  try {
    await Promise.all([fetchModels(), fetchJobs()]);
  } finally {
    loading.value = false;
  }
}

const { mergeJob, startPolling } = usePortalImageTaskQueue({
  api: props.api,
  jobs,
  fetchJobs,
  pollIntervalMs: () => props.pollIntervalMs
});

async function scrollToBottom() {
  await nextTick();
  const el = resultStreamRef.value;
  if (!el) return;
  el.scrollTop = el.scrollHeight;
  // 缩略图异步加载后高度会变，稍后再兜一次，确保停在最新任务
  window.setTimeout(() => {
    const target = resultStreamRef.value;
    if (target) target.scrollTop = target.scrollHeight;
  }, 150);
}

function handleAddReferences(files: File[]) {
  const result = addReferences(files);
  if (result.rejected > 0) props.notifyError?.("仅支持上传图片文件");
  if (result.added > 0) ui.operation = "edit";
}

function handleRemoveReference(id: string) {
  const removedIndex = removeReference(id);
  if (removedIndex === null) return;
  editForm.prompt = editForm.prompt.replace(/@图片(\d+)/g, (token, rawNumber: string) => {
    const number = Number(rawNumber);
    if (number === removedIndex + 1) return "";
    return number > removedIndex + 1 ? `@图片${number - 1}` : token;
  });
  if (sourceImageReferences.value.length === 0) clearMaskImage();
}

function handleMoveReference(payload: { id: string; direction: PortalImageReferenceMove }) {
  const moved = moveReference(payload.id, payload.direction);
  if (!moved) return;
  editForm.prompt = editForm.prompt.replace(/@图片(\d+)/g, (token, rawNumber: string) => {
    const number = Number(rawNumber);
    if (number === moved.from + 1) return `@图片${moved.to + 1}`;
    if (number === moved.to + 1) return `@图片${moved.from + 1}`;
    return token;
  });
}

function insertReferenceToken(index: number, input: InputInstance | null = composerInputRef.value) {
  const token = `@图片${index + 1}`;
  const textarea = input?.textarea;
  const prompt = editForm.prompt;
  const start = textarea?.selectionStart ?? prompt.length;
  const end = textarea?.selectionEnd ?? start;
  const before = prompt.slice(0, start);
  const after = prompt.slice(end);
  const prefix = before && !/\s$/.test(before) ? " " : "";
  const suffix = " ";
  const inserted = `${prefix}${token}${suffix}`;
  editForm.prompt = `${before}${inserted}${after}`;
  const cursor = before.length + inserted.length;
  void nextTick(() => {
    textarea?.focus();
    textarea?.setSelectionRange(cursor, cursor);
  });
}

function insertExpandedReferenceToken(index: number) {
  insertReferenceToken(index, expandedPromptInputRef.value);
}

function onMaskImageChange(event: Event) {
  const input = event.target as HTMLInputElement;
  setMask(input.files?.[0] || null);
  input.value = "";
}

function clearMaskImage() {
  setMask(null);
  maskImageInputKey.value += 1;
}

function handleComposerKeydown(event: KeyboardEvent) {
  if (event.isComposing || composerComposing.value || event.keyCode === 229) return;
  if (event.key === "Enter" && !event.shiftKey) {
    event.preventDefault();
    void submit();
  }
}

function resetComposerInputs() {
  if (ui.operation === "generation") {
    generationForm.prompt = "";
    return;
  }
  editForm.prompt = "";
  resetReferences();
  maskImageInputKey.value += 1;
}

async function submit() {
  if (!canSubmit.value) return;
  submitting.value = true;
  try {
    const created = await props.api.createTask(buildTaskPayload());
    resetComposerInputs();
    try {
      const task = await props.api.getTask(created.task_id);
      mergeJob(task);
    } catch {
      await fetchJobs();
    }
    props.notifySuccess?.(`任务已加入队列，${props.usageMessage}`);
    startPolling();
    void scrollToBottom();
  } catch (error) {
    props.notifyError?.((error as Error).message || "创建生图任务失败");
  } finally {
    submitting.value = false;
  }
}

async function submitFromExpandedEditor() {
  if (!canSubmit.value) return;
  promptEditorExpanded.value = false;
  await submit();
}

function buildTaskPayload() {
  if (ui.operation === "generation") {
    const model = selectedGenerationModel.value;
    return {
      operation: "generation" as const,
      model: model?.model_code || "",
      group_id: model?.group_id,
      prompt: generationForm.prompt.trim(),
      n: generationForm.n,
      size: resolveOpenAIImageSize(generationForm.resolution, generationForm.aspectRatio)
    };
  }
  const model = selectedEditModel.value;
  const form = new FormData();
  form.append("operation", "edit");
  for (const file of sourceImageFiles.value) {
    form.append("image[]", file);
  }
  if (maskImageFile.value) form.append("mask", maskImageFile.value);
  form.append("model", model?.model_code || "");
  if (model?.group_id) form.append("group_id", model.group_id);
  form.append("prompt", editForm.prompt.trim());
  form.append("n", String(editForm.n));
  form.append("size", resolveOpenAIImageSize(editForm.resolution, editForm.aspectRatio));
  return form;
}

async function retryJob(job: PortalImageJobRecord) {
  submitting.value = true;
  try {
    const created = await props.api.createTask({
      operation: job.operation || "generation",
      model: job.model_code,
      group_id: job.group_id,
      prompt: job.retry_prompt || job.prompt,
      n: job.requested_output_count || job.image_count || 1,
      size: job.size || undefined
    });
    mergeJob(await props.api.getTask(created.task_id));
    props.notifySuccess?.(`任务已加入队列，${props.usageMessage}`);
    startPolling();
    void scrollToBottom();
  } catch (error) {
    props.notifyError?.((error as Error).message || "重试任务失败");
  } finally {
    submitting.value = false;
  }
}

async function copyPrompt(job: PortalImageJobRecord) {
  const text = copyPromptText(job);
  if (!text) return;
  try {
    await navigator.clipboard.writeText(text);
    props.notifySuccess?.("提示词已复制");
  } catch {
    props.notifyError?.("复制失败，请手动选择文本复制");
  }
}

function copyPromptText(job: PortalImageJobRecord) {
  return (job.prompt || job.retry_prompt || "").trim();
}

function displayPromptText(job: PortalImageJobRecord) {
  return (job.prompt || "").trim();
}

function shouldShowPromptToggle(job: PortalImageJobRecord) {
  const text = displayPromptText(job);
  return text.length > 96 || /\r?\n/.test(text);
}

function isPromptExpanded(job: PortalImageJobRecord) {
  return Boolean(expandedPromptState.value[job.id]);
}

function togglePrompt(job: PortalImageJobRecord) {
  expandedPromptState.value = {
    ...expandedPromptState.value,
    [job.id]: !expandedPromptState.value[job.id]
  };
}

async function cancelJob(job: PortalImageJobRecord) {
  if (!activeStatuses.has(job.status) || cancellingJobId.value) return;
  cancellingJobId.value = job.id;
  try {
    mergeJob(await props.api.cancelTask(job.id));
    props.notifySuccess?.("任务已取消");
  } catch (error) {
    props.notifyError?.((error as Error).message || "取消任务失败");
  } finally {
    cancellingJobId.value = "";
  }
}

async function deleteJob(job: PortalImageJobRecord) {
  if (activeStatuses.has(job.status) || deletingJobId.value) return;
  const confirmed = props.confirmDelete
    ? await props.confirmDelete(job)
    : window.confirm("确定删除该生图任务记录及已保留的图片吗？此操作不可恢复。");
  if (!confirmed) return;

  deletingJobId.value = job.id;
  try {
    await props.api.deleteTask(job.id);
    jobs.value = jobs.value.filter((item) => item.id !== job.id);
    const nextExpandedPrompts = { ...expandedPromptState.value };
    delete nextExpandedPrompts[job.id];
    expandedPromptState.value = nextExpandedPrompts;
    props.notifySuccess?.("任务记录已删除");
  } catch (error) {
    props.notifyError?.((error as Error).message || "删除任务记录失败");
  } finally {
    deletingJobId.value = "";
  }
}

function statusLabel(status: string) {
  switch (status) {
    case "pending":
      return "排队中";
    case "running":
      return "生成中";
    case "completed":
      return "已完成";
    case "failed":
      return "失败";
    case "cancelled":
      return "已取消";
    default:
      return status || "-";
  }
}

function statusClass(status: string) {
  if (status === "completed") return "task-status done";
  if (status === "failed") return "task-status fail";
  if (status === "pending") return "task-status queue";
  return "task-status run";
}

function formatTimestamp(value?: number) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}

function elapsedText(job: PortalImageJobRecord) {
  const start = job.created_at || Date.now();
  const end = job.completed_at || Date.now();
  const seconds = Math.max(0, Math.round((end - start) / 1000));
  return `${seconds}s`;
}

function resolveJobImages(job: PortalImageJobRecord): PortalImagePreviewAsset[] {
  return (job.assets || [])
    .map((asset) => ({
      preview: resolveAssetUrl(asset.preview_url || asset.display_url || asset.original_url || ""),
      original: resolveAssetUrl(asset.original_url || asset.preview_url || asset.display_url || "")
    }))
    .filter((asset) => asset.preview);
}

function resolveAssetUrl(src: string) {
  const value = src.trim();
  if (!value) return "";
  if (/^(https?:|data:|blob:)/i.test(value)) return value;
  const base = (props.assetBaseUrl || "").trim().replace(/\/$/, "");
  if (!base) return value;
  if (value.startsWith("/")) {
    if (value === base || value.startsWith(`${base}/`)) return value;
    return `${base}${value}`;
  }
  return `${base}/${value}`;
}

function hasUpstreamImageContent(job: PortalImageJobRecord) {
  return job.image_count > 0 || job.inline_count > 0 || job.url_count > 0;
}

function buildJobCard(job: PortalImageJobRecord): PortalImageJobCard {
  const images = resolveJobImages(job);
  const isActive = activeStatuses.has(job.status);
  const isFailed = job.status === "failed";
  const promptText = displayPromptText(job);
  const canCopyPromptValue = Boolean(copyPromptText(job));
  const canDownloadValue = job.status === "completed" && images.length > 0;
  const canRetryValue = !isActive && job.status !== "completed" && job.operation !== "edit";
  const canCancelValue = isActive && (!cancellingJobId.value || cancellingJobId.value === job.id);
  const canDeleteValue = !isActive && (!deletingJobId.value || deletingJobId.value === job.id);
  const upstreamImageReturned = hasUpstreamImageContent(job);

  let visualLabel = "无图像";
  let visualHint = "";
  let visualTone: PortalImageJobCard["visualTone"] = "neutral";
  if (isFailed) {
    visualLabel = "生成失败";
    visualHint = job.error_message || "任务未完成";
    visualTone = "fail";
  } else if (job.status === "completed" && images.length === 0 && upstreamImageReturned) {
    visualLabel = job.url_count > 0 ? "已返回图片 URL" : "已返回图片内容";
    visualHint =
      job.url_count > 0
        ? "上游已返回可访问 URL，本次没有额外镜像到本地预览。"
        : "上游已返回 Base64 图片，本次没有额外保留原图文件。";
    visualTone = "success";
  }

  return {
    job,
    images,
    isActive,
    isFailed,
    isPromptExpanded: isPromptExpanded(job),
    promptText,
    showPromptToggle: shouldShowPromptToggle(job),
    canCopyPrompt: canCopyPromptValue,
    canDownload: canDownloadValue,
    canRetry: canRetryValue,
    canCancel: canCancelValue,
    canDelete: canDeleteValue,
    copyTooltip: canCopyPromptValue ? "复制提示词" : "当前任务没有可复制的提示词",
    downloadTooltip: canDownloadValue ? "下载原图" : "当前任务暂无可下载原图",
    retryTooltip: canRetryValue
      ? "重试"
      : job.operation === "edit"
        ? "参考图任务暂不支持重试"
        : isActive
          ? "任务进行中不可重试"
          : job.status === "completed"
            ? "任务已成功，无需重试"
            : "当前状态不可重试",
    cancelTooltip:
      cancellingJobId.value === job.id
        ? "正在取消任务"
        : canCancelValue
          ? "取消任务"
          : isActive && cancellingJobId.value
            ? "当前有其他任务正在取消"
            : "当前状态不可取消",
    deleteTooltip:
      deletingJobId.value === job.id
        ? "正在删除任务记录"
        : canDeleteValue
          ? "删除任务记录及已保留的图片"
          : "当前有其他任务正在删除",
    statusLabel: statusLabel(job.status),
    statusClass: statusClass(job.status),
    createdAtText: formatTimestamp(job.created_at),
    elapsedText: elapsedText(job),
    visualLabel,
    visualHint,
    visualTone
  };
}

function openPreview(src: string) {
  previewImage.value = src;
}

function downloadImage(src: string) {
  window.open(src, "_blank", "noopener,noreferrer");
}

function downloadAll(images: PortalImagePreviewAsset[]) {
  for (const image of images) {
    if (image.original) downloadImage(image.original);
  }
}

onMounted(async () => {
  await refreshAll();
  void scrollToBottom();
});
</script>

<template>
  <div class="image-page">
    <header class="studio-bar">
      <div class="studio-bar__title">
        <span class="studio-bar__eyebrow">在线体验 · AI GATEWAY</span>
        <h1>AI 生图</h1>
      </div>
      <div class="studio-bar__actions">
        <el-segmented v-model="activeFilter" size="small" :options="filterOptions" />
        <el-button class="studio-bar__refresh" :icon="refreshIcon" :loading="loading" text @click="refreshAll">刷新</el-button>
      </div>
    </header>

    <div ref="resultStreamRef" class="result-stream">
        <div class="queue-notice">
          <span class="queue-notice__icon">⏳</span>
          <span>生成的图片仅临时保留，过期后图片和任务记录都会被自动删除，请及时下载需要的原图。</span>
        </div>

        <div v-if="visibleJobCards.length" class="result-list">
          <article v-for="card in visibleJobCards" :key="card.job.id" class="task-card" :class="{ active: card.isActive }">
            <div class="task-visual">
              <div v-if="card.isActive" class="skeleton-grid">
                <span />
              </div>
              <div v-else-if="card.images.length" class="thumb-grid">
                <button v-for="image in card.images" :key="image.preview" class="thumb" type="button" @click="openPreview(image.preview)">
                  <img :src="image.preview" alt="" loading="lazy" decoding="async" />
                  <span class="thumb-action" @click.stop="downloadImage(image.original)">下载</span>
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
                        @click="copyPrompt(card.job)"
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
                        @click="downloadAll(card.images)"
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
                        @click="retryJob(card.job)"
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
                        :loading="cancellingJobId === card.job.id"
                        :disabled="!card.canCancel"
                        aria-label="取消任务"
                        @click="cancelJob(card.job)"
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
                        :loading="deletingJobId === card.job.id"
                        :disabled="!card.canDelete"
                        aria-label="删除任务记录"
                        @click="deleteJob(card.job)"
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
                  @click="togglePrompt(card.job)"
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
                <span v-if="card.job.status === 'completed'" class="cost-chip">
                  <el-icon><Coin /></el-icon>{{ formatUSD(card.job.caller_charge_usd) }}
                </span>
                <span class="task-time">{{ card.createdAtText }}</span>
              </div>

              <template v-if="card.isActive">
                <div class="progress indeterminate"><i /></div>
                <div class="progress-note">
                  <span>{{ card.job.status === "pending" ? "等待执行" : "模型推理中" }}</span>
                  <span>已用 {{ card.elapsedText }}</span>
                </div>
              </template>
              <div v-else-if="card.isFailed" class="fail-box">{{ card.job.error_message || "任务未完成" }}</div>
            </div>
          </article>
        </div>

        <div v-else class="stream-empty">
          <span class="stream-empty__icon">🎨</span>
          <h2>想生成什么？</h2>
          <p>在下方输入提示词，选择模型与参数，点击发送即可加入生成队列。</p>
        </div>
      </div>

      <footer class="composer">
        <div class="composer-box">
          <PortalImageReferenceTray
            class="composer-reference-tray"
            :references="sourceImageReferences"
            @add="handleAddReferences"
            @remove="handleRemoveReference"
            @move="handleMoveReference"
            @preview="openPreview"
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

    <el-dialog v-model="previewVisible" width="min(920px, 92vw)" append-to-body>
      <img v-if="previewImage" class="preview-image" :src="previewImage" alt="" />
    </el-dialog>

    <el-dialog v-model="promptEditorExpanded" title="编辑提示词" width="min(900px, calc(100vw - 32px))" append-to-body>
      <div class="expanded-editor">
        <PortalImageReferenceTray
          class="expanded-editor__references"
          :references="sourceImageReferences"
          @add="handleAddReferences"
          @remove="handleRemoveReference"
          @move="handleMoveReference"
          @preview="openPreview"
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
  </div>
</template>

<style scoped>
.image-page {
  --studio-content-width: 960px;
  /* 壳层 DsAppShell 的 canvas 用 min-height（非定高），满高页必须自身定高，
     否则内容超视口时 flex:1 无法约束、会退化成整页滚动。
     56 = 顶栏高度，76 = 内容区上下内边距，均取自 DsAppShell 布局常量。 */
  height: calc(100dvh - 56px - 76px);
  min-height: 420px;
  display: flex;
  flex-direction: column;
  color: var(--ds-ink);
  background: var(--ds-paper);
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-shell);
  overflow: hidden;
}

/* —— 顶部轻量标题条（常驻，不随结果流滚动） —— */
.studio-bar {
  flex-shrink: 0;
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  padding: 15px 22px 13px;
  border-bottom: 1px solid var(--ds-line);
  background: linear-gradient(180deg, var(--ds-panel), color-mix(in srgb, var(--ds-panel) 55%, var(--ds-paper)));
}

.studio-bar__title {
  min-width: 0;
}

.studio-bar__eyebrow {
  display: block;
  margin-bottom: 3px;
  color: var(--ds-faint);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
}

.studio-bar__title h1 {
  margin: 0;
  font-size: 20px;
  font-weight: 750;
  letter-spacing: 0;
  color: var(--ds-ink);
}

.studio-bar__actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

.studio-bar :deep(.el-segmented) {
  --el-segmented-item-selected-bg-color: var(--ds-accent);
  --el-segmented-item-selected-color: var(--ds-white);
  border-radius: var(--ds-radius-pill);
  padding: 3px;
}

.studio-bar__refresh {
  color: var(--ds-muted);
}

/* —— 结果画布：卡片浮于 paper 背景之上 —— */
.result-stream {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 20px 20px 12px;
  scroll-behavior: smooth;
}

.queue-notice {
  position: sticky;
  top: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 8px;
  box-sizing: border-box;
  width: min(100%, var(--studio-content-width));
  margin: 0 auto 16px;
  padding: 8px 14px;
  border-radius: var(--ds-radius-pill);
  border: 1px solid color-mix(in srgb, var(--ds-warning) 24%, var(--ds-line));
  background: color-mix(in srgb, var(--ds-warning) 10%, var(--ds-panel));
  color: color-mix(in srgb, var(--ds-warning) 42%, var(--ds-ink));
  font-size: 12px;
  line-height: 1.5;
  box-shadow: var(--ds-shadow-sm);
}

.queue-notice__icon {
  flex-shrink: 0;
  font-size: 13px;
}

.result-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  box-sizing: border-box;
  width: min(100%, var(--studio-content-width));
  margin: 0 auto;
}

.stream-empty {
  height: 100%;
  min-height: 260px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  color: var(--ds-muted);
  text-align: center;
}

.stream-empty__icon {
  font-size: 44px;
  opacity: 0.9;
}

.stream-empty h2 {
  margin: 10px 0 0;
  color: var(--ds-ink);
  font-size: 18px;
  font-weight: 750;
  letter-spacing: 0;
}

.stream-empty p {
  margin: 0;
  font-size: 13px;
}

/* —— 结果卡：画布上浮起的媒体卡 —— */
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
  box-shadow: 0 0 0 3px var(--ds-accent-soft), var(--ds-shadow-sm);
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
  border-radius: 50%;
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
  border-radius: 4px;
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
  border-radius: inherit;
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
  border-radius: 6px;
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

.action-buttons {
  display: flex;
  align-items: center;
  gap: 6px;
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
  box-shadow: inset 0 1px 0 color-mix(in srgb, var(--ds-white) 65%, transparent);
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

/* —— 底部 Composer —— */
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
  border-radius: 16px;
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-panel);
  padding: 12px 14px 10px;
  transition: border-color 0.18s ease, box-shadow 0.18s ease;
}

.composer-box:focus-within {
  border-color: color-mix(in srgb, var(--ds-accent) 55%, var(--ds-line));
  box-shadow: 0 0 0 4px var(--ds-accent-soft), var(--ds-shadow-panel);
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
  border-radius: 6px;
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

.preview-image {
  display: block;
  max-width: 100%;
  max-height: 78vh;
  margin: 0 auto;
  border-radius: 10px;
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

@media (max-width: 960px) {
  /* 移动端壳层侧栏堆叠、内边距变化，改回自然整页滚动 */
  .image-page {
    height: auto;
    min-height: 0;
  }
}

@media (max-width: 560px) {
  .studio-bar {
    align-items: stretch;
    flex-direction: column;
    padding: 12px;
  }

  .studio-bar__actions {
    min-width: 0;
    justify-content: space-between;
  }

  .studio-bar__actions :deep(.el-segmented) {
    min-width: 0;
  }

  .studio-bar__title h1 {
    white-space: nowrap;
  }

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

  .task-card {
    grid-template-columns: 1fr;
  }

  .task-visual {
    width: 160px;
  }
}
</style>
