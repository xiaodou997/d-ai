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
import type {
  PortalTaskPage,
  PortalTaskQuery,
  PortalTaskRecord
} from "@/platform/ai/tasks";
import { portalEnv } from "@/env";
import { useAuthStore } from "@/stores/auth";
import type {
  AiApiKeyRevealOutput,
  AiApiKeyWriteRequest,
  ConsoleImageGenerateRequest,
  ConsoleImageJob,
  ConsoleImageModel,
} from "./types/aiCustomer";
import {
  toApiKey,
  toApiKeys,
  toChatModels,
  toChatSession,
  toChatSessionDetail,
  toChatSessions,
  toCreatedApiKey,
  toCurrentSubscription,
  toGroupPrices,
  toImageJobs,
  toOrder,
  toOrderPage,
  toPlanPage,
  toPurchaseResult,
  toSubscriptionPage,
  toVisibleGroups
} from "./aiCustomerMappers";
import { createTypedOperationRequest, type OperationBody } from ".";

function request() {
  return authenticatedRequest();
}

const headers = () => apiHeaders;
const baseUrl = () => apiBaseUrl;
const runtimeBasePath = "/runtime/v1";
const typedRequest = createTypedOperationRequest(request());

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

function toApiKeyStatus(value: string): OperationBody<"ai-update-user-self-api-key-status">["status"] {
  if (value === "active" || value === "disabled") return value;
  throw new Error(`Unexpected API key status: ${value}`);
}

function toApiKeyWriteBody(
  value: AiApiKeyWriteRequest
): OperationBody<"ai-create-user-self-api-key"> {
  return {
    name: value.name,
    group_id: value.group_id,
    quota_limit_micro_usd: value.quota_limit_micro_usd,
    status: value.status ? toApiKeyStatus(value.status) : undefined,
    expires_at: value.expires_at,
    limit_policy: value.limit_policy
      ? {
          concurrency_limit: value.limit_policy.concurrency_limit,
          status: value.limit_policy.status
        }
      : undefined
  };
}

// ==================== AI 用户自助扁平端点（userType=4，身份由 JWT claims 推导） ====================

