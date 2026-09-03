import type { components, operations } from "@/api/ai";
import {
  normalizeIdentityIncluded,
  resolveIdentityTenantLabel,
  resolveIdentityTenantMeta,
  resolveIdentityUserLabel,
  resolveIdentityUserMeta,
  type IdentityIncluded
} from "@/platform/ai/identity";

type Schemas = components["schemas"];

export type UsageLogDTO = Schemas["UsageLogDTO"];
export type UsageStatsDTO = Schemas["UsageStatsDTO"];
export type UsageLogDetailDTO = Schemas["UsageLogDetailDTO"];
export interface UsageAttemptDetail {
  route_id?: string;
  provider_code?: string;
  upstream_model?: string;
  endpoint_id?: string;
  pool_id?: string;
  credential_id?: string;
  http_status?: number;
  outcome: string;
  latency_ms?: number;
  first_byte_ms?: number;
  total_ms?: number;
  error?: string;
  score?: number;
}
export type UsageSummaryRowDTO = Schemas["UsageSummaryRowDTO"];
export type UsageUnitSummaryRowDTO = Schemas["UsageUnitSummaryRowDTO"];
export type UsageUpstreamSummaryRowDTO = Schemas["UsageUpstreamSummaryRowDTO"];
export type UsageUserRankingRowDTO = Schemas["UsageUserRankingRowDTO"];
export type DailyTrendRowDTO = Schemas["DailyTrendRowDTO"];
export type IdentityIncludedDTO = IdentityIncluded;

export type AdminUsageQuery = NonNullable<operations["ai-list-usage-logs"]["parameters"]["query"]>;
export type AdminUsageSummaryQuery = NonNullable<operations["ai-list-usage-summary"]["parameters"]["query"]>;
export type AdminUsageRankingQuery = NonNullable<operations["ai-list-usage-user-ranking"]["parameters"]["query"]>;
export type AdminUsageTrendQuery = NonNullable<operations["ai-list-daily-trend"]["parameters"]["query"]>;
export type AdminUsageUpstreamSummaryQuery = NonNullable<operations["ai-list-usage-upstream-summary"]["parameters"]["query"]>;

export interface IdentityDisplay {
  label: string;
  meta: string;
}

export interface UsageRowIdentity {
  tenant: IdentityDisplay;
  user: IdentityDisplay;
}

export type AdminUsageRow = UsageLogDTO & { identity: UsageRowIdentity };
export type AdminUsageRankingRow = UsageUserRankingRowDTO & { identity: UsageRowIdentity };

export interface UsageFilters {
  tenant_id: string;
  user_id: string;
  model_code: string;
  request_status: string;
  request_source: string;
}

export interface UsagePagination {
  page: number;
  size: number;
  total: number;
}

export interface UsageMetric {
  label: string;
  value: string;
  hint?: string;
}

export interface UsageHighlight {
  label: string;
  value: string;
  hint: string;
}

export interface UsageFilterChip {
  key: string;
  label: string;
  value: string;
}

export type UsageWorkbenchTab = "records" | "errors" | "ranking" | "upstream";

export function mapAdminUsageRows(rows: UsageLogDTO[] | null | undefined, rawIncluded: unknown): AdminUsageRow[] {
  const included = normalizeIdentityIncluded(rawIncluded);
  return (rows ?? []).map((row) => ({ ...row, identity: usageIdentity(row, included) }));
}

export function mapAdminUsageRankingRows(
  rows: UsageUserRankingRowDTO[] | null | undefined,
  rawIncluded: unknown
): AdminUsageRankingRow[] {
  const included = normalizeIdentityIncluded(rawIncluded);
  return (rows ?? []).map((row) => ({ ...row, identity: usageIdentity(row, included) }));
}

export function normalizeUsageAttempts(value: unknown): UsageAttemptDetail[] {
  if (!Array.isArray(value)) return [];
  return value.filter(isRecord).map((item) => ({
    route_id: optionalString(item.route_id),
    provider_code: optionalString(item.provider_code),
    upstream_model: optionalString(item.upstream_model),
    endpoint_id: optionalString(item.endpoint_id),
    pool_id: optionalString(item.pool_id),
    credential_id: optionalString(item.credential_id),
    http_status: optionalNumber(item.http_status),
    outcome: optionalString(item.outcome) || "unknown",
    latency_ms: optionalNumber(item.latency_ms),
    first_byte_ms: optionalNumber(item.first_byte_ms),
    total_ms: optionalNumber(item.total_ms),
    error: optionalString(item.error),
    score: optionalNumber(item.score)
  }));
}

