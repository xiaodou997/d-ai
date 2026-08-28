import { authenticatedRequest, apiHeaders, apiBaseUrl } from "./request";
import {
  createTypedOperationRequest,
  type OperationBody,
  type OperationQuery,
  type OperationResponse
} from ".";
import type { components } from "./generated/dai";
import type {
  AccountDTO,
  AccountsOutputBody,
  AccountWriteRequest,
  UpstreamAccountTransferBindingDTO,
  UpstreamAccountTransferAccountDTO,
  UpstreamAccountExportOutputBody,
  UpstreamAccountExportRequest,
  UpstreamAccountImportOutputBody,
  UpstreamAccountImportPreviewOutputBody,
  UpstreamAccountImportRequest,
  PriceBookDTO,
  PriceBookEntriesOutputBody,
  PriceBookEntryDTO,
  PriceBookEntryWriteRequest,
  PriceBooksOutputBody,
  PriceBookWriteRequest
} from "./types/admin";
import type {
  AuditLogsOutputBody,
  CredentialPoolDTO,
  DiscoveredUpstreamModelDTO,
  ImportUpstreamModelsRequest,
  ModelCapabilityInferResult,
  CredentialPoolWriteRequest,
  CredentialPoolsOutputBody,
  DashboardSummaryDTO,
  DashboardTopModelsOutputBody,
  DashboardTopTenantsOutputBody,
  DashboardRecentErrorsOutputBody,
  OAuthPoolHealthOutputBody,
  PoolAvailableModelsDTO,
  PoolCredentialDTO,
  PoolCredentialPatchRequest,
  PoolCredentialWriteRequest,
  PoolCredentialsOutputBody,
  UpstreamModelBindingDTO,
  UpstreamModelBindingsOutputBody,
  UpstreamModelBindingWriteRequest,
  UpstreamAccountTestRequest,
  UpstreamAccountTestResult,
  RouteWeightsOutputBody,
  RuntimeLimitPolicyDTO,
  RuntimeLimitPoliciesOutputBody,
  TenantUpstreamAccessDTO,
  TenantUpstreamAccessOutputBody,
  TenantUpstreamPolicyRef,
  SystemStatusDTO,
  RiskControlConfigDTO,
  RiskControlConfigWriteRequest,
  RiskControlTestResultDTO,
  RiskControlLogsOutputBody,
  RiskEventDTO,
  RiskEventsOutputBody,
  LiteLLMModelsOutputBody,
  LiteLLMModelInfo
} from "./types/ai";

function request() {
  return authenticatedRequest();
}

const typedRequest = createTypedOperationRequest(authenticatedRequest());

type PriceBookTransport = components["schemas"]["PriceBookDTO"];
type PriceBookEntryTransport = components["schemas"]["PriceBookEntryDTO"];
type PriceBookPageTransport = OperationResponse<"ai-list-price-books">;
type PriceBookEntriesPageTransport = OperationResponse<"ai-list-price-book-entries">;
type AccountTransport = components["schemas"]["AccountDTO"];
type AccountsPageTransport = OperationResponse<"ai-list-upstream-accounts">;
type AccountExportTransport = OperationResponse<"ai-export-upstream-accounts">;
type AccountImportPreviewTransport = OperationResponse<"ai-preview-import-upstream-accounts">;
type AccountImportTransport = OperationResponse<"ai-import-upstream-accounts">;
type AccountTransferTransport = components["schemas"]["UpstreamAccountTransferAccountDTO"];
type AccountTransferBindingTransport = components["schemas"]["UpstreamAccountTransferBindingDTO"];
type DiscoveredModelsTransport = OperationResponse<"ai-fetch-account-upstream-models">;
type UpstreamModelBindingTransport = components["schemas"]["UpstreamModelBindingDTO"];
type UpstreamModelBindingsTransport = OperationResponse<"ai-list-account-model-bindings">;
type AccountTestTransport = OperationResponse<"ai-test-account-upstream">;
type AccountModelImportTransport = OperationResponse<"ai-import-account-upstream-models">;
type PoolAvailableModelsTransport = OperationResponse<"ai-get-pool-available-models">;
type PoolModelImportTransport = OperationResponse<"ai-import-pool-available-models">;
type DashboardSummaryTransport = OperationResponse<"ai-get-dashboard-summary">;
type DashboardTopModelsTransport = OperationResponse<"ai-list-dashboard-top-models">;
type DashboardTopTenantsTransport = OperationResponse<"ai-list-dashboard-top-tenants">;
type DashboardRecentErrorsTransport = OperationResponse<"ai-list-dashboard-recent-errors">;
type AuditLogsTransport = OperationResponse<"ai-list-audit-logs">;
type RuntimeLimitPolicyTransport = components["schemas"]["RuntimeLimitPolicyDTO"];
type RuntimeLimitPoliciesTransport = OperationResponse<"ai-list-runtime-limit-policies">;
type TenantUpstreamAccessTransport = components["schemas"]["TenantUpstreamAccessDTO"];
type TenantUpstreamAccessPageTransport = OperationResponse<"ai-list-tenant-upstream-access">;
type LimitPolicyWriteBody = OperationBody<"ai-create-runtime-limit-policy">;
type LimitPolicyScopeType = LimitPolicyWriteBody["scope_type"];
type LimitPolicyStatus = NonNullable<LimitPolicyWriteBody["status"]>;
type TenantUpstreamAccessWriteBody = OperationBody<"ai-replace-tenant-upstream-access">;
type BindingWriteBody = OperationBody<"ai-create-account-model-binding">;
type BindingApiFormat = NonNullable<BindingWriteBody["api_format"]>;
type BindingCapabilityType = NonNullable<BindingWriteBody["capability_type"]>;
type BindingImageStreamMode = NonNullable<BindingWriteBody["image_stream_mode"]>;
type BindingStatus = NonNullable<BindingWriteBody["status"]>;
type InferEndpointProtocol = NonNullable<OperationQuery<"ai-infer-model-capability">["endpoint_protocol"]>;

