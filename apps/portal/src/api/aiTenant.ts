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
  type OperationQuery,
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
type PriceBookTransport = components["schemas"]["PriceBookDTO"];
type PriceBookEntryTransport = components["schemas"]["PriceBookEntryDTO"];
type PriceBookPageTransport = OperationResponse<"ai-list-tenant-price-books">;
type PriceBookEntriesPageTransport = OperationResponse<"ai-list-tenant-price-book-entries">;
type PriceBookTransferTransport = OperationResponse<"ai-export-tenant-price-book">;
type AvailableModelsTransport = OperationResponse<"ai-list-tenant-self-available-models">;
type UpstreamResourcesTransport = OperationResponse<"ai-list-tenant-upstream-resources">;
type GroupEffectivePricesTransport = OperationResponse<"ai-list-tenant-self-group-effective-prices">;
type UserGroupsTransport = OperationResponse<"ai-list-user-groups">;
type LimitPoliciesTransport = OperationResponse<"ai-list-tenant-self-user-limit-policies">;
type DashboardSummaryTransport = OperationResponse<"ai-get-tenant-self-dashboard-summary">;
type DashboardTopModelsTransport = OperationResponse<"ai-list-tenant-self-dashboard-top-models">;
type DashboardRecentErrorsTransport = OperationResponse<"ai-list-tenant-self-dashboard-recent-errors">;

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

function toPriceBook(value: PriceBookTransport): TenantAiPriceBook {
  if (value.owner_type !== "platform" && value.owner_type !== "tenant") {
    throw new Error(`Unexpected price book owner type: ${value.owner_type}`);
  }
  if (value.status !== "active" && value.status !== "disabled") {
    throw new Error(`Unexpected price book status: ${value.status}`);
  }
  return { ...stripSchema(value), owner_type: value.owner_type, status: value.status };
}

function toPriceBookPage(value: PriceBookPageTransport): { items: TenantAiPriceBook[]; total: number } {
  return { items: value.items?.map(toPriceBook) ?? [], total: value.total };
}

function toPriceBookEntry(value: PriceBookEntryTransport): TenantAiPriceBookEntry {
  return {
    ...stripSchema(value),
    token_price_tiers: value.token_price_tiers?.map(stripSchema) ?? [],
    image_prices: value.image_prices?.map(stripSchema) ?? undefined,
    video_prices: value.video_prices?.map(stripSchema) ?? undefined
  };
}

function toPriceBookEntriesPage(value: PriceBookEntriesPageTransport): { items: TenantAiPriceBookEntry[]; total: number } {
  return { items: value.items?.map(toPriceBookEntry) ?? [], total: value.total };
}

function toPriceBookTransfer(value: PriceBookTransferTransport): TenantAiPriceBookTransferBundle {
  if (value.schema_version !== 1) throw new Error(`Unsupported price book schema version: ${value.schema_version}`);
  return {
    schema_version: 1,
    name: value.name,
    description: value.description,
    entries: value.entries?.map(toPriceBookEntry) ?? []
  };
}

function toPriceBookEntryBody(value: TenantAiPriceBookEntryWriteRequest): OperationBody<"ai-upsert-tenant-price-book-entry"> {
  return {
    capability_type: value.capability_type,
    token_price_tiers: value.token_price_tiers,
    image_default_price_usd: value.image_default_price_usd,
    video_default_price_usd: value.video_default_price_usd,
    image_prices: value.image_prices,
    video_prices: value.video_prices,
    audio_tts_per_1m_chars_usd: value.audio_tts_per_1m_chars_usd,
    audio_stt_per_minute_usd: value.audio_stt_per_minute_usd
  };
}

