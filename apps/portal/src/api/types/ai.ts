// AI 业务页类型（批4）。复用 types/admin.ts 已有 AI 强类型，仅补充 V1 网关页需要、admin.ts 尚未覆盖的类型。
// 与统一后端 AI DTO 对齐：统一 { items, total }，usage 列表为 { total, stats, records }。
// 缺口字段（v4 后端确无）按 V1 风格 ?? '-' / ?? 0 优雅降级，不删 V1 的列。

import type {
  IdentityIncluded,
  IdentityIncludedTenant,
  IdentityIncludedUser
} from "@/platform/ai/identity";
import type { components } from "../generated/dai";

export type IdentityUserDTO = IdentityIncludedUser;
export type IdentityTenantDTO = IdentityIncludedTenant;
export type IdentityIncludedDTO = IdentityIncluded;
export type LiteLLMModelInfo = components["schemas"]["LiteLLMModelInfo"];
export type LiteLLMModelsOutputBody = components["schemas"]["LiteLLMModelsOutputBody"];

export type {
  AccountDTO,
  AccountsOutputBody,
  AccountWriteRequest,
  UpstreamAccountExportOutputBody,
  UpstreamAccountExportRequest,
  UpstreamAccountImportOutputBody,
  UpstreamAccountImportPreviewOutputBody,
  UpstreamAccountImportRequest,
  UpstreamAccountTransferAccountDTO,
  UpstreamAccountTransferBindingDTO,
  PriceBookDTO,
  PriceBooksOutputBody,
  PriceBookWriteRequest,
  PriceBookEntryDTO,
  PriceBookEntriesOutputBody,
  PriceBookEntryWriteRequest,
  ResolutionUSDPriceDTO,
  DashboardTopModelDTO,
  DashboardTopModelsOutputBody,
  DashboardRecentErrorDTO,
  DashboardRecentErrorsOutputBody
} from "./admin";

// ---- System status ----
export interface ComponentStatusDTO {
  status: string;
  error?: string;
}

export interface HealthRecordDTO {
  target_id: string;
  kind: string; // "deployment" | "credential" | "unknown"
  state: string; // "closed" | "open" | "half_open" | "unknown"
  consecutive_failures: number;
  opened_at?: number;
  next_probe_at?: number;
}

export interface HealthSummaryDTO {
  total_tracked: number;
  open_count: number;
  half_open_count: number;
  records: HealthRecordDTO[];
}

export interface SystemStatusDTO {
  timestamp: number;
  db: ComponentStatusDTO;
  redis: ComponentStatusDTO;
  health: HealthSummaryDTO;
}

// ---- Route weights ----
export interface ScoreWeightsDTO {
  cost: number;
  latency: number;
  load: number;
  health: number;
}

export interface RouteWeightsOutputBody {
  scope: string;
  weights: ScoreWeightsDTO;
}

// ---- Dashboard summary / top tenants ----
export interface DashboardSummaryDTO {
  total_requests: number;
  successful_requests: number;
  failed_requests: number;
  total_tokens: number;
  total_prompt_tokens: number;
  total_completion_tokens: number;
  total_catalog_base_usd: number;
  total_tenant_payable_usd: number;
  total_retail_base_usd: number;
  total_user_payable_usd: number;
  total_user_charged_usd: number;
  avg_latency_ms: number;
  avg_request_total_ms: number;
  avg_first_response_byte_ms: number;
}

export interface DashboardTopTenantDTO {
  tenant_id: string;
  request_count: number;
  total_tokens: number;
  total_tenant_payable_usd: number;
}

export interface DashboardTopTenantsOutputBody {
  items: DashboardTopTenantDTO[];
  total: number;
  included?: IdentityIncludedDTO;
}

// ---- Audit logs ----
export interface AuditLogDTO {
  id: string;
  actor?: string;
  action: string;
  object_type?: string;
  object_id?: string;
  request_summary?: unknown;
  result: string;
  http_status?: number;
  created_at?: number;
}

export interface AuditLogsOutputBody {
  items: AuditLogDTO[];
  total: number;
}

// ---- Credential pools ----
export interface CredentialPoolDTO {
  id: string;
  name: string;
  tenant_display_name: string;
  tenant_access_mode: "public" | "restricted";
  fixed_provider_type: string;
  oauth_strategy: string;
  notes?: string;
  status: string;
  price_book_id?: string;
  tenant_multiplier: number;
  created_at?: number;
  updated_at?: number;
}

