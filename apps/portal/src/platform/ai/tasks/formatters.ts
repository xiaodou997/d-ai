import type {
  PortalTaskRecord,
  PortalTaskSource,
  PortalTaskStatus,
  PortalTaskType
} from "./types";

export function portalTaskStatusLabel(status: PortalTaskStatus): string {
  return {
    pending: "待执行",
    running: "执行中",
    completed: "已完成",
    failed: "失败",
    cancelled: "已取消"
  }[status];
}

export function portalTaskStatusTagType(status: PortalTaskStatus): "info" | "primary" | "success" | "danger" | "warning" {
  const types = {
    pending: "info",
    running: "primary",
    completed: "success",
    failed: "danger",
    cancelled: "warning"
  } as const;
  return types[status];
}

// DsTag 色调映射(DsUI 迁移后使用;portalTaskStatusTagType 保留给 element-plus 场景)
export function portalTaskStatusTone(status: PortalTaskStatus): "neutral" | "accent" | "positive" | "danger" | "warning" {
  const tones = {
    pending: "neutral",
    running: "accent",
    completed: "positive",
    failed: "danger",
    cancelled: "warning"
  } as const;
  return tones[status];
}

export function portalTaskTypeLabel(type: PortalTaskType): string {
  return {
    "images.generation": "文生图",
    "images.edit": "图生图",
    "chat.completions": "AI 对话"
  }[type];
}

export function portalTaskSourceLabel(source: PortalTaskSource): string {
  return {
    portal: "门户",
    api_key: "API Key"
  }[source];
}

export function formatPortalTaskTime(value?: number): string {
  return value ? new Date(value).toLocaleString() : "-";
}

export function formatPortalTaskDuration(task: PortalTaskRecord): string {
  const end = task.completed_at || (task.status === "running" ? Date.now() : task.started_at);
  if (!end) return "-";
  const start = task.started_at || task.created_at;
  const seconds = Math.max(0, Math.round((end - start) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const rest = seconds % 60;
  return rest ? `${minutes}m ${rest}s` : `${minutes}m`;
}

export function formatPortalTaskUSD(value?: number): string {
  if (value === undefined || value === null) return "-";
  return `$${value.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 6 })}`;
}

export function shortPortalTaskID(value: string): string {
  return value.length <= 16 ? value : `${value.slice(0, 8)}…${value.slice(-6)}`;
}
