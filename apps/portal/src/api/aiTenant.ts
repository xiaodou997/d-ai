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

const headers = () => apiHeaders;
const baseUrl = () => apiBaseUrl;
const runtimeBasePath = "/runtime/v1";

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
    return request()<TenantAiApiKeysOutputBody>({
      method: "GET",
      path: "/api/v1/tenant-api-keys",
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  createApiKey(body: TenantAiApiKeyWriteRequest) {
    return request()<TenantAiApiKeyCreatedOutputBody>({
      method: "POST",
      path: "/api/v1/tenants/me/api-keys",
      headers: headers(),
      body,
      baseUrl: baseUrl()
    });
  },
  updateApiKey(apiKeyId: string, body: TenantAiApiKeyWriteRequest) {
    return request()<TenantAiApiKey>({
      method: "PATCH",
      path: `/api/v1/tenants/me/api-keys/${encodeURIComponent(apiKeyId)}`,
      headers: headers(),
      body,
      baseUrl: baseUrl()
    });
  },
  updateApiKeyStatus(apiKeyId: string, status: string) {
    return request()<TenantAiApiKey>({
      method: "PATCH",
      path: `/api/v1/tenants/me/api-keys/${encodeURIComponent(apiKeyId)}/status`,
      headers: headers(),
      body: { status },
      baseUrl: baseUrl()
    });
  },
  revealApiKey(apiKeyId: string) {
    return request()<TenantAiApiKeyRevealOutputBody>({
      method: "POST",
      path: `/api/v1/tenants/me/api-keys/${encodeURIComponent(apiKeyId)}/reveal`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  rotateApiKey(apiKeyId: string) {
    return request()<TenantAiApiKeyCreatedOutputBody>({
      method: "POST",
      path: `/api/v1/tenants/me/api-keys/${encodeURIComponent(apiKeyId)}/rotate`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  deleteApiKey(apiKeyId: string) {
    return request()<TenantAiDeleteOutputBody>({
      method: "DELETE",
      path: `/api/v1/tenants/me/api-keys/${encodeURIComponent(apiKeyId)}`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },

  // ---- 租户自有分组 ----
  listMyGroups() {
    return request()<TenantAiVisibleGroupsOutputBody>({
      method: "GET",
      path: "/api/v1/tenants/me/groups",
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  getGroup(groupId: string) {
    return request()<TenantAiVisibleGroup>({
      method: "GET",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  createGroup(body: TenantAiGroupWriteRequest) {
    return request()<TenantAiVisibleGroup>({
      method: "POST",
      path: "/api/v1/tenants/me/groups",
      headers: headers(),
      body,
      baseUrl: baseUrl()
    });
  },
  updateGroup(groupId: string, body: TenantAiGroupWriteRequest) {
    return request()<TenantAiVisibleGroup>({
      method: "PATCH",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}`,
      headers: headers(),
      body,
      baseUrl: baseUrl()
    });
  },
  updateGroupStatus(groupId: string, status: "active" | "disabled") {
    return request()<TenantAiVisibleGroup>({
      method: "PATCH",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/status`,
      headers: headers(),
      body: { status },
      baseUrl: baseUrl()
    });
  },
  deleteGroup(groupId: string) {
    return request()<TenantAiDeleteOutputBody>({
      method: "DELETE",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  getGroupClientSurfacePolicy(groupId: string) {
    return request()<TenantAiClientSurfacePolicy>({
      method: "GET",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/client-surface-policy`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  replaceGroupClientSurfacePolicy(groupId: string, body: TenantAiClientSurfacePolicyWrite) {
    return request()<TenantAiClientSurfacePolicy>({
      method: "PUT",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/client-surface-policy`,
      headers: headers(),
      body,
      baseUrl: baseUrl()
    });
  },
  listGroupDispatchRules(groupId: string) {
    return request()<{ items: TenantAiDispatchRule[] | null; total: number }>({
      method: "GET",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/dispatch-rules`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  createGroupDispatchRule(groupId: string, body: TenantAiDispatchRuleWriteRequest) {
    return request()<TenantAiDispatchRule>({
      method: "POST",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/dispatch-rules`,
      headers: headers(),
      body,
      baseUrl: baseUrl()
    });
  },
  updateGroupDispatchRule(groupId: string, ruleId: string, body: TenantAiDispatchRuleWriteRequest) {
    return request()<TenantAiDispatchRule>({
      method: "PATCH",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/dispatch-rules/${encodeURIComponent(ruleId)}`,
      headers: headers(),
      body,
      baseUrl: baseUrl()
    });
  },
  updateGroupDispatchRuleStatus(groupId: string, ruleId: string, status: "active" | "disabled") {
    return request()<TenantAiDispatchRule>({
      method: "PATCH",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/dispatch-rules/${encodeURIComponent(ruleId)}/status`,
      headers: headers(),
      body: { status },
      baseUrl: baseUrl()
    });
  },
  deleteGroupDispatchRule(groupId: string, ruleId: string) {
    return request()<TenantAiDeleteOutputBody>({
      method: "DELETE",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/dispatch-rules/${encodeURIComponent(ruleId)}`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  previewGroupDispatch(groupId: string, body: TenantAiDispatchPreviewRequest) {
    return request()<TenantAiDispatchPreview>({
      method: "POST",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/dispatch-rules/preview`,
      headers: headers(),
      body,
      baseUrl: baseUrl()
    });
  },
  listGroupDispatchModels(groupId: string, clientSurface: string) {
    return request()<{ items: TenantAiDispatchModel[] | null; total: number }>({
      method: "GET",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/dispatch-models`,
      query: { client_surface: clientSurface },
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  listGroupTargets(groupId: string) {
    return request()<{ items: TenantAiGroupTarget[]; total: number }>({
      method: "GET",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/targets`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  addGroupTarget(groupId: string, body: TenantAiGroupTargetWriteRequest) {
    return request()<TenantAiGroupTarget>({
      method: "POST",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/targets`,
      headers: headers(),
      body,
      baseUrl: baseUrl()
    });
  },
  updateGroupTarget(groupId: string, bindingId: string, body: TenantAiGroupTargetWriteRequest) {
    return request()<TenantAiGroupTarget>({
      method: "PATCH",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/targets/${encodeURIComponent(bindingId)}`,
      headers: headers(),
      body,
      baseUrl: baseUrl()
    });
  },
  deleteGroupTarget(groupId: string, bindingId: string) {
    return request()<TenantAiDeleteOutputBody>({
      method: "DELETE",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/targets/${encodeURIComponent(bindingId)}`,
      headers: headers(),
      baseUrl: baseUrl()
    });
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
    return request()<components["schemas"]["LiteLLMModelsOutputBody"]>({
      method: "GET", path: "/api/v1/tenants/me/price-books/litellm/models", query: { q, limit }, headers: headers(), baseUrl: baseUrl()
    }).then((response): TenantAiLiteLLMModelsOutput => ({
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
