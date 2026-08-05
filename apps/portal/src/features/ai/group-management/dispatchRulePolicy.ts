import type { TenantAiDispatchRule } from "../../../types/aiTenant";
import { capabilityForSurface } from "./catalog";

type PricedModel = { readonly model_code: string; readonly capability_type: string };
type DispatchModelAvailability = { readonly available_targets: number };

export function eligibleModels<T extends PricedModel>(entries: readonly T[], clientSurface: string) {
  const capability = capabilityForSurface(clientSurface);
  return entries
    .filter((entry) => entry.capability_type === capability)
    .filter((entry, index, list) => list.findIndex((item) => item.model_code === entry.model_code) === index)
    .sort((left, right) => left.model_code.localeCompare(right.model_code));
}

export function isRulePriced(rule: TenantAiDispatchRule, entries: readonly PricedModel[]) {
  const capability = capabilityForSurface(rule.client_surface);
  return entries.some((entry) => entry.model_code === rule.target_model_code && entry.capability_type === capability);
}

export function selectableDispatchModels<T extends DispatchModelAvailability>(models: readonly T[]) {
  return models.filter((model) => model.available_targets > 0);
}

export function validateMatchPattern(matchType: string, value: string) {
  if (!value.trim()) return "请填写匹配值";
  if (matchType !== "regex") return "";
  try {
    new RegExp(value.trim());
    return "";
  } catch {
    return "匹配值不是合法的正则表达式";
  }
}