function usageIdentity(
  row: { tenant_id?: string | null; user_id?: string | null; external_user_id?: string | null },
  included: IdentityIncluded
): UsageRowIdentity {
  const tenantId = row.tenant_id ?? "";
  const userId = row.user_id ?? "";
  const externalUserId = row.external_user_id ?? "";
  return {
    tenant: {
      label: resolveIdentityTenantLabel(tenantId, included),
      meta: resolveIdentityTenantMeta(tenantId, included)
    },
    user: userId
      ? {
          label: resolveIdentityUserLabel(userId, included),
          meta: resolveIdentityUserMeta(userId, included)
        }
      : {
          label: externalUserId || "未命名用户",
          meta: ""
        }
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function optionalString(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function optionalNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

// 租户侧使用记录
import type { components as PlatformComponents } from "@/api/platform";

type PlatformSchemas = PlatformComponents["schemas"];

export type TenantUsageLog = Schemas["TenantUsageLogDTO"];
export type TenantUsageStats = Schemas["UsageStatsDTO"];
export type TenantUsageQuery = NonNullable<operations["ai-list-tenant-self-usage-logs"]["parameters"]["query"]>;
export type TenantUsageSummaryQuery = NonNullable<operations["ai-list-tenant-self-usage-summary"]["parameters"]["query"]>;
export type TenantUsageSummaryRow = Schemas["UsageSummaryRowDTO"];
export type TenantUsageUser = PlatformSchemas["EndUserItem"];

export interface TenantUsageFilters {
  dateRange: [number, number] | null;
  userId: string;
  modelCode: string;
  requestStatus: string;
  requestSource: string;
}

export type TenantUsageRow = TenantUsageLog & {
  userLabel: string;
};

export const EMPTY_TENANT_USAGE_STATS: TenantUsageStats = {
  total_requests: 0,
  success_count: 0,
  failed_count: 0,
  total_tokens: 0,
  total_catalog_base_usd: 0,
  total_tenant_payable_usd: 0,
  total_user_charged_usd: 0,
  avg_latency_ms: 0,
  avg_request_total_ms: 0,
  avg_first_response_byte_ms: 0
};

export function defaultTenantUsageDateRange(): [number, number] {
  const to = new Date();
  const from = new Date();
  from.setDate(from.getDate() - 6);
  from.setHours(0, 0, 0, 0);
  to.setHours(23, 59, 59, 999);
  return [from.getTime(), to.getTime()];
}

export function defaultTenantUsageFilters(): TenantUsageFilters {
  return {
    dateRange: defaultTenantUsageDateRange(),
    userId: "",
    modelCode: "",
    requestStatus: "",
    requestSource: ""
  };
}

export function mapTenantUsageRows(
  records: TenantUsageLog[] | null | undefined,
  users: TenantUsageUser[]
): TenantUsageRow[] {
  const directory = new Map(users.map((user) => [String(user.userId), user]));
  return (records ?? []).map((record) => {
    const user = record.user_id ? directory.get(record.user_id) : undefined;
    return {
      ...record,
      userLabel: record.user_id
        ? record.username || user?.username || user?.email || record.user_id
        : record.external_user_id || "-"
    };
  });
}

// 用户侧使用记录
export type CustomerUsageLog = Schemas["UserUsageLogDTO"];
export type CustomerUsageSummary = Schemas["UserUsageSummaryDTO"];
export type CustomerUsageQuery = NonNullable<operations["ai-list-user-self-usage-logs"]["parameters"]["query"]>;

export interface CustomerUsageFilters {
  requestSource: string;
  requestStatus: string;
  keyword: string;
  limit: number;
}

export interface CustomerUsageStats {
  totalRequests: number;
  successRequests: number;
  failedRequests: number;
  totalTokens: number;
  totalAmountUSD: number;
  avgLatency: number;
}

export function filterCustomerUsageRows(rows: CustomerUsageLog[], filters: CustomerUsageFilters) {
  const keyword = filters.keyword.trim().toLowerCase();
  return rows.filter((row) => {
    if (filters.requestStatus && row.request_status !== filters.requestStatus) return false;
    if (!keyword) return true;
    return [
      row.request_id,
      row.trace_id,
      row.model_code,
      row.group_name_snapshot,
      row.billing_group_label_snapshot,
      row.error_code,
      row.error_message
    ].filter(Boolean).some((value) => String(value).toLowerCase().includes(keyword));
  });
}

export function summarizeCustomerUsage(rows: CustomerUsageLog[]): CustomerUsageStats {
  const successRequests = rows.filter((row) => row.request_status === "success").length;
  const latencyRows = rows.filter((row) => row.latency_ms != null);
  return {
    totalRequests: rows.length,
    successRequests,
    failedRequests: rows.length - successRequests,
    totalTokens: rows.reduce((sum, row) => sum + Number(row.total_tokens || 0), 0),
    totalAmountUSD: rows.reduce((sum, row) => sum + Number(row.user_charged_usd || 0), 0),
    avgLatency: latencyRows.length
      ? latencyRows.reduce((sum, row) => sum + Number(row.latency_ms || 0), 0) / latencyRows.length
      : 0
  };
}
