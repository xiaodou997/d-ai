// AI 租户侧业务类型。统一后端为 huma 扁平强类型：
// - 列表读端点返回 { items, total }
// - usage 端点（1:1 搬 V1 console）返回 { total, stats, records }（snake_case）
// 字段以 contracts/openapi.yaml / transport DTO 为准。
//
// 「上游+分组」重构后：分组与私有价格表归租户所有；平台公共价格表只读共享。
// 用户零售与上游账号结算是两条独立账本，任何倍率都不会跨账本相乘。
//
// 复用既有 types/tenant.ts 里已定义的 AI key 形状。

import type { IdentityIncluded } from "@/platform/ai/identity";
import type { components } from "@/api/ai";

export type { TenantAiApiKey, TenantAiApiKeysOutputBody } from "./tenant";

// ==================== 可用模型（分组暴露的去重模型集合） ====================

export interface TenantAiAvailableModel {
  id: string;
  model_code: string;
  model_name?: string;
  capability_type: string;
  context_window?: number | null;
  default_max_output_tokens: number;
  max_output_tokens?: number | null;
  status?: string;
  input_per_1m_usd_min?: number;
  input_per_1m_usd_max?: number;
  output_per_1m_usd_min?: number;
  output_per_1m_usd_max?: number;
  cache_write_per_1m_usd_min?: number;
  cache_write_per_1m_usd_max?: number;
  cache_read_per_1m_usd_min?: number;
  cache_read_per_1m_usd_max?: number;
  has_context_tiers?: boolean;
  image_default_price_usd_min?: number;
  image_default_price_usd_max?: number;
  video_default_price_usd_min?: number;
  video_default_price_usd_max?: number;
  image_prices?: Array<{ resolution: string; price_usd_min: number; price_usd_max: number }>;
  video_prices?: Array<{ resolution: string; price_usd_min: number; price_usd_max: number }>;
}

export interface TenantAiAvailableModelsOutputBody {
  items: TenantAiAvailableModel[];
  total: number;
}

// ==================== 租户自有分组 ====================

export type TenantAiVisibleGroup = components["schemas"]["GroupDTO"];

export interface TenantAiVisibleGroupsOutputBody {
  items: TenantAiVisibleGroup[];
  total: number;
}

export type TenantAiGroupWriteRequest = components["schemas"]["GroupWriteRequest"];

export interface TenantAiGroupTarget {
  id: string;
  group_id: string;
  account_id?: string;
  credential_pool_id?: string;
  target_type: "account" | "pool";
  account_name?: string;
  pool_name?: string;
  default_provider_family?: string;
  fixed_provider_type?: string;
  priority: number;
  status: "active" | "disabled";
  // 该绑定的上游资源当前是否仍可被本租户路由。管理员把资源转 restricted、撤销授权或
  // 停用后，绑定不会自动消失，请求会被网关 fail-closed 拒；available=false 时以
  // unavailable_reason 说明原因，用于把这种「已绑定但请求全失败」的哑故障显式化。
  available?: boolean;
  unavailable_reason?: "inactive" | "access_revoked" | "missing";
}

export interface TenantAiGroupTargetWriteRequest {
  account_id?: string;
  credential_pool_id?: string;
  priority?: number;
  status?: "active" | "disabled";
}

export type TenantAiClientSurfacePolicy = components["schemas"]["GroupClientSurfacePolicyDTO"];
export type TenantAiClientSurfacePolicyWrite = components["schemas"]["GroupClientSurfacePolicyWriteRequest"];
export type TenantAiClientSurface = NonNullable<TenantAiClientSurfacePolicy["allowed_surfaces"]>[number];
export type TenantAiDispatchRule = components["schemas"]["GroupDispatchRuleDTO"];
export type TenantAiDispatchRuleWriteRequest = components["schemas"]["GroupDispatchRuleWriteRequest"];
export type TenantAiDispatchPreviewRequest = components["schemas"]["GroupDispatchPreviewRequest"];
export type TenantAiDispatchPreview = components["schemas"]["GroupDispatchPreviewDTO"];
export type TenantAiDispatchModel = components["schemas"]["DispatchModelDTO"];

