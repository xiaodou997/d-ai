import type {
  TenantSubPurchasePolicyInput
} from "@/api/types/aiTenant";

export function defaultSubscriptionPurchasePolicy(): TenantSubPurchasePolicyInput {
  return {
    lifetime_max_purchases: null,
    period_type: "none",
    period_max_purchases: null,
    rolling_window_hours: null,
    calendar_unit: "",
    calendar_timezone: "",
    allow_advance_purchase: true
  };
}

export function subscriptionPurchasePolicyLabel(
  policy?: TenantSubPurchasePolicyInput | null
): string {
  if (!policy) return "不限购买频次";
  const parts: string[] = [];
  if (policy.lifetime_max_purchases != null) {
    parts.push(`累计最多 ${policy.lifetime_max_purchases} 次`);
  }
  if (policy.period_type === "rolling") {
    parts.push(
      `每 ${policy.rolling_window_hours} 小时最多 ${policy.period_max_purchases} 次`
    );
  } else if (policy.period_type === "calendar") {
    const units: Record<string, string> = {
      day: "日",
      week: "周",
      month: "月"
    };
    const unit = units[policy.calendar_unit || ""] || "周期";
    parts.push(`每自然${unit}最多 ${policy.period_max_purchases} 次`);
  }
  if (!policy.allow_advance_purchase) parts.push("不可提前购买");
  return parts.length ? parts.join("；") : "不限购买频次";
}
