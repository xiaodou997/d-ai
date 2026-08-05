import { authenticatedRequest, portalHeadersFor, serviceBaseUrl } from "./request";
import type {
  AccountDTO,
  AccountsOutputBody,
  AccountWriteRequest,
  UpstreamAccountExportOutputBody,
  UpstreamAccountExportRequest,
  UpstreamAccountImportOutputBody,
  UpstreamAccountImportPreviewOutputBody,
  UpstreamAccountImportRequest,
  CreditsPerUSDOutputBody,
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
  RiskEventsOutputBody
} from "./types/ai";

function request(service: "urm" | "ai" = "urm") {
  return authenticatedRequest(service);
}

type AiWorkbenchWindowQuery = {
  date_from?: string;
  date_to?: string;
};

export const aiAdminApi = {
  // ---- 上游账号（ai_upstream_accounts）----
  listUpstreamAccounts() {
    return request("ai")<AccountsOutputBody>({
      method: "GET",
      path: "/api/v1/upstream-accounts",
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  createUpstreamAccount(body: AccountWriteRequest) {
    return request("ai")<AccountDTO>({
      method: "POST",
      path: "/api/v1/upstream-accounts",
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  updateUpstreamAccount(accountId: string, body: AccountWriteRequest) {
    return request("ai")<AccountDTO>({
      method: "PATCH",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}`,
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  updateUpstreamAccountStatus(accountId: string, status: "active" | "disabled") {
    return request("ai")<AccountDTO>({
      method: "PATCH",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/status`,
      headers: portalHeadersFor("ai"),
      body: { status },
      baseUrl: serviceBaseUrl("ai")
    });
  },
  deleteUpstreamAccount(accountId: string) {
    return request("ai")<{ deleted: boolean }>({
      method: "DELETE",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  exportUpstreamAccounts(body: UpstreamAccountExportRequest) {
    return request("ai")<UpstreamAccountExportOutputBody>({
      method: "POST",
      path: "/api/v1/upstream-accounts/export",
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  previewImportUpstreamAccounts(body: UpstreamAccountImportRequest) {
    return request("ai")<UpstreamAccountImportPreviewOutputBody>({
      method: "POST",
      path: "/api/v1/upstream-accounts/import/preview",
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  importUpstreamAccounts(body: UpstreamAccountImportRequest) {
    return request("ai")<UpstreamAccountImportOutputBody>({
      method: "POST",
      path: "/api/v1/upstream-accounts/import",
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  // ---- 上游模型发现 / 导入（账号维度）----
  fetchAccountUpstreamModels(accountId: string) {
    return request("ai")<{ items: DiscoveredUpstreamModelDTO[] }>({
      method: "GET",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/upstream-models`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  listAccountModelBindings(accountId: string) {
    return request("ai")<UpstreamModelBindingsOutputBody>({
      method: "GET",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/model-bindings`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  testUpstreamAccount(accountId: string, body: UpstreamAccountTestRequest) {
    return request("ai")<UpstreamAccountTestResult>({
      method: "POST",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/test`,
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  createAccountModelBinding(accountId: string, body: UpstreamModelBindingWriteRequest) {
    return request("ai")<UpstreamModelBindingDTO>({
      method: "POST",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/model-bindings`,
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  updateAccountModelBinding(accountId: string, bindingId: string, body: UpstreamModelBindingWriteRequest) {
    return request("ai")<UpstreamModelBindingDTO>({
      method: "PATCH",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/model-bindings/${encodeURIComponent(bindingId)}`,
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  deleteAccountModelBinding(accountId: string, bindingId: string) {
    return request("ai")<{ deleted: boolean }>({
      method: "DELETE",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/model-bindings/${encodeURIComponent(bindingId)}`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  importAccountUpstreamModels(accountId: string, body: ImportUpstreamModelsRequest) {
    return request("ai")<{ created: string[]; skipped: string[] }>({
      method: "POST",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/import-upstream-models`,
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  inferModelCapability(modelCode: string, endpointProtocol?: string) {
    return request("ai")<ModelCapabilityInferResult>({
      method: "GET",
      path: "/api/v1/model-capability/infer",
      headers: portalHeadersFor("ai"),
      query: { model_code: modelCode, endpoint_protocol: endpointProtocol || undefined },
      baseUrl: serviceBaseUrl("ai")
    });
  },

  // ---- 价格表 ----
  listPriceBooks() {
    return request("ai")<PriceBooksOutputBody>({
      method: "GET",
      path: "/api/v1/price-books",
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  createPriceBook(body: { name: string; description?: string }) {
    return request("ai")<PriceBookDTO>({
      method: "POST",
      path: "/api/v1/price-books",
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  getPriceBook(bookId: string) {
    return request("ai")<PriceBookDTO>({
      method: "GET",
      path: `/api/v1/price-books/${encodeURIComponent(bookId)}`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  updatePriceBook(bookId: string, body: PriceBookWriteRequest) {
    return request("ai")<PriceBookDTO>({
      method: "PATCH",
      path: `/api/v1/price-books/${encodeURIComponent(bookId)}`,
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  deletePriceBook(bookId: string) {
    return request("ai")<{ deleted: boolean }>({
      method: "DELETE",
      path: `/api/v1/price-books/${encodeURIComponent(bookId)}`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  listPriceBookEntries(bookId: string) {
    return request("ai")<PriceBookEntriesOutputBody>({
      method: "GET",
      path: `/api/v1/price-books/${encodeURIComponent(bookId)}/entries`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  upsertPriceBookEntry(bookId: string, modelCode: string, body: PriceBookEntryWriteRequest) {
    return request("ai")<PriceBookEntryDTO>({
      method: "PUT",
      path: `/api/v1/price-books/${encodeURIComponent(bookId)}/entries/${encodeURIComponent(modelCode)}`,
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  deletePriceBookEntry(bookId: string, modelCode: string) {
    return request("ai")<{ deleted: boolean }>({
      method: "DELETE",
      path: `/api/v1/price-books/${encodeURIComponent(bookId)}/entries/${encodeURIComponent(modelCode)}`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  searchLiteLLMModels(q: string, limit = 50) {
    return request("ai")<{ items: any[]; total: number }>({
      method: "GET",
      path: "/api/v1/price-books/litellm/models",
      query: { q, limit },
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  syncCommonModels(bookId: string) {
    return request("ai")<{ synced: number; missing: string[] }>({
      method: "POST",
      path: `/api/v1/price-books/${encodeURIComponent(bookId)}/sync-common`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },

  // ---- 全局汇率 ----
  getCreditsPerUSD() {
    return request("ai")<CreditsPerUSDOutputBody>({
      method: "GET",
      path: "/api/v1/pricing/credits-per-usd",
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  setCreditsPerUSD(creditsPerUsd: number) {
    return request("ai")<CreditsPerUSDOutputBody>({
      method: "PUT",
      path: "/api/v1/pricing/credits-per-usd",
      headers: portalHeadersFor("ai"),
      body: { credits_per_usd: creditsPerUsd },
      baseUrl: serviceBaseUrl("ai")
    });
  },

  // ---- Dashboard ----
  getDashboardSummary(params: AiWorkbenchWindowQuery = {}) {
    return request("ai")<DashboardSummaryDTO>({
      method: "GET",
      path: "/api/v1/dashboard/summary",
      query: params,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  listDashboardTopModels(params: AiWorkbenchWindowQuery & { limit?: number } = {}) {
    return request("ai")<DashboardTopModelsOutputBody>({
      method: "GET",
      path: "/api/v1/dashboard/top-models",
      query: params,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  listDashboardTopTenants(params: AiWorkbenchWindowQuery & { limit?: number } = {}) {
    return request("ai")<DashboardTopTenantsOutputBody>({
      method: "GET",
      path: "/api/v1/dashboard/top-tenants",
      query: params,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  listDashboardRecentErrors(params: AiWorkbenchWindowQuery & { limit?: number } = {}) {
    return request("ai")<DashboardRecentErrorsOutputBody>({
      method: "GET",
      path: "/api/v1/dashboard/recent-errors",
      query: params,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  // ---- Runtime limit policies ----
  listRuntimeLimitPolicies(params: Record<string, string | number | undefined> = {}) {
    return request("ai")<RuntimeLimitPoliciesOutputBody>({
      method: "GET",
      path: "/api/v1/limit-policies",
      query: params,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  createRuntimeLimitPolicy(body: Record<string, unknown>) {
    return request("ai")<RuntimeLimitPolicyDTO>({
      method: "POST",
      path: "/api/v1/limit-policies",
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  updateRuntimeLimitPolicy(policyId: string, body: Record<string, unknown>) {
    return request("ai")<RuntimeLimitPolicyDTO>({
      method: "PATCH",
      path: `/api/v1/limit-policies/${encodeURIComponent(policyId)}`,
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  updateRuntimeLimitPolicyStatus(policyId: string, status: string) {
    return request("ai")<RuntimeLimitPolicyDTO>({
      method: "PATCH",
      path: `/api/v1/limit-policies/${encodeURIComponent(policyId)}/status`,
      headers: portalHeadersFor("ai"),
      body: { status },
      baseUrl: serviceBaseUrl("ai")
    });
  },
  listTenantUpstreamAccess(tenantId: string) {
    return request("ai")<TenantUpstreamAccessOutputBody>({
      method: "GET",
      path: `/api/v1/tenants/${encodeURIComponent(tenantId)}/upstream-access`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  replaceTenantUpstreamAccess(tenantId: string, policies: TenantUpstreamPolicyRef[]) {
    return request("ai")<{ updated: boolean }>({
      method: "PUT",
      path: `/api/v1/tenants/${encodeURIComponent(tenantId)}/upstream-access`,
      headers: portalHeadersFor("ai"),
      body: { policies },
      baseUrl: serviceBaseUrl("ai")
    });
  },

  // ---- Audit logs ----
  listGatewayAuditLogs(params: Record<string, string | number | undefined> = {}) {
    return request("ai")<AuditLogsOutputBody>({
      method: "GET",
      path: "/api/v1/audit-logs",
      query: params,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },

  // ---- 风控中心（内容安全审核）----
  getRiskControlConfig() {
    return request("ai")<RiskControlConfigDTO>({
      method: "GET",
      path: "/api/v1/risk-control/config",
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  updateRiskControlConfig(body: RiskControlConfigWriteRequest) {
    return request("ai")<RiskControlConfigDTO>({
      method: "PUT",
      path: "/api/v1/risk-control/config",
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  testRiskControlModeration(text: string) {
    return request("ai")<RiskControlTestResultDTO>({
      method: "POST",
      path: "/api/v1/risk-control/test",
      headers: portalHeadersFor("ai"),
      body: { text },
      baseUrl: serviceBaseUrl("ai")
    });
  },
  listRiskControlLogs(params: Record<string, string | number | undefined> = {}) {
    return request("ai")<RiskControlLogsOutputBody>({
      method: "GET",
      path: "/api/v1/risk-control/logs",
      query: params,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  listRiskControlEvents(params: Record<string, string | number | undefined> = {}) {
    return request("ai")<RiskEventsOutputBody>({
      method: "GET",
      path: "/api/v1/risk-control/events",
      query: params,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  resolveRiskControlEvent(eventId: string, body: { status: string; note?: string }) {
    return request("ai")<RiskEventDTO>({
      method: "POST",
      path: `/api/v1/risk-control/events/${encodeURIComponent(eventId)}/resolve`,
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },

  // ---- System & 路由策略 ----
  getSystemStatus() {
    return request("ai")<SystemStatusDTO>({
      method: "GET",
      path: "/api/v1/system/status",
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  getRouteWeights(scope = "global") {
    return request("ai")<RouteWeightsOutputBody>({
      method: "GET",
      path: `/api/v1/route-weights/${encodeURIComponent(scope)}`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  putRouteWeights(scope: string, body: { cost: number; latency: number; load: number; health: number }) {
    return request("ai")<RouteWeightsOutputBody>({
      method: "PUT",
      path: `/api/v1/route-weights/${encodeURIComponent(scope)}`,
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },

  // ---- Credential pools ----
  listCredentialPools() {
    return request("ai")<CredentialPoolsOutputBody>({
      method: "GET",
      path: "/api/v1/credential-pools",
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  createCredentialPool(body: CredentialPoolWriteRequest) {
    return request("ai")<CredentialPoolDTO>({
      method: "POST",
      path: "/api/v1/credential-pools",
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  patchCredentialPool(poolId: string, body: CredentialPoolWriteRequest) {
    return request("ai")<CredentialPoolDTO>({
      method: "PATCH",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}`,
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  updateCredentialPoolStatus(poolId: string, status: "active" | "disabled") {
    return request("ai")<CredentialPoolDTO>({
      method: "PATCH",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/status`,
      headers: portalHeadersFor("ai"),
      body: { status },
      baseUrl: serviceBaseUrl("ai")
    });
  },
  deleteCredentialPool(poolId: string) {
    return request("ai")<{ deleted: boolean }>({
      method: "DELETE",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  listPoolCredentials(poolId: string) {
    return request("ai")<PoolCredentialsOutputBody>({
      method: "GET",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/credentials`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  createPoolCredential(poolId: string, body: PoolCredentialWriteRequest) {
    return request("ai")<PoolCredentialDTO>({
      method: "POST",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/credentials`,
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  patchPoolCredential(poolId: string, credId: string, body: PoolCredentialPatchRequest) {
    return request("ai")<PoolCredentialDTO>({
      method: "PATCH",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/credentials/${encodeURIComponent(credId)}`,
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  deletePoolCredential(poolId: string, credId: string) {
    return request("ai")<{ deleted: boolean }>({
      method: "DELETE",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/credentials/${encodeURIComponent(credId)}`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  refreshPoolCredential(poolId: string, credId: string) {
    return request("ai")<PoolCredentialDTO>({
      method: "POST",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/credentials/${encodeURIComponent(credId)}/refresh`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  getPoolAvailableModels(poolId: string) {
    return request("ai")<PoolAvailableModelsDTO>({
      method: "GET",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/available-models`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  listPoolModelBindings(poolId: string) {
    return request("ai")<UpstreamModelBindingsOutputBody>({
      method: "GET",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/model-bindings`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  createPoolModelBinding(poolId: string, body: UpstreamModelBindingWriteRequest) {
    return request("ai")<UpstreamModelBindingDTO>({
      method: "POST",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/model-bindings`,
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  updatePoolModelBinding(poolId: string, bindingId: string, body: UpstreamModelBindingWriteRequest) {
    return request("ai")<UpstreamModelBindingDTO>({
      method: "PATCH",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/model-bindings/${encodeURIComponent(bindingId)}`,
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  deletePoolModelBinding(poolId: string, bindingId: string) {
    return request("ai")<{ deleted: boolean }>({
      method: "DELETE",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/model-bindings/${encodeURIComponent(bindingId)}`,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
  importPoolAvailableModels(poolId: string, body: { models: string[] }) {
    return request("ai")<{ created: string[]; skipped: string[] }>({
      method: "POST",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/import-available-models`,
      headers: portalHeadersFor("ai"),
      body,
      baseUrl: serviceBaseUrl("ai")
    });
  },
  getOAuthPoolHealth() {
    return request("ai")<OAuthPoolHealthOutputBody>({
      method: "GET",
      path: "/api/v1/oauth-pool-health",
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },
};
