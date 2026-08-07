import type { UsageTagKind, UsageTagMeta } from "./types";

export const requestStatusMeta: Record<string, UsageTagMeta> = {
  success: { label: "成功", tone: "positive" },
  failed: { label: "失败", tone: "danger" },
  error: { label: "错误", tone: "danger" },
  rejected: { label: "拒绝", tone: "warning" },
  pending: { label: "待处理", tone: "warning" }
};

export const requestSourceMeta: Record<string, UsageTagMeta> = {
  api_key: { label: "API", tone: "neutral" },
  web_chat: { label: "网页对话", tone: "positive" },
  web_image: { label: "网页生图", tone: "warning" }
};

export const requestSourceOptions = [
  { label: "API", value: "api_key" },
  { label: "网页对话", value: "web_chat" },
  { label: "网页生图", value: "web_image" }
];

export const billableUnitMeta: Record<string, UsageTagMeta> = {
  token: { label: "按量（Token）", tone: "info" },
  input_token: { label: "按输入（Token）", tone: "info" },
  output_token: { label: "按输出（Token）", tone: "info" },
  image: { label: "按次（图片）", tone: "accent" },
  second: { label: "按时长（秒）", tone: "accent" },
  request: { label: "按次（请求）", tone: "neutral" }
};

export const streamMeta: Record<string, UsageTagMeta> = {
  true: { label: "流式", tone: "accent" },
  false: { label: "非流", tone: "neutral" }
};

export const tokenUsageSourceMeta: Record<string, UsageTagMeta> = {
  upstream: { label: "上游", tone: "positive" },
  estimated: { label: "估算", tone: "warning" }
};

export const conversionMeta: Record<string, UsageTagMeta> = {
  true: { label: "转换", tone: "warning" },
  false: { label: "直通", tone: "positive" }
};

/** 推理强度分级配色：低→灰蓝渐进到 max→红。 */
export const reasoningEffortMeta: Record<string, UsageTagMeta> = {
  low: { label: "low", tone: "neutral" },
  medium: { label: "medium", tone: "info" },
  high: { label: "high", tone: "accent" },
  xhigh: { label: "xhigh", tone: "warning" },
  max: { label: "max", tone: "danger" }
};

const kindMetaTable: Record<UsageTagKind, Record<string, UsageTagMeta>> = {
  status: requestStatusMeta,
  source: requestSourceMeta,
  billableUnit: billableUnitMeta,
  stream: streamMeta,
  tokenUsageSource: tokenUsageSourceMeta,
  conversion: conversionMeta,
  effort: reasoningEffortMeta
};

/** 查某维度枚举值的展示元信息；未知值兜底原文 + neutral。 */
export function usageTagMeta(kind: UsageTagKind, value: string | boolean | null | undefined): UsageTagMeta | null {
  if (value === null || value === undefined || value === "") return null;
  const key = String(value);
  const meta = kindMetaTable[kind][key];
  if (meta) return meta;
  return { label: key, tone: "neutral" };
}

export function requestSourceLabel(value?: string | null): string {
  if (!value) return "-";
  return requestSourceMeta[value]?.label ?? value;
}
