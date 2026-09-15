import { createTypedOperationRequest, type OperationResponse } from "@/api";
import { authenticatedRequest, apiBaseUrl, apiHeaders } from "@/api/request";

export type RequestRecord = NonNullable<OperationResponse<"ai-v2-requests">["records"]>[number];
export type RecordPage = OperationResponse<"ai-v2-requests">;
export type RecordSummary = OperationResponse<"ai-v2-record-summary">;
export type RecordKind = "requests" | "errors" | "settlements";
export interface RecordQuery { tenant_name?: string; user_name?: string; group?: string; api_key_name?: string; tenant_id?: string; user_id?: string; model?: string; source?: string; from?: string; to?: string; cursor?: string; limit?: number; }
const request = createTypedOperationRequest(authenticatedRequest());
const common = { headers: apiHeaders, baseUrl: apiBaseUrl };
export const recordsApi = {
  list(kind: RecordKind, query: RecordQuery, signal?: AbortSignal): Promise<RecordPage> {
    if (kind === "errors") return request<"ai-v2-request-errors">({ ...common, method: "GET", path: "/api/v2/request-errors", query, signal });
    if (kind === "settlements") return request<"ai-v2-settlements">({ ...common, method: "GET", path: "/api/v2/billing/settlements", query, signal });
    return request<"ai-v2-requests">({ ...common, method: "GET", path: "/api/v2/requests", query, signal });
  },
  detail(id: string, signal?: AbortSignal) {
    return request<"ai-v2-settlement-detail">({ ...common, method: "GET", path: `/api/v2/billing/settlements/${encodeURIComponent(id)}`, pathParams: { requestID: id }, signal });
  },
  summary(query: RecordQuery, signal?: AbortSignal) {
    return request<"ai-v2-record-summary">({ ...common, method: "GET", path: "/api/v2/request-summary", query, signal });
  },
  startDebug(body: { tenant_id: string; api_key_id: string; model: string; hours: number }) {
    return request<"ai-v2-debug-session">({ ...common, method: "POST", path: "/api/v2/request-debug-sessions", body });
  },
  debug(id: string) {
    return request<"ai-v2-debug-payload">({ ...common, method: "GET", path: `/api/v2/requests/${encodeURIComponent(id)}/debug`, pathParams: { requestID: id } });
  },
  refund(id: string, reason: string) {
    return request<"ai-v2-refund-settlement">({ ...common, method: "POST", path: `/api/v2/billing/settlements/${encodeURIComponent(id)}/refund`, pathParams: { requestID: id }, body: { reason } });
  }
};
export function chargeReason(reason: string): string {
  return ({ reported_usage: "按上游已报告用量结算", reported_usage_before_disconnect: "客户端连接中断，仅结算取消前已报告用量", request_error_waived: "请求明确报错，费用与计费额度均免收", missing_usage: "上游未报告用量，未计费", missing_media_evidence: "缺少实际产出依据，未计费", confirmed_media_output: "按已确认的媒体产出结算", zero_charge: "本次费用为零", unconfirmed_execution: "执行中断且结果未确认，未计费" } as Record<string, string>)[reason] || reason;
}
export function chargeLabel(row: RequestRecord): string {
  return ({ pending: "结算处理中", posted: "已结算", waived: "未计费", review: "结算待处理", refunded: "已退款" } as Record<string, string>)[row.charge.state] || "待结算";
}
export function microUSD(value?: number | null): string { return value == null ? "—" : `$${(value / 1_000_000).toFixed(6)}`; }
export function tokenCount(value?: number | null): string { return value == null ? "未报告" : value.toLocaleString("zh-CN"); }
