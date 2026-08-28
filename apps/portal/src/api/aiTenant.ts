import {
  authenticatedRequest,
  apiHeaders,
  apiBaseUrl
} from "./request";
import { redirectPortalToLogin } from "@/platform";
import {
  appendPortalQuery,
  createPortalRuntimeTransport,
  portalStatusOptions
} from "@/platform/ai/runtime";
import { formatUSD } from "@/platform/ai/usage";
import { type PortalImageTaskCreateResponse } from "@/platform/ai/images";
import type { components } from "./generated/dai";
import type {
  PortalTaskPage,
  PortalTaskQuery,
  PortalTaskRecord
} from "@/platform/ai/tasks";
import { portalEnv } from "@/env";
import { useAuthStore } from "@/stores/auth";
import {
  createTypedOperationRequest,
  type OperationBody,
  type OperationResponse
} from ".";
import type {
  ChatModel,
  ConsoleImageGenerateRequest,
  ConsoleImageJob,
  ConsoleImageModel,
  ChatSession,
  ChatSessionDetail,
  TenantAiApiKey,
  TenantAiApiKeyCreatedOutputBody,
  TenantAiApiKeyRevealOutputBody,
  TenantAiApiKeysOutputBody,
  TenantAiApiKeyWriteRequest,
  TenantAiAvailableModelsOutputBody,
  TenantAiDashboardRecentErrorsOutputBody,
  TenantAiDashboardSummary,
  TenantAiLimitPolicy,
  TenantAiLimitPoliciesOutputBody,
  TenantAiLimitPolicyWriteRequest,
  TenantAiDashboardTopModelsOutputBody,
  TenantAiDeleteOutputBody,
  TenantAiClientSurfacePolicy,
  TenantAiClientSurfacePolicyWrite,
  TenantAiDispatchPreview,
  TenantAiDispatchPreviewRequest,
  TenantAiDispatchModel,
  TenantAiDispatchRule,
  TenantAiDispatchRuleWriteRequest,
  TenantAiGroupEffectivePricesOutputBody,
  TenantAiGroupTarget,
  TenantAiGroupTargetWriteRequest,
  TenantAiGroupWriteRequest,
  TenantAiPriceBook,
  TenantAiPriceBookEntry,
  TenantAiPriceBookEntryWriteRequest,
  TenantAiLiteLLMPriceModel,
  TenantAiLiteLLMModelsOutput,
  TenantAiPriceBookTransferBundle,
  TenantAiUpstreamResource,
  TenantAiUserGroupsOutputBody,
  TenantAiUserGroupWriteRequest,
  TenantAiVisibleGroup,
  TenantAiVisibleGroupsOutputBody,
  TenantSubPlan,
  TenantSubPurchasePolicyRevision,
  TenantSubscription,
  TenantSubOrder,
  TenantSubPage,
  TenantSubPlanWriteRequest
} from "./types/aiTenant";

function request() {
  return authenticatedRequest();
}

const typedRequest = createTypedOperationRequest(authenticatedRequest());

const headers = () => apiHeaders;
const baseUrl = () => apiBaseUrl;
const runtimeBasePath = "/runtime/v1";

type ApiKeyPageTransport = OperationResponse<"ai-list-tenant-self-api-keys">;
type GroupPageTransport = OperationResponse<"ai-list-groups">;
type DispatchRulePageTransport = OperationResponse<"ai-list-group-dispatch-rules">;
type DispatchModelPageTransport = OperationResponse<"ai-list-group-dispatch-models">;
type GroupTargetPageTransport = OperationResponse<"ai-list-group-targets">;

function stripSchema<T>(value: T): Omit<T, "$schema"> {
  const { $schema: _schema, ...rest } = value as T & { $schema?: string };
  return rest as Omit<T, "$schema">;
}

