import { createTypedOperationRequest } from "@/api";
import { authenticatedRequest, apiHeaders, apiBaseUrl } from "@/api/request";
import { recordsApi, type RequestRecord } from "./recordsApi";
import type { AdminUsageTrendQuery, TenantUsageQuery, TenantUsageSummaryQuery } from "./model";
const request = createTypedOperationRequest(authenticatedRequest());
const common = { headers: apiHeaders, baseUrl: apiBaseUrl };
export const adminUsageApi = {
  listDailyTrend(query: AdminUsageTrendQuery, signal?: AbortSignal) {
    return request<"ai-list-daily-trend">({ ...common, method: "GET", path: "/api/v1/analytics/daily-trend", query, signal });
  },
  listUpstreamSummary(query: TenantUsageQuery, signal?: AbortSignal) {
    return request<"ai-list-usage-upstream-summary">({ ...common, method: "GET", path: "/api/v1/usage-upstream-summary", query, signal });
  }
};
export async function listTenantUsageRecords(query: TenantUsageQuery, signal?: AbortSignal) {
  const filter = { user_id: query.user_id, model: query.model_code, source: query.request_source, from: query.date_from, to: query.date_to };
  const stats = await recordsApi.summary(filter, signal);
  const records: RequestRecord[] = [];
  let cursor: string | undefined;
  const limit = Math.min(500, Math.max(1, query.limit || 20));
  do {
    const page = await recordsApi.list("settlements", { ...filter, cursor, limit: Math.min(100, limit - records.length) }, signal);
    records.push(...(page.records || [])); cursor = page.next_cursor;
  } while (cursor && records.length < limit);
  return { records, total: stats.requests, stats };
}
export function listTenantUsageSummary(query: TenantUsageSummaryQuery, signal?: AbortSignal) {
  return request<"ai-list-tenant-self-usage-summary">({ ...common, method: "GET", path: "/api/v1/tenants/me/usage-summary", query, signal });
}
export async function listCustomerUsageRecords(query: TenantUsageQuery, signal?: AbortSignal) {
  const page = await recordsApi.list("requests", { source: query.request_source, limit: Math.min(100, query.limit || 20) }, signal);
  return { items: page.records || [] };
}
export function getCustomerUsageSummary(source?: string, signal?: AbortSignal) { return recordsApi.summary({ source }, signal); }