function stripSchema<T>(value: T): Omit<T, "$schema"> {
  const { $schema: _schema, ...rest } = value as T & { $schema?: string };
  return rest as Omit<T, "$schema">;
}

function toPriceBook(value: PriceBookTransport): PriceBookDTO {
  if (value.status !== "active" && value.status !== "disabled") {
    throw new Error(`Unexpected price book status: ${value.status}`);
  }
  return {
    id: value.id,
    name: value.name,
    description: value.description,
    status: value.status,
    created_at: value.created_at,
    updated_at: value.updated_at
  };
}

function toPriceBooks(value: PriceBookPageTransport): PriceBooksOutputBody {
  return { items: value.items?.map(toPriceBook) ?? [], total: value.total };
}

function toPriceBookEntry(value: PriceBookEntryTransport): PriceBookEntryDTO {
  return {
    ...stripSchema(value),
    token_price_tiers: value.token_price_tiers?.map(stripSchema) ?? [],
    image_prices: value.image_prices?.map(stripSchema) ?? undefined,
    video_prices: value.video_prices?.map(stripSchema) ?? undefined
  };
}

function toPriceBookEntries(value: PriceBookEntriesPageTransport): PriceBookEntriesOutputBody {
  return { items: value.items?.map(toPriceBookEntry) ?? [], total: value.total };
}