export interface TenantAiDispatchPriceConflict {
	group_id: string;
	group_name: string;
	rule_id: string;
	api_format: string;
	match_value: string;
	target_model: string;
	required_capability: string;
}

export interface TenantAiGroupDependencyCounts {
	user_bindings: number;
	api_key_bindings: number;
	subscription_plans: number;
}

// ==================== 价格表 ====================

export interface TenantAiPriceBook {
  id: string;
  owner_type: "platform" | "tenant";
  owner_tenant_id?: string;
  writable: boolean;
  name: string;
  description: string;
  status: "active" | "disabled";
  revision: number;
  created_at?: number;
  updated_at?: number;
}

export interface TenantAiTokenPriceTierUSD {
  up_to_input_tokens: number | null;
  input_per_1m_usd: number;
  output_per_1m_usd: number;
  cache_write_per_1m_usd: number;
  cache_read_per_1m_usd: number;
}

export interface TenantAiPriceBookEntry {
  model_code: string;
  capability_type: string;
  token_price_tiers: TenantAiTokenPriceTierUSD[];
  image_default_price_usd: number;
  video_default_price_usd: number;
  image_prices?: Array<{ resolution: string; price: number }>;
  video_prices?: Array<{ resolution: string; price: number }>;
  audio_tts_per_1m_chars_usd: number;
  audio_stt_per_minute_usd: number;
  source?: string;
  manually_edited?: boolean;
  updated_at?: number;
}

export interface TenantAiLiteLLMPriceModel {
  model_code: string;
  capability_type: string;
  token_price_tiers: TenantAiTokenPriceTierUSD[];
}

export type TenantAiPriceBookEntryWriteRequest = Omit<
  TenantAiPriceBookEntry,
  "model_code" | "source" | "manually_edited" | "updated_at"
>;

export interface TenantAiPriceBookTransferBundle {
  schema_version: 1;
  name: string;
  description?: string;
  entries: TenantAiPriceBookEntry[];
}

// ==================== 安全上游目录 ====================

export interface TenantAiUpstreamModel {
  model_code: string;
  capability_type: string;
  api_format: string;
  availability: "available" | "no_price_configured";
  price?: TenantAiPriceBookEntry;
}

export interface TenantAiUpstreamResource {
  id: string;
  resource_kind: "direct_upstream" | "oauth_pool";
  name: string;
  tenant_multiplier: number;
  price_book_id?: string;
  price_book_name?: string;
  price_book_revision?: number;
  models: TenantAiUpstreamModel[];
}

// ==================== 租户→用户 分组绑定（加价倍率 + 套餐收窄） ====================

export interface TenantAiUserGroup {
  group_id: string;
  group_name?: string;
  multiplier_override?: number | null; // 为空=继承分组默认用户倍率
  status: string;
}

export interface TenantAiUserGroupsOutputBody {
  items: TenantAiUserGroup[];
  total: number;
}

export interface TenantAiUserGroupWriteRequest {
  multiplier_override?: number | null;
  user_default_visible?: boolean;
  user_default_multiplier?: number | null;
  status?: string;
}

// ==================== 限流策略（scope-only：user / api_key） ====================

export interface TenantAiLimitPolicy {
  id: string;
  scope_type: string;
  scope_id: string;
  concurrency_limit?: number | null;
  status: "active" | "disabled";
  created_by?: string | null;
  created_at?: number | null;
  updated_at?: number | null;
}

export interface TenantAiLimitPoliciesOutputBody {
  items: TenantAiLimitPolicy[];
  total: number;
}

export interface TenantAiLimitPolicyWriteRequest {
  concurrency_limit?: number | null;
  status?: "active" | "disabled";
}

// ==================== 分组生效售价（每模型 USD 单价） ====================

export interface TenantAiGroupEffectivePrice {
  model_code: string;
  capability_type: string;
  token_price_tiers: TenantAiTokenPriceTier[];
  image_default_price_usd: number;
  video_default_price_usd: number;
  image_prices?: Array<{ resolution: string; price: number }>;
  video_prices?: Array<{ resolution: string; price: number }>;
  audio_tts_per_1m_chars_usd: number;
  audio_stt_per_minute_usd: number;
}