export interface CredentialPoolsOutputBody {
  items: CredentialPoolDTO[];
  total: number;
}

export interface UpstreamModelBindingDTO {
  id: string;
  model_code: string;
  capability_type: string;
  api_format: string;
  upstream_model_name: string;
  status: string;
  image_stream_mode?: string;
  image_edit_transport?: "application/json" | "multipart/form-data";
  image_upstream_response_format?: "url" | "b64_json";
  image_max_output_count?: number;
  image_edit_max_output_count?: number;
  created_at?: number;
  updated_at?: number;
}

export interface UpstreamModelBindingsOutputBody {
  items: UpstreamModelBindingDTO[];
  total: number;
}

export interface UpstreamModelBindingWriteRequest {
  model_code?: string;
  capability_type?: string;
  api_format?: string;
  upstream_model_name?: string;
  status?: string;
  image_stream_mode?: string;
  image_edit_transport?: "application/json" | "multipart/form-data";
  image_upstream_response_format?: "" | "url" | "b64_json";
  image_max_output_count?: number;
  image_edit_max_output_count?: number;
}

export interface DiscoveredUpstreamModelDTO {
  id: string;
  name: string;
  capability_type: string;
  api_format: string;
  exists: boolean;
}

export interface ImportUpstreamModelsRequest {
  models: string[];
  api_format?: string;
}

export interface ModelCapabilityInferResult {
  capability_type: string;
  api_format: string;
  source: "external" | "heuristic";
}

export interface UpstreamAccountTestRequest {
  model_code: string;
  prompt?: string;
  image_edit?: boolean;
  image?: UpstreamAccountTestImage;
}

export interface UpstreamAccountTestImage {
  filename: string;
  mime_type: "image/png" | "image/jpeg" | "image/webp";
  b64_json: string;
}

export interface UpstreamAccountTestResult {
  ok: boolean;
  http_status: number;
  latency_ms: number;
  capability: string;
  api_format: string;
  upstream_model: string;
  reply_text?: string;
  image_b64?: string;
  image_mime?: string;
  image_url?: string;
  image_stream_mode?: string;
  image_edit_transport?: "application/json" | "multipart/form-data";
  image_upstream_response_format?: "url" | "b64_json";
  actual_image_format?: string;
  prompt_tokens?: number;
  output_tokens?: number;
  total_tokens?: number;
  error?: string;
}

export interface CredentialPoolWriteRequest {
  name: string;
  tenant_display_name?: string;
  tenant_access_mode?: "public" | "restricted";
  fixed_provider_type?: string;
  oauth_strategy?: string;
  notes?: string;
  price_book_id?: string;
  tenant_multiplier?: number;
}

export interface PoolCredentialDTO {
  id: string;
  pool_id: string;
  name: string;
  provider_type: string;
  email: string;
  token_type: string;
  scope: string;
  expires_at?: number;
  auth_metadata?: Record<string, unknown>;
  weight: number;
  status: string;
  invalid_reason?: string;
  last_used_at?: number;
  last_refreshed_at?: number;
  last_failed_at?: number;
  consecutive_fail_count: number;
  success_count: number;
  fail_count: number;
  created_at?: number;
  updated_at?: number;
}

export interface PoolCredentialsOutputBody {
  items: PoolCredentialDTO[];
  total: number;
}

export interface PoolCredentialWriteRequest {
  name?: string;
  provider_type?: string;
  email?: string;
  access_token: string;
  refresh_token?: string;
  token_type?: string;
  scope?: string;
  expires_at?: number;
  weight?: number;
  auth_metadata?: Record<string, unknown>;
  account_id?: string;
  plan_type?: string;
  user_id?: string;
  account_user_id?: string;
}

export interface PoolCredentialPatchRequest {
  status?: string;
  weight?: number;
}

export interface PoolAvailableModelsDTO {
  pool_id: string;
  fixed_provider_type: string;
  models: string[];
  source: string;
}

export interface OAuthPoolHealthDTO {
  pool_id: string;
  pool_name: string;
  fixed_provider_type: string;
  oauth_strategy: string;
  total: number;
  active: number;
  invalid: number;
  disabled: number;
  expiring_soon: number;
}

export interface OAuthPoolHealthOutputBody {
  items: OAuthPoolHealthDTO[];
  total: number;
}

