import type { RequestAdapter } from "@/api";
import type { components as AiComponents } from "@/api/ai";
import type { components as PlatformComponents } from "@/api/platform";

import { authenticatedRequest, apiHeaders, apiBaseUrl } from "@/api/request";
import type {
  AdminUsageQuery,
  AdminUsageRankingQuery,
  AdminUsageSummaryQuery,
  AdminUsageTrendQuery,
  AdminUsageUpstreamSummaryQuery,
  CustomerUsageQuery,
  TenantUsageQuery,
  TenantUsageSummaryQuery
} from "./model";

type AiSchemas = AiComponents["schemas"];
type PlatformSchemas = PlatformComponents["schemas"];

export interface AdminUsageApi {
  listLogs: (query: AdminUsageQuery, signal?: AbortSignal) => Promise<AiSchemas["UsageLogsOutputBody"]>;
  getDetail: (requestId: string, signal?: AbortSignal) => Promise<AiSchemas["UsageLogDetailDTO"]>;
  listSummary: (query: AdminUsageSummaryQuery, signal?: AbortSignal) => Promise<AiSchemas["UsageSummaryOutputBody"]>;
  listUnitSummary: (query: AdminUsageSummaryQuery, signal?: AbortSignal) => Promise<AiSchemas["UsageUnitSummaryOutputBody"]>;
  listUpstreamSummary: (query: AdminUsageUpstreamSummaryQuery, signal?: AbortSignal) => Promise<AiSchemas["UsageUpstreamSummaryOutputBody"]>;
  listUserRanking: (query: AdminUsageRankingQuery, signal?: AbortSignal) => Promise<AiSchemas["UsageUserRankingOutputBody"]>;
  listDailyTrend: (query: AdminUsageTrendQuery, signal?: AbortSignal) => Promise<AiSchemas["DailyTrendOutputBody"]>;
}

export function createAdminUsageApi(adapter: RequestAdapter = authenticatedRequest()): AdminUsageApi {
  const base = apiBaseUrl;
  const headers = apiHeaders;
  return {
    listLogs: (query, signal) => adapter({ method: "GET", path: "/api/v1/usage-logs", query, headers, baseUrl: base, signal }),
    getDetail: (requestId, signal) => adapter({
      method: "GET",
      path: `/api/v1/usage-logs/${encodeURIComponent(requestId)}`,
      headers,
      baseUrl: base,
      signal
    }),
    listSummary: (query, signal) => adapter({ method: "GET", path: "/api/v1/usage-summary", query, headers, baseUrl: base, signal }),
    listUnitSummary: (query, signal) => adapter({ method: "GET", path: "/api/v1/usage-unit-summary", query, headers, baseUrl: base, signal }),
    listUpstreamSummary: (query, signal) => adapter({ method: "GET", path: "/api/v1/usage-upstream-summary", query, headers, baseUrl: base, signal }),
    listUserRanking: (query, signal) => adapter({ method: "GET", path: "/api/v1/usage-ranking/users", query, headers, baseUrl: base, signal }),
    listDailyTrend: (query, signal) => adapter({ method: "GET", path: "/api/v1/analytics/daily-trend", query, headers, baseUrl: base, signal })
  };
}

export const adminUsageApi = createAdminUsageApi();

export function listAdminUsageDailyTrend(query: AdminUsageTrendQuery, signal?: AbortSignal) {
  return adminUsageApi.listDailyTrend(query, signal);
}

export interface TenantUsageApi {
  listRecords: (query: TenantUsageQuery, signal?: AbortSignal) => Promise<AiSchemas["TenantUsageLogsOutputBody"]>;
  listSummary: (query: TenantUsageSummaryQuery, signal?: AbortSignal) => Promise<AiSchemas["UsageSummaryOutputBody"]>;
  listUsers: (signal?: AbortSignal) => Promise<PlatformSchemas["PageEndUserItem"]>;
}

export function createTenantUsageApi(
  aiRequest: RequestAdapter = authenticatedRequest(),
  platformRequest: RequestAdapter = authenticatedRequest()
): TenantUsageApi {
  return {
    listRecords: (query, signal) => aiRequest({
      method: "GET",
      path: "/api/v1/tenants/me/usage-logs",
      query,
      headers: apiHeaders,
      baseUrl: apiBaseUrl,
      signal
    }),
    listSummary: (query, signal) => aiRequest({
      method: "GET",
      path: "/api/v1/tenants/me/usage-summary",
      query,
      headers: apiHeaders,
      baseUrl: apiBaseUrl,
      signal
    }),
    listUsers: (signal) => platformRequest({
      method: "GET",
      path: "/api/v1/users",
      query: { page: 1, size: 200 },
      headers: apiHeaders,
      baseUrl: apiBaseUrl,
      signal
    })
  };
}

export const tenantUsageApi = createTenantUsageApi();

export function listTenantUsageRecords(query: TenantUsageQuery, signal?: AbortSignal) {
  return tenantUsageApi.listRecords(query, signal);
}

export function listTenantUsageSummary(query: TenantUsageSummaryQuery, signal?: AbortSignal) {
  return tenantUsageApi.listSummary(query, signal);
}

export interface CustomerUsageApi {
  listRecords: (query: CustomerUsageQuery, signal?: AbortSignal) => Promise<AiSchemas["UserUsageLogsOutputBody"]>;
  getSummary: (requestSource?: string, signal?: AbortSignal) => Promise<AiSchemas["UserUsageSummaryDTO"]>;
}

export function createCustomerUsageApi(adapter: RequestAdapter = authenticatedRequest()): CustomerUsageApi {
  return {
    listRecords: (query, signal) => adapter({
      method: "GET",
      path: "/api/v1/user-usage-logs",
      query,
      headers: apiHeaders,
      baseUrl: apiBaseUrl,
      signal
    }),
    getSummary: (requestSource, signal) => adapter({
      method: "GET",
      path: "/api/v1/user-usage-summary",
      query: { request_source: requestSource || undefined },
      headers: apiHeaders,
      baseUrl: apiBaseUrl,
      signal
    })
  };
}

export const customerUsageApi = createCustomerUsageApi();

export function listCustomerUsageRecords(query: CustomerUsageQuery, signal?: AbortSignal) {
  return customerUsageApi.listRecords(query, signal);
}

export function getCustomerUsageSummary(requestSource?: string, signal?: AbortSignal) {
  return customerUsageApi.getSummary(requestSource);
}
