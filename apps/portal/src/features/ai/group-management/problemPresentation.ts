import { h } from "vue";
import { ElMessageBox } from "element-plus";
import { HttpProblem } from "@/platform";

import type {
  TenantAiDispatchPriceConflict,
  TenantAiGroupDependencyCounts
} from "@/api/types/aiTenant";
export { errorMessage } from "./errorMessage";

export function dispatchPriceConflicts(error: unknown): TenantAiDispatchPriceConflict[] {
  if (!(error instanceof HttpProblem) || error.code !== "dispatch_rule_price_conflict") return [];
  const conflicts = error.meta?.conflicts;
  return Array.isArray(conflicts) ? conflicts as TenantAiDispatchPriceConflict[] : [];
}

export async function showDispatchPriceConflict(error: unknown) {
  const conflicts = dispatchPriceConflicts(error);
  if (!conflicts.length) return false;
  await ElMessageBox.alert(
    h("div", { class: "price-conflict-scroll" }, [
      h("table", { class: "price-conflict-table" }, [
        h("thead", [h("tr", ["分组", "API 格式", "匹配值", "目标模型", "所需能力"].map((label) => h("th", label)))]),
        h("tbody", conflicts.map((item) => h("tr", [
          h("td", item.group_name || item.group_id || "当前分组"),
          h("td", item.api_format || "-"),
          h("td", item.match_value || "-"),
          h("td", item.target_model || "-"),
          h("td", item.required_capability || "-")
        ])))
      ])
    ]),
    "价格完整性冲突",
    { confirmButtonText: "知道了", customClass: "price-conflict-dialog" }
  );
  return true;
}

export function groupDependencyCounts(error: unknown): TenantAiGroupDependencyCounts | null {
  if (!(error instanceof HttpProblem) || error.code !== "group_in_use") return null;
  const value = error.meta?.dependencies as Partial<TenantAiGroupDependencyCounts> | undefined;
  if (!value || typeof value !== "object") return null;
  return {
    user_bindings: Number(value.user_bindings || 0),
    api_key_bindings: Number(value.api_key_bindings || 0),
    subscription_plans: Number(value.subscription_plans || 0)
  };
}

export async function showGroupDependencies(error: unknown) {
  const counts = groupDependencyCounts(error);
  if (!counts) return false;
  const rows = [
    ["用户分组绑定", counts.user_bindings],
    ["API 密钥绑定", counts.api_key_bindings],
    ["订阅套餐", counts.subscription_plans]
  ].filter(([, count]) => Number(count) > 0);
  await ElMessageBox.alert(
    h("div", rows.map(([label, count]) => h("div", { class: "dependency-row" }, [
      h("span", String(label)),
      h("strong", String(count))
    ]))),
    "分组仍在使用中",
    { confirmButtonText: "知道了" }
  );
  return true;
}