export interface TenantAiTokenPriceTier {
  up_to_input_tokens: number | null;
  input_per_1m_usd: number;
  output_per_1m_usd: number;
  cache_write_per_1m_usd: number;
  cache_read_per_1m_usd: number;
}

export interface TenantAiGroupEffectivePricesOutputBody {
  group_id: string;
  retail_price_book_id: string;
  effective_user_multiplier: number;
  items: TenantAiGroupEffectivePrice[];
  total: number;
}

// ==================== API Key 写端点 ====================

export interface TenantAiApiKeyWriteRequest {
  name: string;
  group_id: string;
  quota_limit_micro_usd?: number | null;
  status?: string;
  expires_at?: number | null;
  limit_policy?: TenantAiLimitPolicyWriteRequest | null;
}

export interface TenantAiApiKeyCreatedOutputBody {
  plaintext_key: string;
  key: import("./tenant").TenantAiApiKey;
}

export interface TenantAiApiKeyRevealOutputBody {
  plaintext_key: string;
}

export interface TenantAiDeleteOutputBody {
  deleted: boolean;
}

// ==================== Dashboard ====================

export interface TenantAiDashboardSummary {
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
}

export interface TenantAiDashboardTopModel {
  model_code: string;
  request_count: number;
  total_tokens: number;
  total_tenant_payable_usd: number;
}

export interface TenantAiDashboardTopModelsOutputBody {
  items: TenantAiDashboardTopModel[];
  total: number;
}

export interface TenantAiDashboardRecentError {
  request_id: string;
  model_code: string;
  request_status: string;
  error_code?: string | null;
  error_message?: string | null;
  http_status?: number | null;
  created_at?: number | null;
}

export interface TenantAiDashboardRecentErrorsOutputBody {
  items: TenantAiDashboardRecentError[];
  total: number;
}

// ==================== 应用运行层聊天（信封端点 + SSE，独立 fetch 通道） ====================

export interface ChatModel {
  group_id: string;
  group_name?: string;
  effective_user_multiplier?: number;
  billing_group_label?: string;
  model_code: string;
  capability_type: string;
  default_api_format: string;
  available_api_formats: string[];
  supports_stream: boolean;
  status: string;
}

export interface ChatSession {
  id: string;
  title: string;
  model_code: string;
  group_id?: string;
  group_name?: string;
  effective_user_multiplier?: number;
  billing_group_label?: string;
  provider_api_format: string;
  selected_route_id: string;
  status: string;
  created_at: number;
  updated_at: number;
}

export interface ChatMessageDTO {
  id: string;
  role: string;
  content: string;
  protocol?: string;
  route_id?: string;
  usage?: Record<string, unknown>;
  error?: Record<string, unknown>;
  created_at: number;
}

export interface ChatSessionDetail {
  session: ChatSession;
  messages: ChatMessageDTO[];
}

export interface ChatUiMessage {
  id: string;
  role: string;
  content: string;
  protocol?: string;
}

export interface ConsoleImageModel {
  group_id?: string;
  group_name?: string;
  effective_user_multiplier?: number;
  billing_group_label?: string;
  model_code: string;
  capability_type: string;
  status: string;
  max_output_count?: number;
  edit_max_output_count?: number;
}

export interface ConsoleImageJob {
  id: string;
  operation?: "generation" | "edit";
  group_id?: string;
  model_code: string;
  prompt: string;
  retry_prompt?: string;
  status: string;
  storage_policy: string;
  raw_image_retained: boolean;
  size?: string;
  quality?: string;
  style?: string;
  response_format?: string;
  requested_output_count?: number;
  caller_charge_usd: number;
  image_count: number;
  inline_count: number;
  url_count: number;
  revised_prompts?: string[];
  assets?: ConsoleImageTaskAsset[];
  error_message?: string;
  created_at: number;
  completed_at?: number;
}

export interface ConsoleImageTaskAsset {
  id?: string;
  index?: number;
  preview_url?: string;
  display_url: string;
  original_url?: string;
  content_type?: string;
  size_bytes?: number;
  preview_content_type?: string;
  preview_size_bytes?: number;
  width?: number;
  height?: number;
  expires_at?: number;
}