function toPriceBookTransferBody(value: TenantAiPriceBookTransferBundle): OperationBody<"ai-import-tenant-price-book"> {
  if (value.schema_version !== 1) throw new Error(`Unsupported price book schema version: ${value.schema_version}`);
  return {
    schema_version: 1,
    name: value.name,
    description: value.description,
    entries: value.entries.map((entry) => ({
      model_code: entry.model_code,
      capability_type: entry.capability_type,
      token_price_tiers: entry.token_price_tiers,
      image_default_price_usd: entry.image_default_price_usd,
      video_default_price_usd: entry.video_default_price_usd,
      image_prices: entry.image_prices ?? null,
      video_prices: entry.video_prices ?? null,
      audio_tts_per_1m_chars_usd: entry.audio_tts_per_1m_chars_usd,
      audio_stt_per_minute_usd: entry.audio_stt_per_minute_usd,
      source: entry.source ?? "manual",
      manually_edited: entry.manually_edited ?? true,
      updated_at: entry.updated_at
    }))
  };
}

function toAvailableModels(value: AvailableModelsTransport): TenantAiAvailableModelsOutputBody {
  return {
    items: value.items?.map((item) => ({
      model_code: item.model_code,
      model_name: item.model_name,
      capability_type: item.capability_type,
      input_per_1m_usd_min: item.input_per_1m_usd_min,
      input_per_1m_usd_max: item.input_per_1m_usd_max,
      output_per_1m_usd_min: item.output_per_1m_usd_min,
      output_per_1m_usd_max: item.output_per_1m_usd_max,
      cache_write_per_1m_usd_min: item.cache_write_per_1m_usd_min,
      cache_write_per_1m_usd_max: item.cache_write_per_1m_usd_max,
      cache_read_per_1m_usd_min: item.cache_read_per_1m_usd_min,
      cache_read_per_1m_usd_max: item.cache_read_per_1m_usd_max,
      has_context_tiers: item.has_context_tiers,
      image_default_price_usd_min: item.image_default_price_usd_min,
      image_default_price_usd_max: item.image_default_price_usd_max,
      video_default_price_usd_min: item.video_default_price_usd_min,
      video_default_price_usd_max: item.video_default_price_usd_max,
      image_prices: item.image_prices?.map(stripSchema) ?? undefined,
      video_prices: item.video_prices?.map(stripSchema) ?? undefined
    })) ?? [],
    total: value.total
  };
}

function toUpstreamResources(value: UpstreamResourcesTransport): { items: TenantAiUpstreamResource[]; total: number } {
  return {
    items: value.items?.map((resource) => {
      if (resource.resource_kind !== "direct_upstream" && resource.resource_kind !== "oauth_pool") {
        throw new Error(`Unexpected upstream resource kind: ${resource.resource_kind}`);
      }
      return {
        id: resource.id,
        resource_kind: resource.resource_kind,
        name: resource.name,
        tenant_multiplier: resource.tenant_multiplier,
        price_book_id: resource.price_book_id,
        price_book_name: resource.price_book_name,
        price_book_revision: resource.price_book_revision,
        models: resource.models?.map((model) => {
          if (model.availability !== "available" && model.availability !== "no_price_configured") {
            throw new Error(`Unexpected upstream model availability: ${model.availability}`);
          }
          return {
            model_code: model.model_code,
            capability_type: model.capability_type,
            api_format: model.api_format,
            availability: model.availability,
            price: model.price ? toPriceBookEntry(model.price) : undefined
          };
        }) ?? []
      };
    }) ?? [],
    total: value.total
  };
}

function toGroupEffectivePrices(value: GroupEffectivePricesTransport): TenantAiGroupEffectivePricesOutputBody {
  return {
    group_id: value.group_id,
    retail_price_book_id: value.retail_price_book_id,
    effective_user_multiplier: value.effective_user_multiplier,
    items: value.items?.map((item) => ({
      model_code: item.model_code,
      capability_type: item.capability_type,
      token_price_tiers: item.token_price_tiers?.map(stripSchema) ?? [],
      image_default_price_usd: item.image_default_price_usd,
      video_default_price_usd: item.video_default_price_usd,
      image_prices: item.image_prices?.map(stripSchema) ?? undefined,
      video_prices: item.video_prices?.map(stripSchema) ?? undefined,
      audio_tts_per_1m_chars_usd: item.audio_tts_per_1m_chars_usd,
      audio_stt_per_minute_usd: item.audio_stt_per_minute_usd
    })) ?? [],
    total: value.total
  };
}

