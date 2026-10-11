import type { PortalImageJobRecord } from "./types";
import { PORTAL_IMAGE_ACTIVE_STATUSES } from "./usePortalImageTaskQueue";

export interface ImageJobCardState {
  awaitingDetails?: boolean;
  assetBaseUrl?: string;
  cancellingJobId: string;
  deletingJobId: string;
  promptExpanded: boolean;
}

export interface PortalImagePreviewAsset {
  preview: string;
  original: string;
}

export interface PortalImageJobCard {
  awaitingDetails: boolean;
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

export function copyPromptText(job: PortalImageJobRecord) {
  return (job.prompt || job.retry_prompt || "").trim();
}

export function displayPromptText(job: PortalImageJobRecord) {
  return (job.prompt || "").trim();
}

export function shouldShowPromptToggle(job: PortalImageJobRecord) {
  const text = displayPromptText(job);
  return text.length > 96 || /\r?\n/.test(text);
}

export function statusLabel(status: string) {
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

export function statusClass(status: string) {
  if (status === "completed") return "task-status done";
  if (status === "failed") return "task-status fail";
  if (status === "pending") return "task-status queue";
  return "task-status run";
}

export function formatTimestamp(value?: number) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}

export function elapsedText(job: PortalImageJobRecord) {
  const start = job.created_at || Date.now();
  const end = job.completed_at || Date.now();
  const seconds = Math.max(0, Math.round((end - start) / 1000));
  return `${seconds}s`;
}

export function resolveJobImages(job: PortalImageJobRecord, assetBaseUrl?: string): PortalImagePreviewAsset[] {
  return (job.assets || [])
    .map((asset) => ({
      preview: resolveAssetUrl(asset.preview_url || asset.display_url || asset.original_url || "", assetBaseUrl),
      original: resolveAssetUrl(asset.original_url || asset.preview_url || asset.display_url || "", assetBaseUrl)
    }))
    .filter((asset) => asset.preview);
}

export function resolveAssetUrl(src: string, assetBaseUrl?: string) {
  const value = src.trim();
  if (!value) return "";
  if (/^(https?:|data:|blob:)/i.test(value)) return value;
  const base = (assetBaseUrl || "").trim().replace(/\/$/, "");
  if (!base) return value;
  if (value.startsWith("/")) {
    if (value === base || value.startsWith(`${base}/`)) return value;
    return `${base}${value}`;
  }
  return `${base}/${value}`;
}

export function hasUpstreamImageContent(job: PortalImageJobRecord) {
  return job.image_count > 0 || job.inline_count > 0 || job.url_count > 0;
}

export function buildImageJobCard(job: PortalImageJobRecord, state: ImageJobCardState): PortalImageJobCard {
  const images = resolveJobImages(job, state.assetBaseUrl);
  const isActive = PORTAL_IMAGE_ACTIVE_STATUSES.has(job.status);
  const isFailed = job.status === "failed";
  const promptText = displayPromptText(job);
  const canCopyPromptValue = Boolean(copyPromptText(job));
  const canDownloadValue = job.status === "completed" && images.length > 0;
  const canRetryValue = !state.awaitingDetails && !isActive && job.status !== "completed" && job.operation !== "edit";
  const canCancelValue = isActive && (!state.cancellingJobId || state.cancellingJobId === job.id);
  const canDeleteValue = !isActive && (!state.deletingJobId || state.deletingJobId === job.id);
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
    awaitingDetails: Boolean(state.awaitingDetails),
    images,
    isActive,
    isFailed,
    isPromptExpanded: state.promptExpanded,
    promptText,
    showPromptToggle: shouldShowPromptToggle(job),
    canCopyPrompt: canCopyPromptValue,
    canDownload: canDownloadValue,
    canRetry: canRetryValue,
    canCancel: canCancelValue,
    canDelete: canDeleteValue,
    copyTooltip: canCopyPromptValue ? "复制提示词" : "当前任务没有可复制的提示词",
    downloadTooltip: canDownloadValue ? "下载原图" : "当前任务暂无可下载原图",
    retryTooltip: state.awaitingDetails ? "等待任务状态同步" : canRetryValue
      ? "重试"
      : job.operation === "edit"
        ? "参考图任务暂不支持重试"
        : isActive
          ? "任务进行中不可重试"
          : job.status === "completed"
            ? "任务已成功，无需重试"
            : "当前状态不可重试",
    cancelTooltip:
      state.cancellingJobId === job.id
        ? "正在取消任务"
        : canCancelValue
          ? "取消任务"
          : isActive && state.cancellingJobId
            ? "当前有其他任务正在取消"
            : "当前状态不可取消",
    deleteTooltip:
      state.deletingJobId === job.id
        ? "正在删除任务记录"
        : canDeleteValue
          ? "删除任务记录及已保留的图片"
          : "当前有其他任务正在删除",
    statusLabel: state.awaitingDetails ? "已受理" : statusLabel(job.status),
    statusClass: statusClass(job.status),
    createdAtText: formatTimestamp(job.created_at),
    elapsedText: elapsedText(job),
    visualLabel,
    visualHint,
    visualTone
  };
}
