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
  AiApiKey,
  AiApiKeyCreatedOutput,
  AiApiKeyRevealOutput,
  AiApiKeysOutput,
  AiApiKeyWriteRequest,
  AiGroupEffectivePricesOutput,
  AiVisibleGroupsOutput,
  AiSubPlan,
  AiSubscription,
  AiSubOrder,
  AiSubPurchaseResult,
  AiSubPage,
  ChatModel,
  ConsoleImageGenerateRequest,
  ConsoleImageJob,
  ConsoleImageModel,
  ChatSession,
  ChatSessionDetail
} from "./types/aiCustomer";

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

// ==================== AI 用户自助扁平端点（userType=4，身份由 JWT claims 推导） ====================

export const aiCustomerApi = {
  // ---- API Key ----
  listApiKeys() {
    return request()<AiApiKeysOutput>({
      method: "GET",
      path: "/api/v1/user-api-keys",
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  createApiKey(body: AiApiKeyWriteRequest) {
    return request()<AiApiKeyCreatedOutput>({
      method: "POST",
      path: "/api/v1/users/me/api-keys",
      body,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  updateApiKey(apiKeyId: string, body: AiApiKeyWriteRequest) {
    return request()<AiApiKey>({
      method: "PATCH",
      path: `/api/v1/users/me/api-keys/${encodeURIComponent(apiKeyId)}`,
      body,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  updateApiKeyStatus(apiKeyId: string, status: string) {
    return request()<AiApiKey>({
      method: "PATCH",
      path: `/api/v1/users/me/api-keys/${encodeURIComponent(apiKeyId)}/status`,
      body: { status },
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  rotateApiKey(apiKeyId: string) {
    return request()<AiApiKeyCreatedOutput>({
      method: "POST",
      path: `/api/v1/users/me/api-keys/${encodeURIComponent(apiKeyId)}/rotate`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  revealApiKey(apiKeyId: string) {
    return request()<AiApiKeyRevealOutput>({
      method: "POST",
      path: `/api/v1/users/me/api-keys/${encodeURIComponent(apiKeyId)}/reveal`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  deleteApiKey(apiKeyId: string) {
    return request()<{ deleted: boolean }>({
      method: "DELETE",
      path: `/api/v1/users/me/api-keys/${encodeURIComponent(apiKeyId)}`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  listMyGroups() {
    return request()<AiVisibleGroupsOutput>({
      method: "GET",
      path: "/api/v1/users/me/groups",
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  getMyGroupEffectivePrices(groupId: string) {
    return request()<AiGroupEffectivePricesOutput>({
      method: "GET",
      path: `/api/v1/users/me/groups/${encodeURIComponent(groupId)}/effective-prices`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },

  // ---- 订阅制套餐（终端用户自助，docs/ai-subscription-design.md §7.1） ----
  listSubscriptionPlans(params: { limit?: number; offset?: number } = {}) {
    return request()<AiSubPage<AiSubPlan>>({
      method: "GET",
      path: "/api/v1/users/me/subscription-plans",
      query: params,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  createSubscriptionOrder(planId: string, idempotencyKey: string) {
    return request()<AiSubPurchaseResult>({
      method: "POST",
      path: "/api/v1/users/me/subscription-orders",
      body: { plan_id: planId },
      headers: { ...headers(), "Idempotency-Key": idempotencyKey },
      baseUrl: baseUrl()
    });
  },
  listSubscriptionOrders(params: { limit?: number; offset?: number } = {}) {
    return request()<AiSubPage<AiSubOrder>>({
      method: "GET",
      path: "/api/v1/users/me/subscription-orders",
      query: params,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  getSubscriptionOrder(orderId: string) {
    return request()<AiSubOrder>({
      method: "GET",
      path: `/api/v1/users/me/subscription-orders/${encodeURIComponent(orderId)}`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  listMySubscriptions(params: { limit?: number; offset?: number } = {}) {
    return request()<AiSubPage<AiSubscription>>({
      method: "GET",
      path: "/api/v1/users/me/subscriptions",
      query: params,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  getCurrentSubscription() {
    return request()<AiSubscription | null>({
      method: "GET",
      path: "/api/v1/users/me/subscriptions/current",
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  listWorkspaceChatSessions(params: { limit?: number } = {}, signal?: AbortSignal) {
    return request()<{ items: ChatSession[]; total: number }>({
      method: "GET",
      path: "/api/v1/users/me/workspace/chat/sessions",
      query: { limit: 50, ...params },
      headers: headers(),
      baseUrl: baseUrl(),
      signal
    });
  },
  listWorkspaceChatModels() {
    return request()<{ items: ChatModel[]; total: number }>({
      method: "GET",
      path: "/api/v1/users/me/workspace/chat/models",
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
      path: "/api/v1/users/me/workspace/chat/sessions",
      body,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  getWorkspaceChatSession(sessionId: string) {
    return request()<ChatSessionDetail>({
      method: "GET",
      path: `/api/v1/users/me/workspace/chat/sessions/${encodeURIComponent(sessionId)}`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  deleteWorkspaceChatSession(sessionId: string) {
    return request()<{ deleted: boolean }>({
      method: "DELETE",
      path: `/api/v1/users/me/workspace/chat/sessions/${encodeURIComponent(sessionId)}`,
      headers: headers(),
      baseUrl: baseUrl()
    });
  },
  listWorkspaceImageJobs(params: { limit?: number } = {}, signal?: AbortSignal) {
    return request()<{ items: ConsoleImageJob[]; total: number }>({
      method: "GET",
      path: "/api/v1/users/me/workspace/image/jobs",
      query: { limit: 50, ...params },
      headers: headers(),
      baseUrl: baseUrl(),
      signal
    });
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
