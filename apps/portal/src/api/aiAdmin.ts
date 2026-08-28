import { authenticatedRequest, apiHeaders, apiBaseUrl } from "./request";
import {
  createTypedOperationRequest,
  type OperationBody,
  type OperationResponse
} from ".";
import type { components } from "./generated/dai";
import type {
  AccountDTO,
  AccountsOutputBody,
  AccountWriteRequest,
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

type AiWorkbenchWindowQuery = {
  date_from?: string;
  date_to?: string;
};

export const aiAdminApi = {
  // ---- 上游账号（ai_upstream_accounts）----
  listUpstreamAccounts() {
    return request()<AccountsOutputBody>({
      method: "GET",
      path: "/api/v1/upstream-accounts",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  createUpstreamAccount(body: AccountWriteRequest) {
    return request()<AccountDTO>({
      method: "POST",
      path: "/api/v1/upstream-accounts",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  updateUpstreamAccount(accountId: string, body: AccountWriteRequest) {
    return request()<AccountDTO>({
      method: "PATCH",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  updateUpstreamAccountStatus(accountId: string, status: "active" | "disabled") {
    return request()<AccountDTO>({
      method: "PATCH",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/status`,
      headers: apiHeaders,
      body: { status },
      baseUrl: apiBaseUrl
    });
  },
  deleteUpstreamAccount(accountId: string) {
    return request()<{ deleted: boolean }>({
      method: "DELETE",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  exportUpstreamAccounts(body: UpstreamAccountExportRequest) {
    return request()<UpstreamAccountExportOutputBody>({
      method: "POST",
      path: "/api/v1/upstream-accounts/export",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  previewImportUpstreamAccounts(body: UpstreamAccountImportRequest) {
    return request()<UpstreamAccountImportPreviewOutputBody>({
      method: "POST",
      path: "/api/v1/upstream-accounts/import/preview",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  importUpstreamAccounts(body: UpstreamAccountImportRequest) {
    return request()<UpstreamAccountImportOutputBody>({
      method: "POST",
      path: "/api/v1/upstream-accounts/import",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  // ---- 上游模型发现 / 导入（账号维度）----
  fetchAccountUpstreamModels(accountId: string) {
    return request()<{ items: DiscoveredUpstreamModelDTO[] }>({
      method: "GET",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/upstream-models`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  listAccountModelBindings(accountId: string) {
    return request()<UpstreamModelBindingsOutputBody>({
      method: "GET",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/model-bindings`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  testUpstreamAccount(accountId: string, body: UpstreamAccountTestRequest) {
    return request()<UpstreamAccountTestResult>({
      method: "POST",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/test`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  createAccountModelBinding(accountId: string, body: UpstreamModelBindingWriteRequest) {
    return request()<UpstreamModelBindingDTO>({
      method: "POST",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/model-bindings`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  updateAccountModelBinding(accountId: string, bindingId: string, body: UpstreamModelBindingWriteRequest) {
    return request()<UpstreamModelBindingDTO>({
      method: "PATCH",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/model-bindings/${encodeURIComponent(bindingId)}`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  deleteAccountModelBinding(accountId: string, bindingId: string) {
    return request()<{ deleted: boolean }>({
      method: "DELETE",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/model-bindings/${encodeURIComponent(bindingId)}`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  importAccountUpstreamModels(accountId: string, body: ImportUpstreamModelsRequest) {
    return request()<{ created: string[]; skipped: string[] }>({
      method: "POST",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/import-upstream-models`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  inferModelCapability(modelCode: string, endpointProtocol?: string) {
    return request()<ModelCapabilityInferResult>({
      method: "GET",
      path: "/api/v1/model-capability/infer",
      headers: apiHeaders,
      query: { model_code: modelCode, endpoint_protocol: endpointProtocol || undefined },
      baseUrl: apiBaseUrl
    });
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
  getDashboardSummary(params: AiWorkbenchWindowQuery = {}) {
    return request()<DashboardSummaryDTO>({
      method: "GET",
      path: "/api/v1/dashboard/summary",
      query: params,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  listDashboardTopModels(params: AiWorkbenchWindowQuery & { limit?: number } = {}) {
    return request()<DashboardTopModelsOutputBody>({
      method: "GET",
      path: "/api/v1/dashboard/top-models",
      query: params,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  listDashboardTopTenants(params: AiWorkbenchWindowQuery & { limit?: number } = {}) {
    return request()<DashboardTopTenantsOutputBody>({
      method: "GET",
      path: "/api/v1/dashboard/top-tenants",
      query: params,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  listDashboardRecentErrors(params: AiWorkbenchWindowQuery & { limit?: number } = {}) {
    return request()<DashboardRecentErrorsOutputBody>({
      method: "GET",
      path: "/api/v1/dashboard/recent-errors",
      query: params,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  // ---- Runtime limit policies ----
  listRuntimeLimitPolicies(params: Record<string, string | number | undefined> = {}) {
    return request()<RuntimeLimitPoliciesOutputBody>({
      method: "GET",
      path: "/api/v1/limit-policies",
      query: params,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  createRuntimeLimitPolicy(body: Record<string, unknown>) {
    return request()<RuntimeLimitPolicyDTO>({
      method: "POST",
      path: "/api/v1/limit-policies",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  updateRuntimeLimitPolicy(policyId: string, body: Record<string, unknown>) {
    return request()<RuntimeLimitPolicyDTO>({
      method: "PATCH",
      path: `/api/v1/limit-policies/${encodeURIComponent(policyId)}`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  updateRuntimeLimitPolicyStatus(policyId: string, status: string) {
    return request()<RuntimeLimitPolicyDTO>({
      method: "PATCH",
      path: `/api/v1/limit-policies/${encodeURIComponent(policyId)}/status`,
      headers: apiHeaders,
      body: { status },
      baseUrl: apiBaseUrl
    });
  },
  listTenantUpstreamAccess(tenantId: string) {
    return request()<TenantUpstreamAccessOutputBody>({
      method: "GET",
      path: `/api/v1/tenants/${encodeURIComponent(tenantId)}/upstream-access`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  replaceTenantUpstreamAccess(tenantId: string, policies: TenantUpstreamPolicyRef[]) {
    return request()<{ updated: boolean }>({
      method: "PUT",
      path: `/api/v1/tenants/${encodeURIComponent(tenantId)}/upstream-access`,
      headers: apiHeaders,
      body: { policies },
      baseUrl: apiBaseUrl
    });
  },

  // ---- Audit logs ----
  listGatewayAuditLogs(params: Record<string, string | number | undefined> = {}) {
    return request()<AuditLogsOutputBody>({
      method: "GET",
      path: "/api/v1/audit-logs",
      query: params,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
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
    return request()<PoolAvailableModelsDTO>({
      method: "GET",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/available-models`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  listPoolModelBindings(poolId: string) {
    return request()<UpstreamModelBindingsOutputBody>({
      method: "GET",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/model-bindings`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  createPoolModelBinding(poolId: string, body: UpstreamModelBindingWriteRequest) {
    return request()<UpstreamModelBindingDTO>({
      method: "POST",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/model-bindings`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  updatePoolModelBinding(poolId: string, bindingId: string, body: UpstreamModelBindingWriteRequest) {
    return request()<UpstreamModelBindingDTO>({
      method: "PATCH",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/model-bindings/${encodeURIComponent(bindingId)}`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  deletePoolModelBinding(poolId: string, bindingId: string) {
    return request()<{ deleted: boolean }>({
      method: "DELETE",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/model-bindings/${encodeURIComponent(bindingId)}`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  importPoolAvailableModels(poolId: string, body: { models: string[] }) {
    return request()<{ created: string[]; skipped: string[] }>({
      method: "POST",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/import-available-models`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
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
