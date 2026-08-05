// AI 用户自助（userType=4）类型 —— 对齐统一后端 Huma self 端点的 snake_case DTO。
// 见 internal/ai/transport/{user_self,apikeys,usage,pricing}.go。

import type { PortalAppRuntimeConfig } from "@/platform/ai/apps";

// ==================== API Key ====================
export interface AiApiKey {
  id: string;
  owner_type: string;
  tenant_id: string;
  user_id?: string | null;
  group_id: string;
  last_four?: string | null;
  name: string;
  quota_limit_credits?: number | null;
  quota_used_credits: number;
  status: string; // active | disabled
  expires_at?: number | null; // Unix 毫秒
  last_used_at?: number | null;
  limit_policy?: AiLimitPolicy | null;
  created_by?: string | null;
  created_at?: number | null;
  updated_at?: number | null;
}

export interface AiApiKeysOutput {
  items: AiApiKey[];
  total: number;
}

export interface AiApiKeyWriteRequest {
  name: string;
  group_id: string;
  quota_limit_credits?: number | null;
  status?: string;
  expires_at?: number | null;
  limit_policy?: AiLimitPolicyWriteRequest | null;
}

export interface AiApiKeyCreatedOutput {
  plaintext_key: string;
  key: AiApiKey;
}

export interface AiApiKeyRevealOutput {
  plaintext_key: string;
}

export interface AiVisibleGroup {
  id: string;
  name: string;
  description?: string;
  effective_user_multiplier: number;
  status: string;
}

export interface AiVisibleGroupsOutput {
  items: AiVisibleGroup[];
  total: number;
}

export interface AiGroupEffectivePrice {
  model_code: string;
  capability_type: string;
  token_price_tiers: AiEffectiveTokenPriceTier[];
  image_default_price_credits: number;
  video_default_price_credits: number;
  image_prices?: Array<{ resolution: string; price: number }>;
  video_prices?: Array<{ resolution: string; price: number }>;
  audio_tts_per_1m_chars_credits: number;
  audio_stt_per_minute_credits: number;
}

export interface AiEffectiveTokenPriceTier {
  up_to_input_tokens: number | null;
  input_per_1m_credits: number;
  output_per_1m_credits: number;
  cache_write_per_1m_credits: number;
  cache_read_per_1m_credits: number;
}

export interface AiGroupEffectivePricesOutput {
  group_id: string;
  effective_user_multiplier: number;
  credits_per_usd: number;
  items: AiGroupEffectivePrice[];
  total: number;
}

// ==================== API Key 限流 ====================
export interface AiLimitPolicy {
  id: string;
  scope_type: string;
  scope_id: string;
  concurrency_limit?: number | null;
  status: "active" | "disabled";
  created_by?: string | null;
  created_at?: number | null;
  updated_at?: number | null;
}

export interface AiLimitPoliciesOutput {
  items: AiLimitPolicy[];
  total: number;
}

export interface AiLimitPolicyWriteRequest {
  concurrency_limit?: number | null;
  status?: "active" | "disabled";
}

// ==================== AI 订阅制套餐（docs/ai-subscription-design.md §7.1） ====================
// 额度一律为微积分（micro，1 积分 = 10000 微积分），前端用 formatCredits 除以 10000 展示。

// 套餐/订阅覆盖的分组（额度消耗 = 命中分组基准价 × 套餐扣额倍率）。
export interface AiSubGroup {
  id: string;
  name: string;
  quota_debit_multiplier: number;
}

export type AiSubPurchaseBlockReason =
  | "purchase_order_processing"
  | "purchase_plan_already_queued"
  | "subscription_queue_full"
  | "advance_purchase_not_allowed"
  | "purchase_lifetime_limit_reached"
  | "purchase_rolling_limit_reached"
  | "purchase_calendar_limit_reached";

export interface AiSubPurchasePolicy {
  lifetime_max_purchases?: number | null;
  period_type: "none" | "rolling" | "calendar";
  period_max_purchases?: number | null;
  rolling_window_hours?: number | null;
  calendar_unit?: "day" | "week" | "month" | "";
  calendar_timezone?: string;
  allow_advance_purchase: boolean;
  version: number;
}

export interface AiSubPurchaseRuleDecision {
  reason: AiSubPurchaseBlockReason;
  retry_at?: string;
  limit?: number;
  used: number;
}

export interface AiSubPurchaseEligibility {
  allowed: boolean;
  primary_reason?: AiSubPurchaseBlockReason;
  blocking_rules: AiSubPurchaseRuleDecision[];
  retry_at?: string;
}

export interface AiSubPurchaseProblemMeta {
  retry_at?: string;
  blocking_rules?: AiSubPurchaseRuleDecision[];
}

export interface AiSubPlan {
  id: string;
  name: string;
  description: string;
  price_credits: number;
  duration_days: number;
  total_limit_micro: number;
  window_5h_limit_micro?: number | null;
  window_7d_limit_micro?: number | null;
  sale_limit?: number | null;
  sold_count: number;
  available_count?: number | null;
  sold_out: boolean;
  groups: AiSubGroup[];
  purchase_policy: AiSubPurchasePolicy;
  purchase_eligibility?: AiSubPurchaseEligibility;
}