function toUserGroups(value: UserGroupsTransport): TenantAiUserGroupsOutputBody {
  return {
    items: value.items?.map((item) => ({
      group_id: item.group_id,
      group_name: item.group_name,
      multiplier_override: item.multiplier_override
    })) ?? [],
    total: value.total
  };
}

function toLimitPolicy(value: components["schemas"]["RuntimeLimitPolicyDTO"]): TenantAiLimitPolicy {
  if (value.scope_type !== "tenant" && value.scope_type !== "user" && value.scope_type !== "api_key") {
    throw new Error(`Unexpected limit policy scope: ${value.scope_type}`);
  }
  if (value.status !== "active" && value.status !== "disabled") {
    throw new Error(`Unexpected limit policy status: ${value.status}`);
  }
  return {
    id: value.id,
    scope_type: value.scope_type,
    scope_id: value.scope_id,
    concurrency_limit: value.concurrency_limit,
    status: value.status,
    created_by: value.created_by,
    created_at: value.created_at,
    updated_at: value.updated_at
  };
}

function toLimitPolicies(value: LimitPoliciesTransport): TenantAiLimitPoliciesOutputBody {
  return { items: value.items?.map(toLimitPolicy) ?? [], total: value.total };
}

function toDashboardSummary(value: DashboardSummaryTransport): TenantAiDashboardSummary {
  return {
    total_requests: value.total_requests,
    successful_requests: value.successful_requests,
    failed_requests: value.failed_requests,
    total_tokens: value.total_tokens,
    total_prompt_tokens: value.total_prompt_tokens,
    total_completion_tokens: value.total_completion_tokens,
    total_catalog_base_usd: value.total_catalog_base_usd,
    total_tenant_payable_usd: value.total_tenant_payable_usd,
    total_retail_base_usd: value.total_retail_base_usd,
    total_user_payable_usd: value.total_user_payable_usd,
    total_user_charged_usd: value.total_user_charged_usd,
    avg_latency_ms: value.avg_latency_ms
  };
}

function toDashboardTopModels(value: DashboardTopModelsTransport): TenantAiDashboardTopModelsOutputBody {
  return { items: value.items?.map((item) => stripSchema(item)) ?? [], total: value.total };
}

