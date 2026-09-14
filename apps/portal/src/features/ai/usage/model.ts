import type { components, operations } from "@/api/ai";
import type { RequestRecord, RecordSummary } from "./recordsApi";
type Schemas = components["schemas"];
export type DailyTrendRowDTO = Schemas["DailyTrendRowDTO"];
export type UsageUpstreamSummaryRowDTO = Schemas["UsageUpstreamSummaryRowDTO"];
export type TenantUsageSummaryRow = Schemas["UsageSummaryRowDTO"];
export type TenantUsageSummaryQuery = NonNullable<operations["ai-list-tenant-self-usage-summary"]["parameters"]["query"]>;
export type AdminUsageTrendQuery = NonNullable<operations["ai-list-daily-trend"]["parameters"]["query"]>;
export type TenantUsageLog = RequestRecord;
export type CustomerUsageLog = RequestRecord;
export type TenantUsageStats = RecordSummary;
export type CustomerUsageSummary = RecordSummary;
export interface TenantUsageQuery { user_id?: string; model_code?: string; request_source?: string; date_from?: string; date_to?: string; limit?: number; offset?: number; }
export type CustomerUsageQuery = TenantUsageQuery;