export interface AiSubWindow {
  limit_micro?: number | null; // null = 该窗口不限
  used_micro: number;
  remaining_micro?: number | null;
  reset_at?: string | null; // 当前窗口重置时刻（RFC3339），后端按服务器时间算好
}

export interface AiSubscription {
  id: string;
  tenant_id: string;
  user_id: string;
  plan_id: string;
  order_id: string;
  plan_name: string;
  duration_days: number;
  status: string; // pending / active / expired / cancelled
  activated_at?: string | null;
  expires_at?: string | null;
  total_limit_micro: number;
  total_used_micro: number;
  total_remaining_micro: number;
  window_5h: AiSubWindow;
  window_7d: AiSubWindow;
  groups: AiSubGroup[];
  created_at: string;
  updated_at: string;
}

export interface AiSubOrder {
  id: string;
  order_no: string;
  tenant_id: string;
  user_id: string;
  plan_id: string;
  plan_name: string;
  price_credits: number;
  status: string; // created / deducting / paid / failed
  urm_event_id?: string;
  subscription_id?: string;
  fail_reason?: string;
  purchase_policy_version: number;
  purchase_policy: AiSubPurchasePolicy;
  paid_at?: string;
  created_at: string;
  updated_at: string;
}

// 购买响应：201 已开通(processing=false, subscription 有值) / 202 处理中(processing=true, 轮询订单)。
export interface AiSubPurchaseResult {
  order: AiSubOrder | null;
  subscription?: AiSubscription | null;
  processing: boolean;
}

export interface AiSubPage<T> {
  items: T[];
  total: number;
  page: number;
  size: number;
}

// ==================== 应用运行层聊天（信封 + SSE） ====================
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
  caller_charge_credits: number;
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

// 使用侧脱敏视图:服务端不返回模型/分组/提示词等底层字段。
export interface AiVisibleAgent {
  id: string;
  name: string;
  description: string;
	capability: "chat" | "image_generation" | "image_edit";
	prompt_strategy: "none" | "caller_variables" | "bound_prompt_exact";
  publisher_label?: string;
	variables: string[];
	prompt_names: string[];
}

export interface UserAppPromptDTO {
  owner_type: "user";
  owner_tenant_id?: string;
  owner_user_id?: string;
  id: string;
  name: string;
  description: string;
  status: "active" | "disabled";
	template_text: string;
	variables: string[];
  created_by?: string;
  updated_by?: string;
  created_at?: number;
  updated_at?: number;
}

export interface UserAppPromptDetailDTO {
	prompt: UserAppPromptDTO;
}

export interface UserAppPromptsOutputBody {
  items: UserAppPromptDTO[];
  total: number;
}

export interface UserAppPromptWriteRequest {
  name?: string;
  description?: string;
  status?: "active" | "disabled";
  template_text?: string;
}

export interface UserAppPromptBindingDTO {
	prompt_id: string;
	prompt_name: string;
	variables: string[];
	binding_role: "primary" | "fragment";
	display_order: number;
}

export interface UserAppDTO {
  owner_type: "user";
  owner_tenant_id?: string;
  owner_user_id?: string;
  id: string;
  name: string;
  description: string;
	status: "active" | "disabled";
	capability: "chat" | "image_generation" | "image_edit";
	prompt_strategy: "none" | "caller_variables" | "bound_prompt_exact";
	prompt_bindings: UserAppPromptBindingDTO[];
  group_id: string;
  model_code: string;
  runtime_config: PortalAppRuntimeConfig;
  published_by_tenant?: boolean;
  created_by?: string;
  updated_by?: string;
  created_at?: number;
  updated_at?: number;
}

export interface UserAppsOutputBody {
  items: UserAppDTO[];
  total: number;
}

export interface UserAppWriteRequest {
	template_id?: "standard_chat" | "keyword_selector" | "dynamic_prompt_composition" | "text_to_image" | "image_to_image";
  name?: string;
  description?: string;
  status?: "active" | "disabled";
	capability?: "chat" | "image_generation" | "image_edit";
	prompt_strategy?: "none" | "caller_variables" | "bound_prompt_exact";
	prompt_ids?: string[];
  group_id?: string;
  model_code?: string;
  runtime_config?: PortalAppRuntimeConfig;
}

export interface AiVisibleAgentsOutput {
  items: AiVisibleAgent[];
  total: number;
}

// 应用密钥：每把密钥只能绑定一个已发布应用（最小权限），不支持直绑裸模型。
export interface AiAppKey {
  id: string;
  owner_type: string;
  tenant_id: string;
  user_id?: string;
  name: string;
  last_four: string;
  status: "active" | "disabled";
  agent_id: string;
  agent_name?: string;
  agent_owner_type?: "tenant" | "user";
  agent_owner_tenant_id?: string;
  expires_at?: number;
  created_by?: string;
  created_at?: number;
  updated_at?: number;
}

export interface AiAppKeysOutput {
  items: AiAppKey[];
  total: number;
}

export interface AiAppKeyWriteRequest {
  name: string;
  status?: "active" | "disabled";
  agent_id: string;
  expires_at?: number | null;
}

export interface AiAppKeyCreatedOutput {
  plaintext_key: string;
  key: AiAppKey;
}

export interface AiAppKeyRevealOutput {
  plaintext_key: string;
}
