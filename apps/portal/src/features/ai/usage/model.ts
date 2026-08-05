import type { components, operations } from "@dai/api-client/ai";

type Schemas = components["schemas"];

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
  totalCredits: number;
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
      row.app_name,
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
    totalCredits: rows.reduce((sum, row) => sum + Number(row.user_charged_credits || 0), 0),
    avgLatency: latencyRows.length
      ? latencyRows.reduce((sum, row) => sum + Number(row.latency_ms || 0), 0) / latencyRows.length
      : 0
  };
}