function toDashboardRecentErrors(value: DashboardRecentErrorsTransport): TenantAiDashboardRecentErrorsOutputBody {
  return {
    items: value.items?.map((item) => ({
      request_id: item.request_id,
      model_code: item.model_code,
      request_status: item.request_status,
      error_code: item.error_code,
      error_message: item.error_message,
      http_status: item.http_status,
      created_at: item.created_at
    })) ?? [],
    total: value.total
  };
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
    return typedRequest<"ai-list-tenant-self-available-models">({
      method: "GET",
      path: "/api/v1/tenants/me/available-models",
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toAvailableModels);
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
    return typedRequest<"ai-list-tenant-price-books">({
      method: "GET", path: "/api/v1/tenants/me/price-books", headers: headers(), baseUrl: baseUrl()
    }).then(toPriceBookPage);
  },
  createPriceBook(body: { name: string; description?: string }) {
    return typedRequest<"ai-create-tenant-price-book">({
      method: "POST", path: "/api/v1/tenants/me/price-books", headers: headers(), body, baseUrl: baseUrl()
    }).then(toPriceBook);
  },
  updatePriceBook(bookId: string, body: { name: string; description?: string; status?: "active" | "disabled" }) {
    return typedRequest<"ai-update-tenant-price-book">({
      method: "PATCH", path: `/api/v1/tenants/me/price-books/${encodeURIComponent(bookId)}`,
      pathParams: { bookID: bookId }, headers: headers(), body, baseUrl: baseUrl()
    }).then(toPriceBook);
  },
  deletePriceBook(bookId: string) {
    return typedRequest<"ai-delete-tenant-price-book">({
      method: "DELETE", path: `/api/v1/tenants/me/price-books/${encodeURIComponent(bookId)}`,
      pathParams: { bookID: bookId }, headers: headers(), baseUrl: baseUrl()
    }).then((value) => ({ deleted: value.deleted }));
  },
  copyPriceBook(bookId: string, name?: string) {
    return typedRequest<"ai-clone-tenant-price-book">({
      method: "POST", path: `/api/v1/tenants/me/price-books/${encodeURIComponent(bookId)}/clone`,
      pathParams: { bookID: bookId }, headers: headers(), body: { name }, baseUrl: baseUrl()
    }).then(toPriceBook);
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
    return typedRequest<"ai-list-tenant-price-book-entries">({
      method: "GET", path: `/api/v1/tenants/me/price-books/${encodeURIComponent(bookId)}/entries`,
      pathParams: { bookID: bookId }, headers: headers(), baseUrl: baseUrl()
    }).then(toPriceBookEntriesPage);
  },
  upsertPriceBookEntry(bookId: string, modelCode: string, body: TenantAiPriceBookEntryWriteRequest) {
    return typedRequest<"ai-upsert-tenant-price-book-entry">({
      method: "PUT", path: `/api/v1/tenants/me/price-books/${encodeURIComponent(bookId)}/entries/${encodeURIComponent(modelCode)}`,
      pathParams: { bookID: bookId, modelCode }, headers: headers(), body: toPriceBookEntryBody(body), baseUrl: baseUrl()
    }).then(toPriceBookEntry);
  },
  deletePriceBookEntry(bookId: string, modelCode: string, capabilityType: string) {
    return typedRequest<"ai-delete-tenant-price-book-entry">({
      method: "DELETE", path: `/api/v1/tenants/me/price-books/${encodeURIComponent(bookId)}/entries/${encodeURIComponent(modelCode)}`,
      pathParams: { bookID: bookId, modelCode }, query: { capability_type: capabilityType }, headers: headers(), baseUrl: baseUrl()
    }).then((value) => ({ deleted: value.deleted }));
  },
  syncCommonPriceModels(bookId: string) {
    return typedRequest<"ai-sync-tenant-common-price-models">({
      method: "POST", path: `/api/v1/tenants/me/price-books/${encodeURIComponent(bookId)}/sync-common`,
      pathParams: { bookID: bookId }, headers: headers(), baseUrl: baseUrl()
    }).then((value) => ({ synced: value.synced, missing: value.missing ?? [] }));
  },
  importPriceBook(body: TenantAiPriceBookTransferBundle) {
    return typedRequest<"ai-import-tenant-price-book">({
      method: "POST", path: "/api/v1/tenants/me/price-books/import", headers: headers(), body: toPriceBookTransferBody(body), baseUrl: baseUrl()
    }).then(toPriceBook);
  },
  exportPriceBook(bookId: string) {
    return typedRequest<"ai-export-tenant-price-book">({
      method: "GET", path: `/api/v1/tenants/me/price-books/${encodeURIComponent(bookId)}/export`,
      pathParams: { bookID: bookId }, headers: headers(), baseUrl: baseUrl()
    }).then(toPriceBookTransfer);
  },

  // ---- 不含地址、密钥和内部模型名的上游目录 ----
  listUpstreamResources() {
    return typedRequest<"ai-list-tenant-upstream-resources">({
      method: "GET", path: "/api/v1/tenants/me/upstream-resources", headers: headers(), baseUrl: baseUrl()
    }).then(toUpstreamResources);
  },
  // ---- 某可见分组对本租户的每模型生效 USD 单价 ----
  getMyGroupEffectivePrices(groupId: string) {
    return typedRequest<"ai-list-tenant-self-group-effective-prices">({
      method: "GET",
      path: `/api/v1/tenants/me/groups/${encodeURIComponent(groupId)}/effective-prices`,
      pathParams: { groupID: groupId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toGroupEffectivePrices);
  },
  // ---- 租户→用户 分组绑定（套餐收窄 + 加价倍率） ----
  listUserGroups(userId: string) {
    return typedRequest<"ai-list-user-groups">({
      method: "GET",
      path: `/api/v1/tenants/me/users/${encodeURIComponent(userId)}/groups`,
      pathParams: { userID: userId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toUserGroups);
  },
  upsertUserGroup(userId: string, groupId: string, body: TenantAiUserGroupWriteRequest) {
    return typedRequest<"ai-upsert-user-group">({
      method: "PUT",
      path: `/api/v1/tenants/me/users/${encodeURIComponent(userId)}/groups/${encodeURIComponent(groupId)}`,
      pathParams: { userID: userId, groupID: groupId },
      headers: headers(),
      body: { multiplier_override: body.multiplier_override ?? undefined },
      baseUrl: baseUrl()
    }).then((value) => ({ deleted: value.deleted }));
  },
  deleteUserGroup(userId: string, groupId: string) {
    return typedRequest<"ai-delete-user-group">({
      method: "DELETE",
      path: `/api/v1/tenants/me/users/${encodeURIComponent(userId)}/groups/${encodeURIComponent(groupId)}`,
      pathParams: { userID: userId, groupID: groupId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then((value) => ({ deleted: value.deleted }));
  },

  // ---- 租户限流策略 ----
  listUserLimitPolicies(userId: string) {
    return typedRequest<"ai-list-tenant-self-user-limit-policies">({
      method: "GET",
      path: `/api/v1/tenants/me/users/${encodeURIComponent(userId)}/limit-policies`,
      pathParams: { userID: userId },
      headers: headers(),
      baseUrl: baseUrl()
    }).then(toLimitPolicies);
  },
  upsertUserLimitPolicy(userId: string, body: TenantAiLimitPolicyWriteRequest) {
    return typedRequest<"ai-upsert-tenant-self-user-limit-policy">({
      method: "PUT",
      path: `/api/v1/tenants/me/users/${encodeURIComponent(userId)}/limit-policies`,
      pathParams: { userID: userId },
      headers: headers(),
      body: { concurrency_limit: body.concurrency_limit, status: body.status },
      baseUrl: baseUrl()
    }).then(toLimitPolicy);
  },

  // ---- Dashboard（tenant 维度，claims-scoped） ----
  getDashboardSummary(params: OperationQuery<"ai-get-tenant-self-dashboard-summary"> = {}) {
    return typedRequest<"ai-get-tenant-self-dashboard-summary">({
      method: "GET",
      path: "/api/v1/tenants/me/dashboard/summary",
      headers: headers(),
      query: params,
      baseUrl: baseUrl()
    }).then(toDashboardSummary);
  },
  getDashboardTopModels(params: OperationQuery<"ai-list-tenant-self-dashboard-top-models"> = {}) {
    return typedRequest<"ai-list-tenant-self-dashboard-top-models">({
      method: "GET",
      path: "/api/v1/tenants/me/dashboard/top-models",
      headers: headers(),
      query: params,
      baseUrl: baseUrl()
    }).then(toDashboardTopModels);
  },
  listDashboardRecentErrors(params: OperationQuery<"ai-list-tenant-self-dashboard-recent-errors"> = {}) {
    return typedRequest<"ai-list-tenant-self-dashboard-recent-errors">({
      method: "GET",
      path: "/api/v1/tenants/me/dashboard/recent-errors",
      headers: headers(),
      query: params,
      baseUrl: baseUrl()
    }).then(toDashboardRecentErrors);
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
