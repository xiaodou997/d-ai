import type { TenantAiClientSurface } from "../../../types/aiTenant";
import { capabilityForSurface, clientSurfaceOptions } from "./catalog";

export type DispatchMatchType = "exact" | "prefix" | "wildcard" | "regex";

export const dispatchMatchOptions: Array<{ value: DispatchMatchType; label: string; description: string; tip: string; placeholder: string; examples: string[] }> = [
  { value: "exact", label: "完全等于", description: "只有用户请求的模型名完全一致时才命中。", tip: "适合单个模型精确改写。", placeholder: "例如 claude-opus-4-8", examples: ["claude-opus-4-8", "claude-sonnet-5"] },
  { value: "prefix", label: "开头匹配", description: "只要模型名以这段文本开头就命中。", tip: "适合同一系列模型，例如所有 claude- 或 gpt-5 开头的请求。", placeholder: "例如 claude- 或 gpt-5", examples: ["claude-", "gpt-5"] },
  { value: "wildcard", label: "通配符", description: "支持 * 和 ? 这类通配符写法。", tip: "适合模型名中间有变化的同类请求。", placeholder: "例如 claude-opus-* 或 gpt-5*", examples: ["claude-*", "claude-opus-*"] },
  { value: "regex", label: "正则表达式", description: "适合复杂规则，但需要懂正则。", tip: "普通场景优先使用前面三种方式。", placeholder: "例如 ^gpt-5(\\.|$)", examples: ["^claude-(opus|sonnet)-", "^gpt-5(\\.|$)"] }
];

export function surfacePresentation(value: TenantAiClientSurface) {
  const option = clientSurfaceOptions.find((item) => item.id === value);
  return option || clientSurfaceOptions[0];
}

export function matchPresentation(value: DispatchMatchType) {
  return dispatchMatchOptions.find((item) => item.value === value) || dispatchMatchOptions[0];
}

export function capabilityDescription(surface: TenantAiClientSurface) {
  const capability = capabilityForSurface(surface);
  return `该入口需要目标模型具备「${capability === "chat" ? "对话" : capability === "embedding" ? "向量" : "图片"}」能力价格。`;
}
