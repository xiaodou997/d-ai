import type {
  TenantAiGroupTarget,
  TenantAiUpstreamResource
} from "../../../types/aiTenant";

export type GroupTargetChange = "add" | "update" | "remove";
export type GroupTargetStatus = "active" | "disabled";
export type GroupTargetKind = "direct_upstream" | "oauth_pool";

export interface GroupTargetDraft {
  priority: number;
  status: GroupTargetStatus;
}

export interface GroupTargetRow extends GroupTargetDraft {
  key: string;
  targetId: string;
  kind: GroupTargetKind;
  name: string;
  protocols: string[];
  tenantMultiplier: number | null;
  availableModels: number;
  linked: boolean;
  selected: boolean;
  selectable: boolean;
  resourceState: "available" | "unpriced" | "missing";
  // 已绑定 target 的当前可用性（仅 linked 行有意义）：绑定后资源被停用/转 restricted/
  // 撤销授权，绑定仍在但请求会被网关 fail-closed 拒。available=false 时以 reason 说明。
  bindingUnavailableReason: "inactive" | "access_revoked" | "missing" | null;
  change: GroupTargetChange | null;
}

export interface GroupTargetSaveFailure {
  action: GroupTargetChange;
  targetKey: string;
  targetName: string;
  message: string;
}

export interface GroupTargetSaveResult {
  added: number;
  updated: number;
  removed: number;
  failures: GroupTargetSaveFailure[];
}

export function resourceKey(kind: GroupTargetKind, id: string) {
  return `${kind}:${id}`;
}

export function bindingKind(binding: TenantAiGroupTarget): GroupTargetKind {
  return binding.account_id ? "direct_upstream" : "oauth_pool";
}

export function bindingTargetId(binding: TenantAiGroupTarget) {
  return binding.account_id || binding.credential_pool_id || "";
}

export function bindingKey(binding: TenantAiGroupTarget) {
  return resourceKey(bindingKind(binding), bindingTargetId(binding));
}

export function resourceAvailableModelCount(resource: TenantAiUpstreamResource) {
  return resource.models.filter((model) => model.availability === "available").length;
}

// bindingUnavailableReason 返回该绑定当前对本租户不可用的原因，可用则 null。
// 后端 available=true 时不带 reason；缺字段（旧响应）视为可用，避免误报红。
export function bindingUnavailableReason(
  binding: TenantAiGroupTarget | undefined
): "inactive" | "access_revoked" | "missing" | null {
  if (!binding || binding.available !== false) return null;
  return binding.unavailable_reason ?? "missing";
}
