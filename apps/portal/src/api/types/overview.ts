export interface OverviewSummary {
  total_requests: number;
  successful_requests: number;
  failed_requests: number;
  success_rate?: number;
  total_tokens: number;
  total_prompt_tokens: number;
  total_completion_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  cache_rate?: number;
  active_tenants: number;
  active_users: number;
  active_accounts: number;
  new_tenants: number;
  new_users: number;
  total_catalog_base_usd: number;
  total_tenant_payable_usd: number;
  total_user_charged_usd: number;
  gross_margin_usd: number;
  gross_margin_rate?: number;
  avg_request_total_ms: number;
  p95_request_total_ms: number;
  avg_first_response_byte_ms: number;
  p95_first_response_byte_ms: number;
}

export interface OverviewAccount {
  target_kind: string;
  target_id: string;
  target_name: string;
  provider_code: string;
  request_count: number;
  success_count: number;
  failed_count: number;
  success_rate?: number;
  prompt_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  cache_rate?: number;
  total_tokens: number;
  catalog_base_usd: number;
  tenant_payable_usd: number;
  last_requested_at?: number;
  health_status?: string;
}

export interface OverviewTrend {
  date: string;
  request_count: number;
  success_count: number;
  failed_count: number;
  total_tokens: number;
  prompt_tokens: number;
  completion_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  catalog_base_usd: number;
  tenant_payable_usd: number;
  user_charged_usd: number;
  avg_latency_ms: number;
  avg_request_total_ms: number;
  avg_first_response_byte_ms: number;
}

export interface OverviewModel {
  model_code: string;
  request_count: number;
  total_prompt_tokens: number;
  total_completion_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  total_tokens: number;
  total_catalog_base_usd: number;
  total_tenant_payable_usd: number;
  total_user_charged_usd: number;
}

export interface OverviewTenant {
  tenant_id: string;
  request_count: number;
  total_tokens: number;
  total_tenant_payable_usd: number;
}

export interface OverviewSnapshot {
  meta: { view: string; date_from: number; date_to: number; generated_at: number; comparison_window?: string };
  summary: OverviewSummary;
  comparison?: OverviewSummary;
  trends: OverviewTrend[];
  accounts: OverviewAccount[];
  models: OverviewModel[];
  tenants: OverviewTenant[];
  users: Array<{ tenant_id: string; user_id: string; request_count: number; success_count: number; failed_count: number; total_tokens: number; total_user_charged_usd: number; last_requested_at?: number }>;
  included?: { tenants?: Record<string, { tenant_name?: string; tenant_id?: string }> };
}