function toApiKeyWrite(body: TenantAiApiKeyWriteRequest): OperationBody<"ai-create-tenant-self-api-key"> {
  return {
    name: body.name,
    group_id: body.group_id,
    quota_limit_micro_usd: body.quota_limit_micro_usd,
    status: body.status === "active" || body.status === "disabled" ? body.status : undefined,
    expires_at: body.expires_at,
    limit_policy: body.limit_policy ?? undefined
  };
}

function toApiKeyStatus(value: string): OperationBody<"ai-update-tenant-self-api-key-status">["status"] {
  if (value === "active" || value === "disabled") return value;
  throw new Error(`Unexpected API key status: ${value}`);
}

function toGroupTarget(value: OperationResponse<"ai-add-group-target">): TenantAiGroupTarget {
  if (value.target_type !== "account" && value.target_type !== "pool") {
    throw new Error(`Unexpected group target type: ${value.target_type}`);
  }
  if (value.status !== "active" && value.status !== "disabled") {
    throw new Error(`Unexpected group target status: ${value.status}`);
  }
  const unavailableReason = value.unavailable_reason;
  if (unavailableReason !== undefined && unavailableReason !== "inactive" && unavailableReason !== "access_revoked" && unavailableReason !== "missing") {
    throw new Error(`Unexpected group target availability reason: ${unavailableReason}`);
  }
  return { ...stripSchema(value), target_type: value.target_type, status: value.status, unavailable_reason: unavailableReason };
}

function toApiKeyPage(value: ApiKeyPageTransport): TenantAiApiKeysOutputBody {
  return { items: value.items?.map((item) => stripSchema(item)) ?? [], total: value.total };
}

function toApiKey(value: OperationResponse<"ai-update-tenant-self-api-key">): TenantAiApiKey {
  return stripSchema(value);
}

function toCreatedApiKey(value: OperationResponse<"ai-create-tenant-self-api-key">): TenantAiApiKeyCreatedOutputBody {
  return { plaintext_key: value.plaintext_key, key: stripSchema(value.key) };
}

function toGroupPage(value: GroupPageTransport): TenantAiVisibleGroupsOutputBody {
  return { items: value.items?.map((item) => stripSchema(item)) ?? [], total: value.total };
}

function toDispatchRulePage(value: DispatchRulePageTransport): { items: TenantAiDispatchRule[]; total: number } {
  return { items: value.items?.map((item) => stripSchema(item)) ?? [], total: value.total };
}

function toDispatchModelPage(value: DispatchModelPageTransport): { items: TenantAiDispatchModel[]; total: number } {
  return { items: value.items?.map((item) => stripSchema(item)) ?? [], total: value.total };
}

function toGroupTargetPage(value: GroupTargetPageTransport): { items: TenantAiGroupTarget[]; total: number } {
  return { items: value.items?.map(toGroupTarget) ?? [], total: value.total };
}

export { formatUSD };

const runtimeTransport = createPortalRuntimeTransport({
  baseUrl,
  getAccessToken: () => useAuthStore().accessToken,
  async onUnauthorized() {
    const authStore = useAuthStore();
    try {
      await authStore.refreshAccessToken();
      return "retry";
    } catch {
      authStore.clear();
      return (await redirectPortalToLogin(portalEnv)) ? "handled" : false;
    }
  },
  runtimeBasePath
});

// ==================== 常量选项 ====================

export const statusOptions = portalStatusOptions;

export const capabilityOptions = [
  { label: "对话", value: "chat" },
  { label: "图像", value: "image" },
  { label: "视频", value: "video" },
  { label: "向量", value: "embedding" },
  { label: "语音合成", value: "audio_tts" },
  { label: "语音识别", value: "audio_stt" },
  { label: "重排序", value: "rerank" }
];

// ==================== AI 扁平端点 ====================