export const aiCustomerApi = {
  // ---- API Key ----
  listApiKeys() {
    return typedRequest<"ai-list-user-self-api-keys">({
      method: "GET",
      path: "/api/v1/user-api-keys",
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toApiKeys);
  },
  createApiKey(body: AiApiKeyWriteRequest) {
    return typedRequest<"ai-create-user-self-api-key">({
      method: "POST",
      path: "/api/v1/users/me/api-keys",
      body: toApiKeyWriteBody(body),
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toCreatedApiKey);
  },
  updateApiKey(apiKeyId: string, body: AiApiKeyWriteRequest) {
    return typedRequest<"ai-update-user-self-api-key">({
      method: "PATCH",
      path: `/api/v1/users/me/api-keys/${encodeURIComponent(apiKeyId)}`,
      pathParams: { apiKeyID: apiKeyId },
      body: toApiKeyWriteBody(body),
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toApiKey);
  },
  updateApiKeyStatus(apiKeyId: string, status: string) {
    return typedRequest<"ai-update-user-self-api-key-status">({
      method: "PATCH",
      path: `/api/v1/users/me/api-keys/${encodeURIComponent(apiKeyId)}/status`,
      pathParams: { apiKeyID: apiKeyId },
      body: { status: toApiKeyStatus(status) },
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toApiKey);
  },
  rotateApiKey(apiKeyId: string) {
    return typedRequest<"ai-rotate-user-self-api-key">({
      method: "POST",
      path: `/api/v1/users/me/api-keys/${encodeURIComponent(apiKeyId)}/rotate`,
      pathParams: { apiKeyID: apiKeyId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toCreatedApiKey);
  },
  revealApiKey(apiKeyId: string) {
    return typedRequest<"ai-reveal-user-self-api-key">({
      method: "POST",
      path: `/api/v1/users/me/api-keys/${encodeURIComponent(apiKeyId)}/reveal`,
      pathParams: { apiKeyID: apiKeyId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then((value): AiApiKeyRevealOutput => ({ plaintext_key: value.plaintext_key }));
  },
  deleteApiKey(apiKeyId: string) {
    return typedRequest<"ai-delete-user-self-api-key">({
      method: "DELETE",
      path: `/api/v1/users/me/api-keys/${encodeURIComponent(apiKeyId)}`,
      pathParams: { apiKeyID: apiKeyId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then((value) => ({ deleted: value.deleted }));
  },
  listMyGroups() {
    return typedRequest<"ai-list-user-self-groups">({
      method: "GET",
      path: "/api/v1/users/me/groups",
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toVisibleGroups);
  },
  getMyGroupEffectivePrices(groupId: string) {
    return typedRequest<"ai-list-user-self-group-effective-prices">({
      method: "GET",
      path: `/api/v1/users/me/groups/${encodeURIComponent(groupId)}/effective-prices`,
      pathParams: { groupID: groupId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toGroupPrices);
  },

  // ---- 订阅制套餐（终端用户自助，docs/ai-subscription-design.md §7.1） ----
  listSubscriptionPlans(params: { limit?: number; offset?: number } = {}) {
    return typedRequest<"ai-list-user-self-subscription-plans">({
      method: "GET",
      path: "/api/v1/users/me/subscription-plans",
      query: params,
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toPlanPage);
  },
  createSubscriptionOrder(planId: string, idempotencyKey: string) {
    return typedRequest<"ai-create-user-self-subscription-order">({
      method: "POST",
      path: "/api/v1/users/me/subscription-orders",
      body: { plan_id: planId },
      headers: { ...headers(), "Idempotency-Key": idempotencyKey },
      baseUrl: baseUrl()
    }).then(toPurchaseResult);
  },
  listSubscriptionOrders(params: { limit?: number; offset?: number } = {}) {
    return typedRequest<"ai-list-user-self-subscription-orders">({
      method: "GET",
      path: "/api/v1/users/me/subscription-orders",
      query: params,
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toOrderPage);
  },
  getSubscriptionOrder(orderId: string) {
    return typedRequest<"ai-get-user-self-subscription-order">({
      method: "GET",
      path: `/api/v1/users/me/subscription-orders/${encodeURIComponent(orderId)}`,
      pathParams: { orderID: orderId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toOrder);
  },
  listMySubscriptions(params: { limit?: number; offset?: number } = {}) {
    return typedRequest<"ai-list-user-self-subscriptions">({
      method: "GET",
      path: "/api/v1/users/me/subscriptions",
      query: params,
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toSubscriptionPage);
  },
  getCurrentSubscription() {
    return typedRequest<"ai-get-user-self-current-subscription">({
      method: "GET",
      path: "/api/v1/users/me/subscriptions/current",
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toCurrentSubscription);
  },
  listWorkspaceChatSessions(params: { limit?: number } = {}, signal?: AbortSignal) {
    return typedRequest<"ai-api-v1-users-me-workspace-chat-sessions">({
      method: "GET",
      path: "/api/v1/users/me/workspace/chat/sessions",
      query: { limit: 50, ...params },
      headers: headers(),
      baseUrl: baseUrl(),
      signal
    }).then(toChatSessions);
  },
  listWorkspaceChatModels() {
    return typedRequest<"ai-api-v1-users-me-workspace-chat-models">({
      method: "GET",
      path: "/api/v1/users/me/workspace/chat/models",
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toChatModels);
  },
  createWorkspaceChatSession(
    body: OperationBody<"ai-api-v1-users-me-workspace-chat-sessions:create">
  ) {
    return typedRequest<"ai-api-v1-users-me-workspace-chat-sessions:create">({
      method: "POST",
      path: "/api/v1/users/me/workspace/chat/sessions",
      body,
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toChatSession);
  },
  getWorkspaceChatSession(sessionId: string) {
    return typedRequest<"ai-api-v1-users-me-workspace-chat-sessions-sessionid">({
      method: "GET",
      path: `/api/v1/users/me/workspace/chat/sessions/${encodeURIComponent(sessionId)}`,
      pathParams: { sessionID: sessionId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toChatSessionDetail);
  },
  deleteWorkspaceChatSession(sessionId: string) {
    return typedRequest<"ai-api-v1-users-me-workspace-chat-sessions-sessionid:delete">({
      method: "DELETE",
      path: `/api/v1/users/me/workspace/chat/sessions/${encodeURIComponent(sessionId)}`,
      pathParams: { sessionID: sessionId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then((value) => ({ deleted: value.deleted }));
  },
  listWorkspaceImageJobs(params: { limit?: number } = {}, signal?: AbortSignal) {
    return typedRequest<"ai-api-v1-users-me-workspace-image-jobs">({
      method: "GET",
      path: "/api/v1/users/me/workspace/image/jobs",
      query: { limit: 50, ...params },
      headers: headers(),
      baseUrl: baseUrl(),
      signal
    }).then(toImageJobs);
  }
};

// ==================== 网页工作台执行端点（信封 + SSE） ====================

export const runtimeChatApi = {
  async listModels() {
    return (await aiCustomerApi.listWorkspaceChatModels()).items ?? [];
  },
  async listSessions() {
    return (await aiCustomerApi.listWorkspaceChatSessions()).items ?? [];
  },
  createSession(body: {
    model_code: string;
    group_id: string;
    title?: string;
  }) {
    return aiCustomerApi.createWorkspaceChatSession(body);
  },
  getSession(sessionId: string) {
    return aiCustomerApi.getWorkspaceChatSession(sessionId);
  },
  deleteSession(sessionId: string) {
    return aiCustomerApi.deleteWorkspaceChatSession(sessionId);
  }
};

export const runtimeImageApi = {
  async listModels() {
    return runtimeTransport.request<ConsoleImageModel[]>("GET", `${runtimeBasePath}/images/models`);
  },
  async listJobs() {
    return (await aiCustomerApi.listWorkspaceImageJobs()).items ?? [];
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
