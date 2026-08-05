export type ReasoningEffort = "low" | "medium" | "high" | "xhigh" | "max";

export type UsageTagTone = "neutral" | "accent" | "positive" | "warning" | "danger" | "info";

export interface UsageTagMeta {
  label: string;
  tone: UsageTagTone;
}

/** UsageTag 支持的枚举维度。 */
export type UsageTagKind =
  | "status"
  | "source"
  | "billableUnit"
  | "stream"
  | "tokenUsageSource"
  | "conversion"
  | "effort";

export interface UsageCostSecondaryItem {
  label: string;
  value: string;
}