export const aiTenantApi = {
  // ---- 可用模型（可见分组暴露的去重模型集合） ----
  listAvailableModels() {
    return request()<TenantAiAvailableModelsOutputBody>({
      method: "GET",
      path: "/api/v1/tenants/me/available-models",
      headers: headers(),
      baseUrl: baseUrl()
    });
  },

  // ---- 租户 API Key ----
  listApiKeys() {
    return typedRequest<"ai-list-tenant-self-api-keys">({
      method: "GET",
      path: "/api/v1/tenant-api-keys",
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toApiKeyPage);
  },
  createApiKey(body: TenantAiApiKeyWriteRequest) {
    return typedRequest<"ai-create-tenant-self-api-key">({
      method: "POST",
      path: "/api/v1/tenants/me/api-keys",
      headers: headers(),
      body: toApiKeyWrite(body),
      baseUrl: baseUrl()
    }).then(toCreatedApiKey);
  },
  updateApiKey(apiKeyId: string, body: TenantAiApiKeyWriteRequest) {
    return typedRequest<"ai-update-tenant-self-api-key">({
      method: "PATCH",
      path: `/api/v1/tenants/me/api-keys/${encodeURIComponent(apiKeyId)}`,
      pathParams: { apiKeyID: apiKeyId },
      headers: headers(),
      body: toApiKeyWrite(body),
      baseUrl: baseUrl()
    }).then(toApiKey);
  },
  updateApiKeyStatus(apiKeyId: string, status: string) {
    return typedRequest<"ai-update-tenant-self-api-key-status">({
      method: "PATCH",
      path: `/api/v1/tenants/me/api-keys/${encodeURIComponent(apiKeyId)}/status`,
      pathParams: { apiKeyID: apiKeyId },
      headers: headers(),
      body: { status: toApiKeyStatus(status) },
      baseUrl: baseUrl()
    }).then(toApiKey);
  },
  revealApiKey(apiKeyId: string) {
    return typedRequest<"ai-reveal-tenant-self-api-key">({
      method: "POST",
      path: `/api/v1/tenants/me/api-keys/${encodeURIComponent(apiKeyId)}/reveal`,
      pathParams: { apiKeyID: apiKeyId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then((value) => ({ plaintext_key: value.plaintext_key }));
  },
  rotateApiKey(apiKeyId: string) {
    return typedRequest<"ai-rotate-tenant-self-api-key">({
      method: "POST",
      path: `/api/v1/tenants/me/api-keys/${encodeURIComponent(apiKeyId)}/rotate`,
      pathParams: { apiKeyID: apiKeyId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toCreatedApiKey);
  },
  deleteApiKey(apiKeyId: string) {
    return typedRequest<"ai-delete-tenant-self-api-key">({
      method: "DELETE",
      path: `/api/v1/tenants/me/api-keys/${encodeURIComponent(apiKeyId)}`,
      pathParams: { apiKeyID: apiKeyId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then((value) => ({ deleted: value.deleted }));
  },

  // ---- 租户自有分组 ----
  listMyGroups() {
    return typedRequest<"ai-list-groups">({
      method: "GET",
      path: "/api/v1/tenants/me/groups",
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toGroupPage);
  },
  getGroup(groupId: string) {
    return typedRequest<"ai-get-group">({
      method: "GET",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}`,
      pathParams: { groupID: groupId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then(stripSchema);
  },
  createGroup(body: OperationBody<"ai-create-group">) {
    return typedRequest<"ai-create-group">({
      method: "POST",
      path: "/api/v1/tenants/me/groups",
      headers: headers(),
      body,
      baseUrl: baseUrl()
    }).then(stripSchema);
  },
  updateGroup(groupId: string, body: OperationBody<"ai-update-group">) {
    return typedRequest<"ai-update-group">({
      method: "PATCH",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}`,
      pathParams: { groupID: groupId },
      headers: headers(),
      body,
      baseUrl: baseUrl()
    }).then(stripSchema);
  },
  updateGroupStatus(groupId: string, status: OperationBody<"ai-update-group-status">["status"]) {
    return typedRequest<"ai-update-group-status">({
      method: "PATCH",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/status`,
      pathParams: { groupID: groupId },
      headers: headers(),
      body: { status },
      baseUrl: baseUrl()
    }).then(stripSchema);
  },
  deleteGroup(groupId: string) {
    return typedRequest<"ai-delete-group">({
      method: "DELETE",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}`,
      pathParams: { groupID: groupId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then((value) => ({ deleted: value.deleted }));
  },
  getGroupClientSurfacePolicy(groupId: string) {
    return typedRequest<"ai-get-group-client-surface-policy">({
      method: "GET",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/client-surface-policy`,
      pathParams: { groupID: groupId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then(stripSchema);
  },
  replaceGroupClientSurfacePolicy(groupId: string, body: OperationBody<"ai-replace-group-client-surface-policy">) {
    return typedRequest<"ai-replace-group-client-surface-policy">({
      method: "PUT",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/client-surface-policy`,
      pathParams: { groupID: groupId },
      headers: headers(),
      body,
      baseUrl: baseUrl()
    }).then(stripSchema);
  },
  listGroupDispatchRules(groupId: string) {
    return typedRequest<"ai-list-group-dispatch-rules">({
      method: "GET",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/dispatch-rules`,
      pathParams: { groupID: groupId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toDispatchRulePage);
  },
  createGroupDispatchRule(groupId: string, body: OperationBody<"ai-add-group-dispatch-rule">) {
    return typedRequest<"ai-add-group-dispatch-rule">({
      method: "POST",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/dispatch-rules`,
      pathParams: { groupID: groupId },
      headers: headers(),
      body,
      baseUrl: baseUrl()
    }).then(stripSchema);
  },
  updateGroupDispatchRule(groupId: string, ruleId: string, body: OperationBody<"ai-update-group-dispatch-rule">) {
    return typedRequest<"ai-update-group-dispatch-rule">({
      method: "PATCH",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/dispatch-rules/${encodeURIComponent(ruleId)}`,
      pathParams: { groupID: groupId, ruleID: ruleId },
      headers: headers(),
      body,
      baseUrl: baseUrl()
    }).then(stripSchema);
  },
  updateGroupDispatchRuleStatus(groupId: string, ruleId: string, status: OperationBody<"ai-update-group-dispatch-rule-status">["status"]) {
    return typedRequest<"ai-update-group-dispatch-rule-status">({
      method: "PATCH",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/dispatch-rules/${encodeURIComponent(ruleId)}/status`,
      pathParams: { groupID: groupId, ruleID: ruleId },
      headers: headers(),
      body: { status },
      baseUrl: baseUrl()
    }).then(stripSchema);
  },
  deleteGroupDispatchRule(groupId: string, ruleId: string) {
    return typedRequest<"ai-delete-group-dispatch-rule">({
      method: "DELETE",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/dispatch-rules/${encodeURIComponent(ruleId)}`,
      pathParams: { groupID: groupId, ruleID: ruleId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then((value) => ({ deleted: value.deleted }));
  },
  previewGroupDispatch(groupId: string, body: OperationBody<"ai-preview-group-dispatch">) {
    return typedRequest<"ai-preview-group-dispatch">({
      method: "POST",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/dispatch-rules/preview`,
      pathParams: { groupID: groupId },
      headers: headers(),
      body,
      baseUrl: baseUrl()
    }).then(stripSchema);
  },
  listGroupDispatchModels(groupId: string, clientSurface: string) {
    return typedRequest<"ai-list-group-dispatch-models">({
      method: "GET",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/dispatch-models`,
      pathParams: { groupID: groupId },
      query: { client_surface: clientSurface },
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toDispatchModelPage);
  },
  listGroupTargets(groupId: string) {
    return typedRequest<"ai-list-group-targets">({
      method: "GET",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/targets`,
      pathParams: { groupID: groupId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toGroupTargetPage);
  },
  addGroupTarget(groupId: string, body: OperationBody<"ai-add-group-target">) {
    return typedRequest<"ai-add-group-target">({
      method: "POST",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/targets`,
      pathParams: { groupID: groupId },
      headers: headers(),
      body,
      baseUrl: baseUrl()
    }).then(toGroupTarget);
  },
  updateGroupTarget(groupId: string, bindingId: string, body: OperationBody<"ai-update-group-target">) {
    return typedRequest<"ai-update-group-target">({
      method: "PATCH",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/targets/${encodeURIComponent(bindingId)}`,
      pathParams: { groupID: groupId, bindingID: bindingId },
      headers: headers(),
      body,
      baseUrl: baseUrl()
    }).then((value) => toGroupTarget(value));
  },
  deleteGroupTarget(groupId: string, bindingId: string) {
    return typedRequest<"ai-delete-group-target">({
      method: "DELETE",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/targets/${encodeURIComponent(bindingId)}`,
      pathParams: { groupID: groupId, bindingID: bindingId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then((value) => ({ deleted: value.deleted }));
  },

  // ---- 租户价格表（平台表只读，租户表可写） ----
  listPriceBooks() {
    return request()<{ items: TenantAiPriceBook[]; total: number }>({
      method: "GET", path: "/api/v1/tenants/me/price-books", headers: headers(), baseUrl: baseUrl()
    });
  },
  createPriceBook(body: { name: string; description?: string }) {
    return request()<TenantAiPriceBook>({
      method: "POST", path: "/api/v1/tenants/me/price-books", headers: headers(), body, baseUrl: baseUrl()
    });
  },
  updatePriceBook(bookId: string, body: { name: string; description?: string; status?: "active" | "disabled" }) {
    return request()<TenantAiPriceBook>({
      method: "PATCH", path: `/api/v1/tenants/me/price-books/${encodeURIComponent(bookId)}`, headers: headers(), body, baseUrl: baseUrl()
    });
  },
  deletePriceBook(bookId: string) {
    return request()<TenantAiDeleteOutputBody>({
      method: "DELETE", path: `/api/v1/tenants/me/price-books/${encodeURIComponent(bookId)}`, headers: headers(), baseUrl: baseUrl()
    });
  },
  copyPriceBook(bookId: string, name?: string) {
    return request()<TenantAiPriceBook>({
      method: "POST", path: `/api/v1/tenants/me/price-books/${encodeURIComponent(bookId)}/clone`, headers: headers(), body: { name }, baseUrl: baseUrl()
    });
  },
  searchLiteLLMPriceModels(q: string, limit = 50) {
    return typedRequest<"ai-search-tenant-litellm-price-models">({
      method: "GET", path: "/api/v1/tenants/me/price-books/litellm/models", query: { q, limit }, headers: headers(), baseUrl: baseUrl()
    }).then((response: components["schemas"]["LiteLLMModelsOutputBody"]): TenantAiLiteLLMModelsOutput => ({
      ...response,
      items: response.items?.map((model) => ({ ...model, token_price_tiers: model.token_price_tiers ?? [] })) ?? []
    }));
  },
  listPriceBookEntries(bookId: string) {
    return request()<{ items: TenantAiPriceBookEntry[]; total: number }>({
      method: "GET", path: `/api/v1/tenants/me/price-books/${encodeURIComponent(bookId)}/entries`, headers: headers(), baseUrl: baseUrl()
    });
  },
  upsertPriceBookEntry(bookId: string, modelCode: string, body: TenantAiPriceBookEntryWriteRequest) {
    return request()<TenantAiPriceBookEntry>({
      method: "PUT", path: `/api/v1/tenants/me/price-books/${encodeURIComponent(bookId)}/entries/${encodeURIComponent(modelCode)}`, headers: headers(), body, baseUrl: baseUrl()
    });
  },
  deletePriceBookEntry(bookId: string, modelCode: string, capabilityType: string) {
    return request()<TenantAiDeleteOutputBody>({
      method: "DELETE", path: `/api/v1/tenants/me/price-books/${encodeURIComponent(bookId)}/entries/${encodeURIComponent(modelCode)}`,
      query: { capability_type: capabilityType }, headers: headers(), baseUrl: baseUrl()
    });
  },
  syncCommonPriceModels(bookId: string) {
    return request()<{ synced: number; missing: string[] }>({
      method: "POST", path: `/api/v1/tenants/me/price-books/${encodeURIComponent(bookId)}/sync-common`, headers: headers(), baseUrl: baseUrl()
    });
  },
  importPriceBook(body: TenantAiPriceBookTransferBundle) {
    return request()<TenantAiPriceBook>({
      method: "POST", path: "/api/v1/tenants/me/price-books/import", headers: headers(), body, baseUrl: baseUrl()
    });
  },
  exportPriceBook(bookId: string) {
    return request()<TenantAiPriceBookTransferBundle>({
      method: "GET", path: `/api/v1/tenants/me/price-books/${encodeURIComponent(bookId)}/export`, headers: headers(), baseUrl: baseUrl()
    });
  },

  // ---- 不含地址、密钥和内部模型名的上游目录 ----
  listUpstreamResources() {
    return request()<{ items: TenantAiUpstreamResource[]; total: number }>({
      method: "GET", path: "/api/v1/tenants/me/upstream-resources", headers: headers(), baseUrl: baseUrl()
    });
  },
  // ---- 某可见分组对本租户的每模型生效 USD 单价 ----
  getMyGroupEffectivePrices(groupId: string) {
    return request()<TenantAiGroupEffectivePricesOutputBody>({
      method: "GET",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/effective-prices`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  // ---- 租户→用户 分组绑定（套餐收窄 + 加价倍率） ----
  listUserGroups(userId: string) {
    return request()<TenantAiUserGroupsOutputBody>({
      method: "GET",
      path: `/api/v1/tenants/me/users/${encodeURIComponent(userId)}/groups`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  upsertUserGroup(userId: string, groupId: string, body: TenantAiUserGroupWriteRequest) {
    return request()<TenantAiDeleteOutputBody>({
      method: "PUT",
      path: `/api/v1/tenants/me/users/${encodeURIComponent(userId)}/groups/${encodeURIComponent(groupId)}`,
      headers: headers(),
      body,
      baseUrl: baseUrl()
    });
  },
  deleteUserGroup(userId: string, groupId: string) {
    return request()<TenantAiDeleteOutputBody>({
      method: "DELETE",
      path: `/api/v1/tenants/me/users/${encodeURIComponent(userId)}/groups/${encodeURIComponent(groupId)}`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },

  // ---- 租户限流策略 ----
  listUserLimitPolicies(userId: string) {
    return request()<TenantAiLimitPoliciesOutputBody>({
      method: "GET",
      path: `/api/v1/tenants/me/users/${encodeURIComponent(userId)}/limit-policies`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  upsertUserLimitPolicy(userId: string, body: TenantAiLimitPolicyWriteRequest) {
    return request()<TenantAiLimitPolicy>({
      method: "PUT",
      path: `/api/v1/tenants/me/users/${encodeURIComponent(userId)}/limit-policies`,
      headers: headers(),
      body,
      baseUrl: baseUrl()
    });
  },

  // ---- Dashboard（tenant 维度，claims-scoped） ----
  getDashboardSummary(params: { date_from?: string; date_to?: string } = {}) {
    return request()<TenantAiDashboardSummary>({
      method: "GET",
      path: "/api/v1/tenants/me/dashboard/summary",
      headers: headers(),
      query: params,
      baseUrl: baseUrl()
    });
  },
  getDashboardTopModels(params: { date_from?: string; date_to?: string; limit?: number } = {}) {
    return request()<TenantAiDashboardTopModelsOutputBody>({
      method: "GET",
      path: "/api/v1/tenants/me/dashboard/top-models",
      headers: headers(),
      query: params,
      baseUrl: baseUrl()
    });
  },
  listDashboardRecentErrors(params: { date_from?: string; date_to?: string; limit?: number } = {}) {
    return request()<TenantAiDashboardRecentErrorsOutputBody>({
      method: "GET",
      path: "/api/v1/tenants/me/dashboard/recent-errors",
      headers: headers(),
      query: params,
      baseUrl: baseUrl()
    });
  },

  // ---- 订阅制套餐（租户自助管理，docs/ai-subscription-design.md §7.2） ----
  listSubscriptionPlans(params: { status?: string; limit?: number; offset?: number } = {}) {
    return request()<TenantSubPage<TenantSubPlan>>({
      method: "GET",
      path: "/api/v1/tenants/me/subscription-plans",
      headers: headers(),
      query: params,
      baseUrl: baseUrl()
    });
  },
  createSubscriptionPlan(body: TenantSubPlanWriteRequest) {
    return request()<TenantSubPlan>({
      method: "POST",
      path: "/api/v1/tenants/me/subscription-plans",
      body,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  updateSubscriptionPlan(planId: string, body: TenantSubPlanWriteRequest) {
    return request()<TenantSubPlan>({
      method: "PUT",
      path: `/api/v1/tenants/me/subscription-plans/${encodeURIComponent(planId)}`,
      body,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  reorderSubscriptionPlans(planIds: string[]) {
    return request()<Record<string, never>>({
      method: "PUT",
      path: "/api/v1/tenants/me/subscription-plans/reorder",
      body: { plan_ids: planIds },
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  listSubscriptionPlanPurchasePolicyRevisions(planId: string) {
    return request()<{ items: TenantSubPurchasePolicyRevision[] }>({
      method: "GET",
      path: `/api/v1/tenants/me/subscription-plans/${encodeURIComponent(planId)}/purchase-policy-revisions`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  setSubscriptionPlanStatus(planId: string, status: "on_sale" | "off_sale") {
    return request()<TenantSubPlan>({
      method: "PUT",
      path: `/api/v1/tenants/me/subscription-plans/${encodeURIComponent(planId)}/status`,
      body: { status },
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  listSubscriptions(params: { user_id?: string; status?: string; limit?: number; offset?: number } = {}) {
    return request()<TenantSubPage<TenantSubscription>>({
      method: "GET",
      path: "/api/v1/tenants/me/subscriptions",
      headers: headers(),
      query: params,
      baseUrl: baseUrl()
    });
  },
  listSubscriptionOrders(params: { user_id?: string; status?: string; limit?: number; offset?: number } = {}) {
    return request()<TenantSubPage<TenantSubOrder>>({
      method: "GET",
      path: "/api/v1/tenants/me/subscription-orders",
      headers: headers(),
      query: params,
      baseUrl: baseUrl()
    });
  },
  listWorkspaceChatSessions(params: { limit?: number } = {}) {
    return request()<{ items: ChatSession[]; total: number }>({
      method: "GET",
      path: "/api/v1/tenants/me/workspace/chat/sessions",
      headers: headers(),
      query: { limit: 50, ...params },
      baseUrl: baseUrl()
    });
  },
  listWorkspaceChatModels() {
    return request()<{ items: ChatModel[]; total: number }>({
      method: "GET",
      path: "/api/v1/tenants/me/workspace/chat/models",
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  createWorkspaceChatSession(body: {
    model_code: string;
    group_id: string;
    title?: string;
  }) {
    return request()<ChatSession>({
      method: "POST",
      path: "/api/v1/tenants/me/workspace/chat/sessions",
      headers: headers(),
      body,
      baseUrl: baseUrl()
    });
  },
  getWorkspaceChatSession(sessionId: string) {
    return request()<ChatSessionDetail>({
      method: "GET",
      path: `/api/v1/tenants/me/workspace/chat/sessions/${encodeURIComponent(sessionId)}`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  deleteWorkspaceChatSession(sessionId: string) {
    return request()<{ deleted: boolean }>({
      method: "DELETE",
      path: `/api/v1/tenants/me/workspace/chat/sessions/${encodeURIComponent(sessionId)}`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  listWorkspaceImageJobs(params: { limit?: number } = {}) {
    return request()<{ items: ConsoleImageJob[]; total: number }>({
      method: "GET",
      path: "/api/v1/tenants/me/workspace/image/jobs",
      headers: headers(),
      query: { limit: 50, ...params },
      baseUrl: baseUrl()
    });
  }
};

// ==================== 网页工作台执行端点（信封 + SSE） ====================

export const runtimeChatApi = {
  listModels() {
    return aiTenantApi.listWorkspaceChatModels().then((res) => res.items ?? []);
  },
  listSessions() {
    return aiTenantApi.listWorkspaceChatSessions().then((res) => res.items ?? []);
  },
  createSession(body: {
    model_code: string;
    group_id: string;
    title?: string;
  }) {
    return aiTenantApi.createWorkspaceChatSession(body);
  },
  getSession(sessionId: string) {
    return aiTenantApi.getWorkspaceChatSession(sessionId);
  },
  deleteSession(sessionId: string) {
    return aiTenantApi.deleteWorkspaceChatSession(sessionId);
  }
};

export const runtimeImageApi = {
  async listModels() {
    return runtimeTransport.request<ConsoleImageModel[]>("GET", `${runtimeBasePath}/images/models`);
  },
  async listJobs() {
    return (await aiTenantApi.listWorkspaceImageJobs()).items ?? [];
  },
  createTask(body: ConsoleImageGenerateRequest | FormData) {
    if (body instanceof FormData) {
      return runtimeTransport.formRequest<PortalImageTaskCreateResponse>("POST", `${runtimeBasePath}/images/tasks`, body);
    }
    return runtimeTransport.request<PortalImageTaskCreateResponse>("POST", `${runtimeBasePath}/images/tasks`, body);
  },
  getTask(taskId: string) {
    return runtimeTransport.request<ConsoleImageJob>("GET", `${runtimeBasePath}/images/tasks/${encodeURIComponent(taskId)}`);
  },
  cancelTask(taskId: string) {
    return runtimeTransport.request<ConsoleImageJob>("POST", `${runtimeBasePath}/images/tasks/${encodeURIComponent(taskId)}/cancel`);
  },
  deleteTask(taskId: string) {
    return runtimeTransport.request<{ deleted: boolean }>("DELETE", `${runtimeBasePath}/images/tasks/${encodeURIComponent(taskId)}`);
  }
};

export const runtimeTaskApi = {
  listTasks(query: PortalTaskQuery = {}) {
    return runtimeTransport.request<PortalTaskPage>("GET", `${runtimeBasePath}/tasks`, undefined, {
      owner_scope: query.owner_scope || undefined,
      user_id: query.user_id || undefined,
      status: query.status || undefined,
      type: query.type || undefined,
      limit: query.limit,
      starting_after: query.starting_after || undefined
    });
  },
  getTask(taskId: string) {
    return runtimeTransport.request<PortalTaskRecord>("GET", `${runtimeBasePath}/tasks/${encodeURIComponent(taskId)}`);
  },
  cancelTask(taskId: string) {
    return runtimeTransport.request<PortalTaskRecord>("POST", `${runtimeBasePath}/tasks/${encodeURIComponent(taskId)}/cancel`);
  },
  deleteTask(taskId: string) {
    return runtimeTransport.request<{ deleted: boolean }>("DELETE", `${runtimeBasePath}/tasks/${encodeURIComponent(taskId)}`);
  }
};

export async function streamRuntimeChatMessage(opts: {
  sessionId: string;
  model?: string;
  messages: Array<{ role: string; content: string }>;
  signal?: AbortSignal;
  onDelta: (delta: string) => void;
  onEvent?: (eventType: string) => void;
}): Promise<void> {
  return runtimeTransport.streamChatMessage(opts);
}
