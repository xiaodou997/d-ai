import type { DashboardSummaryDTO, SystemStatusDTO } from "@/api/types/ai";
import type { DailyTrendRowDTO } from "@/features/ai/usage/model";
import { formatDisplayUSD } from "@/shared/currency";

export function formatNumber(value: number | string | null | undefined) {
  return (Number(value) || 0).toLocaleString("zh-CN", { maximumFractionDigits: 2 });
}

export function formatUSD(value: number | string | null | undefined) {
  return formatDisplayUSD(value);
}

export function formatMs(value: number | string | null | undefined) {
  return `${Math.round(Number(value) || 0).toLocaleString("zh-CN")} ms`;
}

export function successRate(summary: DashboardSummaryDTO) {
  if (!summary.total_requests) return "0%";
  return `${((summary.successful_requests * 100) / summary.total_requests).toFixed(1)}%`;
}

export function trendLabels(rows: DailyTrendRowDTO[]) {
  return rows.map((row) => row.date.slice(5));
}

export function statusLabel(status?: string) {
  if (status === "ok" || status === "healthy" || status === "closed" || status === "active") return "正常";
  if (status === "disabled") return "已停用";
  if (status === "half_open") return "观察中";
  if (status === "warning") return "需要关注";
  if (status === "error" || status === "unhealthy" || status === "open") return "异常";
  return status || "未知";
}

export function statusTone(status?: string): "positive" | "warning" | "danger" | "neutral" {
  if (status === "ok" || status === "healthy" || status === "closed" || status === "active") return "positive";
  if (status === "half_open" || status === "warning") return "warning";
  if (status === "error" || status === "unhealthy" || status === "open") return "danger";
  return "neutral";
}

export function systemStatusLabel(system: SystemStatusDTO | null) {
  if (!system) return "未连接";
  if (system.db.status === "error" || system.redis.status === "error" || system.health.open_count > 0) return "需要关注";
  if (system.health.half_open_count > 0) return "观察中";
  return "运行正常";
}

export function systemStatusTone(system: SystemStatusDTO | null): "positive" | "warning" | "danger" | "neutral" {
  if (!system) return "neutral";
  if (system.db.status === "error" || system.redis.status === "error" || system.health.open_count > 0) return "danger";
  if (system.health.half_open_count > 0) return "warning";
  return "positive";
}