export interface ConsoleImageGenerateRequest {
  operation?: "generation" | "edit";
  model: string;
  group_id?: string;
  prompt: string;
  n?: number;
  images?: Array<{ image_url: string }>;
  mask?: { image_url: string };
  size?: string;
  response_format?: string;
  stream?: boolean;
  background?: string;
  input_fidelity?: string;
  moderation?: string;
  output_format?: string;
  output_compression?: number;
  user?: string;
}

// ==================== AI 订阅制套餐（docs/ai-subscription-design.md §7.2） ====================
// 售价与额度统一为 micro-USD（1 USD = 1,000,000 micro-USD）。

// 套餐绑定的分组（额度消耗 = 基准价 × 套餐扣额倍率）。
export interface TenantSubPlanGroup {
  id: string;
  name: string;
  quota_debit_multiplier: number;
}

export type TenantSubPurchasePeriodType = "none" | "rolling" | "calendar";
export type TenantSubPurchaseCalendarUnit = "day" | "week" | "month";

export interface TenantSubPurchasePolicyInput {
  lifetime_max_purchases?: number | null;
  period_type: TenantSubPurchasePeriodType;
  period_max_purchases?: number | null;
  rolling_window_hours?: number | null;
  calendar_unit?: TenantSubPurchaseCalendarUnit | "";
  calendar_timezone?: string;
  allow_advance_purchase: boolean;
}

export interface TenantSubPurchasePolicy extends TenantSubPurchasePolicyInput {
  version: number;
}

export interface TenantSubPurchasePolicyRevision {
  plan_id: string;
  version: number;
  policy: TenantSubPurchasePolicy;
  changed_by?: string;
  changed_at: string;
}

export interface TenantSubPlan {
  id: string;
  tenant_id: string;
  name: string;
  description: string;
  price_micro_usd: number;
  duration_days: number;
  total_limit_micro_usd: number;
  window_5h_limit_micro_usd?: number | null;
  window_7d_limit_micro_usd?: number | null;
  status: "draft" | "on_sale" | "off_sale";
  sort_order: number;
  sale_limit?: number | null;
  sold_count: number;
  reserved_count: number;
  available_count?: number | null;
  sold_out: boolean;
  groups: TenantSubPlanGroup[];
  purchase_policy: TenantSubPurchasePolicy;
  created_at: string;
  updated_at: string;
}

export interface TenantSubWindow {
  limit_micro_usd?: number | null;
  used_micro_usd: number;
  remaining_micro_usd?: number | null;
  reset_at?: string | null;
}

export interface TenantSubscription {
  id: string;
  tenant_id: string;
  user_id: string;
  plan_id: string;
  order_id: string;
  plan_name: string;
  duration_days: number;
  status: string;
  activated_at?: string | null;
  expires_at?: string | null;
  total_limit_micro_usd: number;
  total_used_micro_usd: number;
  total_remaining_micro_usd: number;
  window_5h: TenantSubWindow;
  window_7d: TenantSubWindow;
  groups: TenantSubPlanGroup[];
  created_at: string;
  updated_at: string;
}

export interface TenantSubOrder {
  id: string;
  order_no: string;
  tenant_id: string;
  user_id: string;
  plan_id: string;
  plan_name: string;
  price_micro_usd: number;
  status: string;
  billing_event_id?: string;
  subscription_id?: string;
  fail_reason?: string;
  purchase_policy_version: number;
  purchase_policy: TenantSubPurchasePolicy;
  paid_at?: string;
  created_at: string;
  updated_at: string;
}

export interface TenantSubPage<T> {
  items: T[];
  total: number;
  page: number;
  size: number;
  included: IdentityIncluded;
}

export interface TenantSubPlanGroupInput {
  group_id: string;
  quota_debit_multiplier: number;
}

export interface TenantSubPlanWriteRequest {
  name: string;
  description?: string;
  price_micro_usd: number;
  duration_days: number;
  total_limit_micro_usd: number;
  window_5h_limit_micro_usd?: number | null;
  window_7d_limit_micro_usd?: number | null;
  sort_order?: number;
  sale_limit?: number | null;
  groups: TenantSubPlanGroupInput[];
  purchase_policy?: TenantSubPurchasePolicyInput;
}