// ---- Runtime limit policies ----
export interface RuntimeLimitPolicyDTO {
  id: string;
  scope_type: string;
  scope_id: string;
  concurrency_limit?: number;
  status: string;
  created_by?: string;
  created_at?: number;
  updated_at?: number;
}

export interface RuntimeLimitPoliciesOutputBody {
  items: RuntimeLimitPolicyDTO[];
  total: number;
  included?: IdentityIncludedDTO;
}

export interface TenantUpstreamAccessDTO {
  resource_kind: "direct_upstream" | "oauth_pool";
  resource_id: string;
  internal_name: string;
  tenant_display_name: string;
  access_mode: "public" | "restricted";
  status: string;
  access_granted: boolean;
  allowed: boolean;
  default_tenant_multiplier: number;
  tenant_multiplier_override?: number;
  effective_tenant_multiplier: number;
}

export interface TenantUpstreamAccessOutputBody {
  items: TenantUpstreamAccessDTO[];
  total: number;
}

export interface TenantUpstreamPolicyRef {
  resource_kind: "direct_upstream" | "oauth_pool";
  resource_id: string;
  access_granted: boolean;
  tenant_multiplier_override?: number;
}

// ---- 风控中心（内容安全审核 v2）----
export interface RiskControlProviderDTO {
  base_url: string;
  model: string;
  has_api_key: boolean;
  timeout_ms: number;
}

export interface KeywordEntryDTO {
  word: string;
  level: "block" | "suspect";
  require_with: string[];
  note: string;
}

export interface PinyinConfigDTO {
  enabled: boolean;
  entries: KeywordEntryDTO[];
  include_initials: boolean;
}

export interface KeywordConfigDTO {
  enabled: boolean;
  entries: KeywordEntryDTO[];
  homoglyph_map_extra: Record<string, string>;
  pinyin: PinyinConfigDTO;
}

export interface RiskControlConfigDTO {
  enabled: boolean;
  mode: "off" | "observe" | "pre_block";
  config_revision: number;
  keyword: KeywordConfigDTO;
  provider: RiskControlProviderDTO;
  thresholds: Record<string, number>;
  sample_rate: number;
  verdict_cache_ttl_seconds: number;
  scope_group_ids: string[];
  violation_window_hours: number;
  risk_event_threshold: number;
  record_non_hits: boolean;
  block_status_code: number;
  block_message: string;
}

export interface RiskControlConfigWriteRequest {
  enabled: boolean;
  mode: "off" | "observe" | "pre_block";
  keyword: KeywordConfigDTO;
  provider: {
    base_url: string;
    model: string;
    api_key?: string;
    timeout_ms: number;
  };
  thresholds: Record<string, number>;
  sample_rate: number;
  verdict_cache_ttl_seconds: number;
  scope_group_ids: string[];
  violation_window_hours: number;
  risk_event_threshold: number;
  record_non_hits: boolean;
  block_status_code: number;
  block_message: string;
}

export interface RiskControlTestResultDTO {
  flagged: boolean;
  matched_keyword?: string;
  hit_layer?: string;
  from_cache: boolean;
  highest_category?: string;
  highest_score?: number;
  category_scores?: Record<string, number>;
  error?: string;
}

export interface RiskControlLogDTO {
  id: string;
  request_id?: string;
  tenant_id?: string;
  user_id?: string;
  api_key_id?: string;
  model_code?: string;
  capability_type?: string;
  mode: string;
  action: string;
  flagged: boolean;
  matched_keyword?: string;
  hit_layer?: string;
  highest_category?: string;
  highest_score?: number;
  category_scores?: Record<string, number>;
  input_excerpt?: string;
  upstream_latency_ms?: number;
  error?: string;
  created_at?: number;
}

export interface RiskControlLogsOutputBody {
  items: RiskControlLogDTO[];
  total: number;
}

export interface RiskEventDTO {
  id: string;
  event_type: string;
  severity: "low" | "medium" | "high";
  tenant_id?: string;
  user_id?: string;
  source_log_id?: string;
  summary: string;
  detail?: Record<string, unknown>;
  status: "open" | "acknowledged" | "resolved" | "dismissed";
  resolved_by?: string;
  resolved_at?: number;
  resolution_note?: string;
  created_at?: number;
}

export interface RiskEventsOutputBody {
  items: RiskEventDTO[];
  total: number;
}