function toPriceBookEntryBody(value: PriceBookEntryWriteRequest): OperationBody<"ai-upsert-price-book-entry"> {
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

function toProviderFamily(value: string | undefined): "openai_compatible" | "anthropic" | "gemini" | undefined {
  if (value === undefined || value === "openai_compatible" || value === "anthropic" || value === "gemini") return value;
  throw new Error(`Unexpected upstream provider family: ${value}`);
}

function toAccountBodyFields(value: AccountWriteRequest) {
  const providerFamily = toProviderFamily(value.default_provider_family);
  return {
    name: value.name,
    tenant_display_name: value.tenant_display_name,
    tenant_access_mode: value.tenant_access_mode,
    base_url: value.base_url,
    extra_headers: value.extra_headers,
    default_provider_family: providerFamily,
    concurrency_limit: value.concurrency_limit ?? undefined,
    price_book_id: value.price_book_id,
    tenant_multiplier: value.tenant_multiplier
  };
}

function toCreateAccountBody(value: AccountWriteRequest): OperationBody<"ai-create-upstream-account"> {
  if (typeof value.api_key !== "string") throw new Error("upstream API key is required");
  return { ...toAccountBodyFields(value), api_key: value.api_key };
}

function toUpdateAccountBody(value: AccountWriteRequest): OperationBody<"ai-update-upstream-account"> {
  return { ...toAccountBodyFields(value), api_key: value.api_key };
}

function toAccount(value: AccountTransport): AccountDTO {
  if (value.status !== "active" && value.status !== "invalid" && value.status !== "disabled") {
    throw new Error(`Unexpected upstream account status: ${value.status}`);
  }
  if (value.tenant_access_mode !== "public" && value.tenant_access_mode !== "restricted") {
    throw new Error(`Unexpected upstream tenant access mode: ${value.tenant_access_mode}`);
  }
  return {
    id: value.id,
    name: value.name,
    tenant_display_name: value.tenant_display_name,
    tenant_access_mode: value.tenant_access_mode,
    base_url: value.base_url,
    extra_headers: value.extra_headers,
    default_provider_family: value.default_provider_family,
    concurrency_limit: value.concurrency_limit,
    price_book_id: value.price_book_id,
    tenant_multiplier: value.tenant_multiplier,
    status: value.status,
    invalid_reason: value.invalid_reason,
    invalid_at: value.invalid_at,
    created_at: value.created_at,
    updated_at: value.updated_at
  };
}

function toAccounts(value: AccountsPageTransport): AccountsOutputBody {
  return { items: value.items?.map(toAccount) ?? [], total: value.total };
}

function toTransferBinding(value: AccountTransferBindingTransport): UpstreamAccountTransferBindingDTO {
  if (value.image_edit_transport !== undefined && value.image_edit_transport !== "application/json" && value.image_edit_transport !== "multipart/form-data") {
    throw new Error(`Unexpected image edit transport: ${value.image_edit_transport}`);
  }
  if (value.image_upstream_response_format !== undefined && value.image_upstream_response_format !== "url" && value.image_upstream_response_format !== "b64_json") {
    throw new Error(`Unexpected image upstream response format: ${value.image_upstream_response_format}`);
  }
  return {
    model_code: value.model_code,
    capability_type: value.capability_type,
    api_format: value.api_format,
    upstream_model_name: value.upstream_model_name,
    status: value.status,
    image_stream_mode: value.image_stream_mode,
    image_edit_transport: value.image_edit_transport,
    image_upstream_response_format: value.image_upstream_response_format,
    image_max_output_count: value.image_max_output_count,
    image_edit_max_output_count: value.image_edit_max_output_count
  };
}

function toTransferAccount(value: AccountTransferTransport): UpstreamAccountTransferAccountDTO {
  if (value.tenant_access_mode !== "public" && value.tenant_access_mode !== "restricted") {
    throw new Error(`Unexpected upstream tenant access mode: ${value.tenant_access_mode}`);
  }
  return {
    name: value.name,
    tenant_display_name: value.tenant_display_name,
    tenant_access_mode: value.tenant_access_mode,
    base_url: value.base_url,
    api_key: value.api_key,
    default_provider_family: value.default_provider_family,
    concurrency_limit: value.concurrency_limit,
    status: value.status,
    extra_headers: value.extra_headers,
    model_bindings: value.model_bindings?.map(toTransferBinding) ?? []
  };
}

function toAccountExport(value: AccountExportTransport): UpstreamAccountExportOutputBody {
  return {
    schema_version: value.schema_version,
    exported_at: value.exported_at,
    contains_plaintext_api_keys: value.contains_plaintext_api_keys,
    accounts: value.accounts?.map(toTransferAccount) ?? []
  };
}

function toImportAction(value: string): "create" | "skip" | "error" {
  if (value === "create" || value === "skip" || value === "error") return value;
  throw new Error(`Unexpected upstream import action: ${value}`);
}

function toAccountImportRequest(value: UpstreamAccountImportRequest): OperationBody<"ai-import-upstream-accounts"> {
  return {
    accounts: value.accounts.map((account) => ({
      name: account.name,
      tenant_display_name: account.tenant_display_name,
      tenant_access_mode: account.tenant_access_mode,
      base_url: account.base_url,
      api_key: account.api_key,
      default_provider_family: account.default_provider_family,
      concurrency_limit: account.concurrency_limit,
      status: account.status,
      extra_headers: account.extra_headers,
      model_bindings: account.model_bindings?.map((binding) => ({ ...binding })) ?? []
    })),
    default_price_book_id: value.default_price_book_id,
    default_tenant_multiplier: value.default_tenant_multiplier,
    duplicate_account_strategy: value.duplicate_account_strategy,
    duplicate_binding_strategy: value.duplicate_binding_strategy
  };
}

function toAccountImportPreview(value: AccountImportPreviewTransport): UpstreamAccountImportPreviewOutputBody {
  return {
    items: value.items?.map((item) => ({
      name: item.name,
      base_url: item.base_url,
      action: toImportAction(item.action),
      reason: item.reason,
      model_binding_count: item.model_binding_count,
      duplicate_model_bindings: item.duplicate_model_bindings,
      warnings: item.warnings ?? []
    })) ?? [],
    summary: value.summary
  };
}

function toAccountImport(value: AccountImportTransport): UpstreamAccountImportOutputBody {
  return {
    created_account_ids: value.created_account_ids ?? [],
    skipped_accounts: value.skipped_accounts?.map((item) => ({ ...item })) ?? [],
    created_model_bindings: value.created_model_bindings?.map((item) => ({ ...item })) ?? [],
    skipped_model_bindings: value.skipped_model_bindings?.map((item) => ({ ...item })) ?? [],
    summary: value.summary
  };
}

function toBindingApiFormat(value: string | undefined): BindingApiFormat | undefined {
  if (
    value === undefined ||
    value === "openai_chat" ||
    value === "openai_responses" ||
    value === "openai_embeddings" ||
    value === "openai_images" ||
    value === "anthropic_messages" ||
    value === "gemini_generate" ||
    value === "gemini_embeddings"
  ) return value;
  throw new Error(`Unexpected upstream API format: ${value}`);
}

function toBindingCapabilityType(value: string | undefined): BindingCapabilityType | undefined {
  if (
    value === undefined ||
    value === "chat" ||
    value === "image" ||
    value === "video" ||
    value === "embedding" ||
    value === "audio_tts" ||
    value === "audio_stt" ||
    value === "rerank"
  ) return value;
  throw new Error(`Unexpected upstream capability type: ${value}`);
}

function toBindingImageStreamMode(value: string | undefined): BindingImageStreamMode | undefined {
  if (value === undefined || value === "auto" || value === "force_stream" || value === "force_sync") return value;
  throw new Error(`Unexpected image stream mode: ${value}`);
}

function toBindingStatus(value: string | undefined): BindingStatus | undefined {
  if (value === undefined || value === "active" || value === "disabled") return value;
  throw new Error(`Unexpected upstream model binding status: ${value}`);
}

function toBindingResponseFormat(value: string | undefined): "" | "url" | "b64_json" | undefined {
  if (value === undefined || value === "" || value === "url" || value === "b64_json") return value;
  throw new Error(`Unexpected upstream image response format: ${value}`);
}

function toAccountModelBindingBody(value: UpstreamModelBindingWriteRequest): BindingWriteBody {
  return {
    model_code: value.model_code,
    capability_type: toBindingCapabilityType(value.capability_type),
    api_format: toBindingApiFormat(value.api_format),
    upstream_model_name: value.upstream_model_name,
    status: toBindingStatus(value.status),
    image_stream_mode: toBindingImageStreamMode(value.image_stream_mode),
    image_edit_transport: value.image_edit_transport,
    image_upstream_response_format: toBindingResponseFormat(value.image_upstream_response_format),
    image_max_output_count: value.image_max_output_count,
    image_edit_max_output_count: value.image_edit_max_output_count
  };
}

function toDiscoveredModels(value: DiscoveredModelsTransport): { items: DiscoveredUpstreamModelDTO[] } {
  return { items: value.items?.map((item) => stripSchema(item)) ?? [] };
}

function toUpstreamModelBinding(value: UpstreamModelBindingTransport): UpstreamModelBindingDTO {
  return stripSchema(value);
}

function toUpstreamModelBindings(value: UpstreamModelBindingsTransport): UpstreamModelBindingsOutputBody {
  return { items: value.items?.map(toUpstreamModelBinding) ?? [], total: value.total };
}

function toAccountTestBody(value: UpstreamAccountTestRequest): OperationBody<"ai-test-account-upstream"> {
  return {
    model_code: value.model_code,
    prompt: value.prompt,
    image_edit: value.image_edit,
    image: value.image ? { ...value.image } : undefined
  };
}

function toTestImageEditTransport(value: string | undefined): "application/json" | "multipart/form-data" | undefined {
  if (value === undefined || value === "application/json" || value === "multipart/form-data") return value;
  throw new Error(`Unexpected image edit transport: ${value}`);
}

function toTestImageResponseFormat(value: string | undefined): "url" | "b64_json" | undefined {
  if (value === undefined || value === "url" || value === "b64_json") return value;
  throw new Error(`Unexpected image upstream response format: ${value}`);
}

function toAccountTest(value: AccountTestTransport): UpstreamAccountTestResult {
  return {
    ok: value.ok,
    http_status: value.http_status,
    latency_ms: value.latency_ms,
    capability: value.capability,
    api_format: value.api_format,
    upstream_model: value.upstream_model,
    reply_text: value.reply_text,
    image_b64: value.image_b64,
    image_mime: value.image_mime,
    image_url: value.image_url,
    image_stream_mode: value.image_stream_mode,
    image_edit_transport: toTestImageEditTransport(value.image_edit_transport),
    image_upstream_response_format: toTestImageResponseFormat(value.image_upstream_response_format),
    actual_image_format: value.actual_image_format,
    prompt_tokens: value.prompt_tokens,
    output_tokens: value.output_tokens,
    total_tokens: value.total_tokens,
    error: value.error
  };
}

function toImportedModels(value: AccountModelImportTransport | PoolModelImportTransport): { created: string[]; skipped: string[] } {
  return { created: value.created ?? [], skipped: value.skipped ?? [] };
}

function toAccountModelImportBody(value: ImportUpstreamModelsRequest): OperationBody<"ai-import-account-upstream-models"> {
  return {
    models: value.models,
    api_format: toBindingApiFormat(value.api_format)
  };
}

function toPoolModelImportBody(value: { models: string[] }): OperationBody<"ai-import-pool-available-models"> {
  return { models: value.models };
}

function toPoolAvailableModels(value: PoolAvailableModelsTransport): PoolAvailableModelsDTO {
  return {
    pool_id: value.pool_id,
    fixed_provider_type: value.fixed_provider_type,
    models: value.models ?? [],
    source: value.source
  };
}

function toInferEndpointProtocol(value: string | undefined): InferEndpointProtocol | undefined {
  if (value === undefined || value === "openai_compatible" || value === "anthropic" || value === "gemini") return value;
  throw new Error(`Unexpected endpoint protocol: ${value}`);
}

function toModelCapability(value: OperationResponse<"ai-infer-model-capability">): ModelCapabilityInferResult {
  return {
    capability_type: value.capability_type,
    api_format: value.api_format,
    source: value.source
  };
}

function toDashboardSummary(value: DashboardSummaryTransport): DashboardSummaryDTO {
  return stripSchema(value);
}

function toDashboardTopModels(value: DashboardTopModelsTransport): DashboardTopModelsOutputBody {
  return { items: value.items?.map((item) => stripSchema(item)) ?? [], total: value.total };
}

function toDashboardTopTenants(value: DashboardTopTenantsTransport): DashboardTopTenantsOutputBody {
  return {
    items: value.items?.map((item) => stripSchema(item)) ?? [],
    total: value.total,
    included: value.included
  };
}

function toDashboardRecentErrors(value: DashboardRecentErrorsTransport): DashboardRecentErrorsOutputBody {
  return { items: value.items?.map((item) => stripSchema(item)) ?? [], total: value.total };
}

function toAuditLogs(value: AuditLogsTransport): AuditLogsOutputBody {
  return { items: value.items?.map((item) => stripSchema(item)) ?? [], total: value.total };
}

function toRuntimeLimitPolicyStatus(value: unknown): LimitPolicyStatus | undefined {
  if (value === undefined || value === "active" || value === "disabled") return value;
  throw new Error(`Unexpected runtime limit policy status: ${String(value)}`);
}

function toLimitPolicyScopeType(value: unknown): LimitPolicyScopeType {
  if (value === "tenant" || value === "user" || value === "api_key") return value;
  throw new Error(`Unexpected runtime limit policy scope: ${String(value)}`);
}

function toLimitPolicyWriteBody(value: Record<string, unknown>): LimitPolicyWriteBody {
  if (typeof value.scope_id !== "string") throw new Error("runtime limit policy scope_id is required");
  const concurrencyLimit = value.concurrency_limit;
  if (concurrencyLimit !== undefined && concurrencyLimit !== null && (typeof concurrencyLimit !== "number" || !Number.isFinite(concurrencyLimit))) {
    throw new Error("runtime limit policy concurrency_limit must be a finite number");
  }
  if (value.created_by !== undefined && typeof value.created_by !== "string") {
    throw new Error("runtime limit policy created_by must be a string");
  }
  return {
    scope_id: value.scope_id,
    scope_type: toLimitPolicyScopeType(value.scope_type),
    concurrency_limit: concurrencyLimit === null ? undefined : concurrencyLimit,
    created_by: value.created_by as string | undefined,
    status: toRuntimeLimitPolicyStatus(value.status)
  };
}

function toRuntimeLimitPolicy(value: RuntimeLimitPolicyTransport): RuntimeLimitPolicyDTO {
  return stripSchema(value);
}

function toRuntimeLimitPolicies(value: RuntimeLimitPoliciesTransport): RuntimeLimitPoliciesOutputBody {
  return {
    items: value.items?.map(toRuntimeLimitPolicy) ?? [],
    total: value.total,
    included: value.included
  };
}

function toTenantUpstreamResourceKind(value: string): "direct_upstream" | "oauth_pool" {
  if (value === "direct_upstream" || value === "oauth_pool") return value;
  throw new Error(`Unexpected tenant upstream resource kind: ${value}`);
}

function toTenantUpstreamAccess(value: TenantUpstreamAccessTransport): TenantUpstreamAccessDTO {
  if (value.access_mode !== "public" && value.access_mode !== "restricted") {
    throw new Error(`Unexpected tenant upstream access mode: ${value.access_mode}`);
  }
  return {
    ...stripSchema(value),
    resource_kind: toTenantUpstreamResourceKind(value.resource_kind)
  };
}

function toTenantUpstreamAccessPage(value: TenantUpstreamAccessPageTransport): TenantUpstreamAccessOutputBody {
  return { items: value.items?.map(toTenantUpstreamAccess) ?? [], total: value.total };
}

function toTenantUpstreamAccessBody(policies: TenantUpstreamPolicyRef[]): TenantUpstreamAccessWriteBody {
  return {
    policies: policies.map((policy) => ({
      resource_kind: toTenantUpstreamResourceKind(policy.resource_kind),
      resource_id: policy.resource_id,
      access_granted: policy.access_granted,
      tenant_multiplier_override: policy.tenant_multiplier_override
    }))
  };
}

export const aiAdminApi = {
  // ---- 上游账号（ai_upstream_accounts）----
  listUpstreamAccounts() {
    return typedRequest<"ai-list-upstream-accounts">({
      method: "GET",
      path: "/api/v1/upstream-accounts",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toAccounts);
  },
  createUpstreamAccount(body: AccountWriteRequest) {
    return typedRequest<"ai-create-upstream-account">({
      method: "POST",
      path: "/api/v1/upstream-accounts",
      headers: apiHeaders,
      body: toCreateAccountBody(body),
      baseUrl: apiBaseUrl
    }).then(toAccount);
  },
  updateUpstreamAccount(accountId: string, body: AccountWriteRequest) {
    return typedRequest<"ai-update-upstream-account">({
      method: "PATCH",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}`,
      pathParams: { accountID: accountId },
      headers: apiHeaders,
      body: toUpdateAccountBody(body),
      baseUrl: apiBaseUrl
    }).then(toAccount);
  },
  updateUpstreamAccountStatus(accountId: string, status: "active" | "disabled") {
    return typedRequest<"ai-update-upstream-account-status">({
      method: "PATCH",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/status`,
      pathParams: { accountID: accountId },
      headers: apiHeaders,
      body: { status },
      baseUrl: apiBaseUrl
    }).then(toAccount);
  },
  deleteUpstreamAccount(accountId: string) {
    return typedRequest<"ai-delete-upstream-account">({
      method: "DELETE",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}`,
      pathParams: { accountID: accountId },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then((value) => ({ deleted: value.deleted }));
  },
  exportUpstreamAccounts(body: UpstreamAccountExportRequest) {
    return typedRequest<"ai-export-upstream-accounts">({
      method: "POST",
      path: "/api/v1/upstream-accounts/export",
      headers: apiHeaders,
      body: { account_ids: body.account_ids, include_model_bindings: body.include_model_bindings ?? false },
      baseUrl: apiBaseUrl
    }).then(toAccountExport);
  },
  previewImportUpstreamAccounts(body: UpstreamAccountImportRequest) {
    return typedRequest<"ai-preview-import-upstream-accounts">({
      method: "POST",
      path: "/api/v1/upstream-accounts/import/preview",
      headers: apiHeaders,
      body: toAccountImportRequest(body),
      baseUrl: apiBaseUrl
    }).then(toAccountImportPreview);
  },
  importUpstreamAccounts(body: UpstreamAccountImportRequest) {
    return typedRequest<"ai-import-upstream-accounts">({
      method: "POST",
      path: "/api/v1/upstream-accounts/import",
      headers: apiHeaders,
      body: toAccountImportRequest(body),
      baseUrl: apiBaseUrl
    }).then(toAccountImport);
  },
  // ---- 上游模型发现 / 导入（账号维度）----
  fetchAccountUpstreamModels(accountId: string) {
    return typedRequest<"ai-fetch-account-upstream-models">({
      method: "GET",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/upstream-models`,
      pathParams: { accountID: accountId },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toDiscoveredModels);
  },
  listAccountModelBindings(accountId: string) {
    return typedRequest<"ai-list-account-model-bindings">({
      method: "GET",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/model-bindings`,
      pathParams: { accountID: accountId },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toUpstreamModelBindings);
  },
  testUpstreamAccount(accountId: string, body: UpstreamAccountTestRequest) {
    return typedRequest<"ai-test-account-upstream">({
      method: "POST",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/test`,
      pathParams: { accountID: accountId },
      headers: apiHeaders,
      body: toAccountTestBody(body),
      baseUrl: apiBaseUrl
    }).then(toAccountTest);
  },
  createAccountModelBinding(accountId: string, body: UpstreamModelBindingWriteRequest) {
    return typedRequest<"ai-create-account-model-binding">({
      method: "POST",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/model-bindings`,
      pathParams: { accountID: accountId },
      headers: apiHeaders,
      body: toAccountModelBindingBody(body),
      baseUrl: apiBaseUrl
    }).then(toUpstreamModelBinding);
  },
  updateAccountModelBinding(accountId: string, bindingId: string, body: UpstreamModelBindingWriteRequest) {
    return typedRequest<"ai-update-account-model-binding">({
      method: "PATCH",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/model-bindings/${encodeURIComponent(bindingId)}`,
      pathParams: { accountID: accountId, bindingID: bindingId },
      headers: apiHeaders,
      body: toAccountModelBindingBody(body),
      baseUrl: apiBaseUrl
    }).then(toUpstreamModelBinding);
  },
  deleteAccountModelBinding(accountId: string, bindingId: string) {
    return typedRequest<"ai-delete-account-model-binding">({
      method: "DELETE",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/model-bindings/${encodeURIComponent(bindingId)}`,
      pathParams: { accountID: accountId, bindingID: bindingId },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then((value) => ({ deleted: value.deleted }));
  },
  importAccountUpstreamModels(accountId: string, body: ImportUpstreamModelsRequest) {
    return typedRequest<"ai-import-account-upstream-models">({
      method: "POST",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/import-upstream-models`,
      pathParams: { accountID: accountId },
      headers: apiHeaders,
      body: toAccountModelImportBody(body),
      baseUrl: apiBaseUrl
    }).then(toImportedModels);
  },
  inferModelCapability(modelCode: string, endpointProtocol?: string) {
    return typedRequest<"ai-infer-model-capability">({
      method: "GET",
      path: "/api/v1/model-capability/infer",
      headers: apiHeaders,
      query: { model_code: modelCode, endpoint_protocol: toInferEndpointProtocol(endpointProtocol || undefined) },
      baseUrl: apiBaseUrl
    }).then(toModelCapability);
  },

  // ---- 价格表 ----
  listPriceBooks() {
    return typedRequest<"ai-list-price-books">({
      method: "GET",
      path: "/api/v1/price-books",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toPriceBooks);
  },
  createPriceBook(body: { name: string; description?: string }) {
    return typedRequest<"ai-create-price-book">({
      method: "POST",
      path: "/api/v1/price-books",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toPriceBook);
  },
  getPriceBook(bookId: string) {
    return typedRequest<"ai-get-price-book">({
      method: "GET",
      path: `/api/v1/price-books/${encodeURIComponent(bookId)}`,
      pathParams: { bookID: bookId },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toPriceBook);
  },
  updatePriceBook(bookId: string, body: PriceBookWriteRequest) {
    return typedRequest<"ai-update-price-book">({
      method: "PATCH",
      path: `/api/v1/price-books/${encodeURIComponent(bookId)}`,
      pathParams: { bookID: bookId },
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toPriceBook);
  },
  deletePriceBook(bookId: string) {
    return typedRequest<"ai-delete-price-book">({
      method: "DELETE",
      path: `/api/v1/price-books/${encodeURIComponent(bookId)}`,
      pathParams: { bookID: bookId },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then((value) => ({ deleted: value.deleted }));
  },
  listPriceBookEntries(bookId: string) {
    return typedRequest<"ai-list-price-book-entries">({
      method: "GET",
      path: `/api/v1/price-books/${encodeURIComponent(bookId)}/entries`,
      pathParams: { bookID: bookId },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toPriceBookEntries);
  },
  upsertPriceBookEntry(bookId: string, modelCode: string, body: PriceBookEntryWriteRequest) {
    return typedRequest<"ai-upsert-price-book-entry">({
      method: "PUT",
      path: `/api/v1/price-books/${encodeURIComponent(bookId)}/entries/${encodeURIComponent(modelCode)}`,
      pathParams: { bookID: bookId, modelCode },
      headers: apiHeaders,
      body: toPriceBookEntryBody(body),
      baseUrl: apiBaseUrl
    }).then(toPriceBookEntry);
  },
  deletePriceBookEntry(bookId: string, modelCode: string) {
    return typedRequest<"ai-delete-price-book-entry">({
      method: "DELETE",
      path: `/api/v1/price-books/${encodeURIComponent(bookId)}/entries/${encodeURIComponent(modelCode)}`,
      pathParams: { bookID: bookId, modelCode },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then((value) => ({ deleted: value.deleted }));
  },
  searchLiteLLMModels(q: string, limit = 50) {
    return typedRequest<"ai-search-litellm-price-models">({
      method: "GET",
      path: "/api/v1/price-books/litellm/models",
      query: { q, limit },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then((response: LiteLLMModelsOutputBody) => ({
      ...response,
      items: (response.items ?? []).map((model: LiteLLMModelInfo) => ({
        ...model,
        token_price_tiers: model.token_price_tiers ?? []
      }))
    }));
  },
  syncCommonModels(bookId: string) {
    return typedRequest<"ai-sync-common-price-book-models">({
      method: "POST",
      path: `/api/v1/price-books/${encodeURIComponent(bookId)}/sync-common`,
      pathParams: { bookID: bookId },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then((value) => ({ synced: value.synced, missing: value.missing ?? [] }));
  },

  // ---- Dashboard ----
  getDashboardSummary(params: OperationQuery<"ai-get-dashboard-summary"> = {}) {
    return typedRequest<"ai-get-dashboard-summary">({
      method: "GET",
      path: "/api/v1/dashboard/summary",
      query: params,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toDashboardSummary);
  },
  listDashboardTopModels(params: OperationQuery<"ai-list-dashboard-top-models"> = {}) {
    return typedRequest<"ai-list-dashboard-top-models">({
      method: "GET",
      path: "/api/v1/dashboard/top-models",
      query: params,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toDashboardTopModels);
  },
  listDashboardTopTenants(params: OperationQuery<"ai-list-dashboard-top-tenants"> = {}) {
    return typedRequest<"ai-list-dashboard-top-tenants">({
      method: "GET",
      path: "/api/v1/dashboard/top-tenants",
      query: params,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toDashboardTopTenants);
  },
  listDashboardRecentErrors(params: OperationQuery<"ai-list-dashboard-recent-errors"> = {}) {
    return typedRequest<"ai-list-dashboard-recent-errors">({
      method: "GET",
      path: "/api/v1/dashboard/recent-errors",
      query: params,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toDashboardRecentErrors);
  },
  // ---- Runtime limit policies ----
  listRuntimeLimitPolicies() {
    return typedRequest<"ai-list-runtime-limit-policies">({
      method: "GET",
      path: "/api/v1/limit-policies",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toRuntimeLimitPolicies);
  },
  createRuntimeLimitPolicy(body: Record<string, unknown>) {
    return typedRequest<"ai-create-runtime-limit-policy">({
      method: "POST",
      path: "/api/v1/limit-policies",
      headers: apiHeaders,
      body: toLimitPolicyWriteBody(body),
      baseUrl: apiBaseUrl
    }).then(toRuntimeLimitPolicy);
  },
  updateRuntimeLimitPolicy(policyId: string, body: Record<string, unknown>) {
    return typedRequest<"ai-update-runtime-limit-policy">({
      method: "PATCH",
      path: `/api/v1/limit-policies/${encodeURIComponent(policyId)}`,
      pathParams: { policyID: policyId },
      headers: apiHeaders,
      body: toLimitPolicyWriteBody(body),
      baseUrl: apiBaseUrl
    }).then(toRuntimeLimitPolicy);
  },
  updateRuntimeLimitPolicyStatus(policyId: string, status: string) {
    return typedRequest<"ai-update-runtime-limit-policy-status">({
      method: "PATCH",
      path: `/api/v1/limit-policies/${encodeURIComponent(policyId)}/status`,
      pathParams: { policyID: policyId },
      headers: apiHeaders,
      body: { status: toRuntimeLimitPolicyStatus(status) as LimitPolicyStatus },
      baseUrl: apiBaseUrl
    }).then(toRuntimeLimitPolicy);
  },
  listTenantUpstreamAccess(tenantId: string) {
    return typedRequest<"ai-list-tenant-upstream-access">({
      method: "GET",
      path: `/api/v1/tenants/${encodeURIComponent(tenantId)}/upstream-access`,
      pathParams: { tenantID: tenantId },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toTenantUpstreamAccessPage);
  },
  replaceTenantUpstreamAccess(tenantId: string, policies: TenantUpstreamPolicyRef[]) {
    return typedRequest<"ai-replace-tenant-upstream-access">({
      method: "PUT",
      path: `/api/v1/tenants/${encodeURIComponent(tenantId)}/upstream-access`,
      pathParams: { tenantID: tenantId },
      headers: apiHeaders,
      body: toTenantUpstreamAccessBody(policies),
      baseUrl: apiBaseUrl
    }).then((value) => ({ updated: value.updated }));
  },

  // ---- Audit logs ----
  listGatewayAuditLogs(params: OperationQuery<"ai-list-audit-logs"> = {}) {
    return typedRequest<"ai-list-audit-logs">({
      method: "GET",
      path: "/api/v1/audit-logs",
      query: params,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toAuditLogs);
  },

  // ---- 风控中心（内容安全审核）----
  getRiskControlConfig() {
    return request()<RiskControlConfigDTO>({
      method: "GET",
      path: "/api/v1/risk-control/config",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  updateRiskControlConfig(body: RiskControlConfigWriteRequest) {
    return request()<RiskControlConfigDTO>({
      method: "PUT",
      path: "/api/v1/risk-control/config",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  testRiskControlModeration(text: string) {
    return request()<RiskControlTestResultDTO>({
      method: "POST",
      path: "/api/v1/risk-control/test",
      headers: apiHeaders,
      body: { text },
      baseUrl: apiBaseUrl
    });
  },
  listRiskControlLogs(params: Record<string, string | number | undefined> = {}) {
    return request()<RiskControlLogsOutputBody>({
      method: "GET",
      path: "/api/v1/risk-control/logs",
      query: params,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  listRiskControlEvents(params: Record<string, string | number | undefined> = {}) {
    return request()<RiskEventsOutputBody>({
      method: "GET",
      path: "/api/v1/risk-control/events",
      query: params,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  resolveRiskControlEvent(eventId: string, body: { status: string; note?: string }) {
    return request()<RiskEventDTO>({
      method: "POST",
      path: `/api/v1/risk-control/events/${encodeURIComponent(eventId)}/resolve`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },

  // ---- System & 路由策略 ----
  getSystemStatus() {
    return request()<SystemStatusDTO>({
      method: "GET",
      path: "/api/v1/system/status",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  getRouteWeights(scope = "global") {
    return request()<RouteWeightsOutputBody>({
      method: "GET",
      path: `/api/v1/route-weights/${encodeURIComponent(scope)}`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  putRouteWeights(scope: string, body: { cost: number; latency: number; load: number; health: number }) {
    return request()<RouteWeightsOutputBody>({
      method: "PUT",
      path: `/api/v1/route-weights/${encodeURIComponent(scope)}`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },

  // ---- Credential pools ----
  listCredentialPools() {
    return request()<CredentialPoolsOutputBody>({
      method: "GET",
      path: "/api/v1/credential-pools",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  createCredentialPool(body: CredentialPoolWriteRequest) {
    return request()<CredentialPoolDTO>({
      method: "POST",
      path: "/api/v1/credential-pools",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  patchCredentialPool(poolId: string, body: CredentialPoolWriteRequest) {
    return request()<CredentialPoolDTO>({
      method: "PATCH",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  updateCredentialPoolStatus(poolId: string, status: "active" | "disabled") {
    return request()<CredentialPoolDTO>({
      method: "PATCH",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/status`,
      headers: apiHeaders,
      body: { status },
      baseUrl: apiBaseUrl
    });
  },
  deleteCredentialPool(poolId: string) {
    return request()<{ deleted: boolean }>({
      method: "DELETE",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  listPoolCredentials(poolId: string) {
    return request()<PoolCredentialsOutputBody>({
      method: "GET",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/credentials`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  createPoolCredential(poolId: string, body: PoolCredentialWriteRequest) {
    return request()<PoolCredentialDTO>({
      method: "POST",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/credentials`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  patchPoolCredential(poolId: string, credId: string, body: PoolCredentialPatchRequest) {
    return request()<PoolCredentialDTO>({
      method: "PATCH",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/credentials/${encodeURIComponent(credId)}`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  deletePoolCredential(poolId: string, credId: string) {
    return request()<{ deleted: boolean }>({
      method: "DELETE",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/credentials/${encodeURIComponent(credId)}`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  refreshPoolCredential(poolId: string, credId: string) {
    return request()<PoolCredentialDTO>({
      method: "POST",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/credentials/${encodeURIComponent(credId)}/refresh`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  getPoolAvailableModels(poolId: string) {
    return typedRequest<"ai-get-pool-available-models">({
      method: "GET",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/available-models`,
      pathParams: { poolID: poolId },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toPoolAvailableModels);
  },
  listPoolModelBindings(poolId: string) {
    return typedRequest<"ai-list-pool-model-bindings">({
      method: "GET",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/model-bindings`,
      pathParams: { poolID: poolId },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toUpstreamModelBindings);
  },
  createPoolModelBinding(poolId: string, body: UpstreamModelBindingWriteRequest) {
    return typedRequest<"ai-create-pool-model-binding">({
      method: "POST",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/model-bindings`,
      pathParams: { poolID: poolId },
      headers: apiHeaders,
      body: toAccountModelBindingBody(body),
      baseUrl: apiBaseUrl
    }).then(toUpstreamModelBinding);
  },
  updatePoolModelBinding(poolId: string, bindingId: string, body: UpstreamModelBindingWriteRequest) {
    return typedRequest<"ai-update-pool-model-binding">({
      method: "PATCH",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/model-bindings/${encodeURIComponent(bindingId)}`,
      pathParams: { poolID: poolId, bindingID: bindingId },
      headers: apiHeaders,
      body: toAccountModelBindingBody(body),
      baseUrl: apiBaseUrl
    }).then(toUpstreamModelBinding);
  },
  deletePoolModelBinding(poolId: string, bindingId: string) {
    return typedRequest<"ai-delete-pool-model-binding">({
      method: "DELETE",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/model-bindings/${encodeURIComponent(bindingId)}`,
      pathParams: { poolID: poolId, bindingID: bindingId },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then((value) => ({ deleted: value.deleted }));
  },
  importPoolAvailableModels(poolId: string, body: { models: string[] }) {
    return typedRequest<"ai-import-pool-available-models">({
      method: "POST",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/import-available-models`,
      pathParams: { poolID: poolId },
      headers: apiHeaders,
      body: toPoolModelImportBody(body),
      baseUrl: apiBaseUrl
    }).then(toImportedModels);
  },
  getOAuthPoolHealth() {
    return request()<OAuthPoolHealthOutputBody>({
      method: "GET",
      path: "/api/v1/oauth-pool-health",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
};
