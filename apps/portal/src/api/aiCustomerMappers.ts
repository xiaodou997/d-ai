import type { OperationResponse } from ".";
import type {
  AiApiKey,
  AiApiKeyCreatedOutput,
  AiApiKeysOutput,
  AiGroupEffectivePrice,
  AiGroupEffectivePricesOutput,
  AiLimitPolicy,
  AiSubOrder,
  AiSubPage,
  AiSubPlan,
  AiSubPurchaseBlockReason,
  AiSubPurchaseEligibility,
  AiSubPurchasePolicy,
  AiSubPurchaseResult,
  AiSubscription,
  AiVisibleGroupsOutput,
  ChatMessageDTO,
  ChatModel,
  ChatSession,
  ChatSessionDetail,
  ConsoleImageJob
} from "./types/aiCustomer";

type ApiKeyTransport = OperationResponse<"ai-update-user-self-api-key">;
type GroupPricesTransport = OperationResponse<"ai-list-user-self-group-effective-prices">;
type GroupPriceTransport = NonNullable<GroupPricesTransport["items"]>[number];
type PlanPageTransport = OperationResponse<"ai-list-user-self-subscription-plans">;
type PlanTransport = NonNullable<PlanPageTransport["items"]>[number];
type OrderPageTransport = OperationResponse<"ai-list-user-self-subscription-orders">;
type OrderTransport = NonNullable<OrderPageTransport["items"]>[number];
type SubscriptionPageTransport = OperationResponse<"ai-list-user-self-subscriptions">;
type SubscriptionTransport = NonNullable<SubscriptionPageTransport["items"]>[number];
type ChatSessionTransport = OperationResponse<"ai-api-v1-users-me-workspace-chat-sessions:create">;
type ImageJobsTransport = OperationResponse<"ai-api-v1-users-me-workspace-image-jobs">;
type ImageJobTransport = NonNullable<ImageJobsTransport["items"]>[number];

function toLimitStatus(value: string): AiLimitPolicy["status"] {
  if (value === "active" || value === "disabled") return value;
  throw new Error(`Unexpected limit policy status: ${value}`);
}

function toLimitPolicy(value: NonNullable<ApiKeyTransport["limit_policy"]>): AiLimitPolicy {
  return {
    id: value.id,
    scope_type: value.scope_type,
    scope_id: value.scope_id,
    concurrency_limit: value.concurrency_limit,
    status: toLimitStatus(value.status),
    created_by: value.created_by,
    created_at: value.created_at,
    updated_at: value.updated_at
  };
}

export function toApiKey(value: ApiKeyTransport): AiApiKey {
  return {
    id: value.id,
    owner_type: value.owner_type,
    tenant_id: value.tenant_id,
    user_id: value.user_id,
    group_id: value.group_id,
    last_four: value.last_four,
    name: value.name,
    quota_limit_micro_usd: value.quota_limit_micro_usd,
    quota_used_micro_usd: value.quota_used_micro_usd,
    status: value.status,
    expires_at: value.expires_at,
    last_used_at: value.last_used_at,
    limit_policy: value.limit_policy ? toLimitPolicy(value.limit_policy) : undefined,
    created_by: value.created_by,
    created_at: value.created_at,
    updated_at: value.updated_at
  };
}

export function toApiKeys(value: OperationResponse<"ai-list-user-self-api-keys">): AiApiKeysOutput {
  return { items: value.items?.map(toApiKey) ?? [], total: value.total };
}

export function toCreatedApiKey(
  value: OperationResponse<"ai-create-user-self-api-key">
): AiApiKeyCreatedOutput {
  return { plaintext_key: value.plaintext_key, key: toApiKey(value.key) };
}

export function toVisibleGroups(
  value: OperationResponse<"ai-list-user-self-groups">
): AiVisibleGroupsOutput {
  return { items: value.items?.map((item) => ({ ...item })) ?? [], total: value.total };
}

function toGroupPrice(value: GroupPriceTransport): AiGroupEffectivePrice {
  return {
    model_code: value.model_code,
    capability_type: value.capability_type,
    token_price_tiers: value.token_price_tiers?.map((tier) => ({ ...tier })) ?? [],
    image_default_price_usd: value.image_default_price_usd,
    video_default_price_usd: value.video_default_price_usd,
    image_prices: value.image_prices?.map((item) => ({ ...item })) ?? undefined,
    video_prices: value.video_prices?.map((item) => ({ ...item })) ?? undefined,
    audio_tts_per_1m_chars_usd: value.audio_tts_per_1m_chars_usd,
    audio_stt_per_minute_usd: value.audio_stt_per_minute_usd
  };
}

export function toGroupPrices(value: GroupPricesTransport): AiGroupEffectivePricesOutput {
  return {
    group_id: value.group_id,
    effective_user_multiplier: value.effective_user_multiplier,
    items: value.items?.map(toGroupPrice) ?? [],
    total: value.total
  };
}

function toPurchaseReason(value: string): AiSubPurchaseBlockReason {
  switch (value) {
    case "purchase_order_processing":
    case "purchase_plan_already_queued":
    case "subscription_queue_full":
    case "advance_purchase_not_allowed":
    case "purchase_lifetime_limit_reached":
    case "purchase_rolling_limit_reached":
    case "purchase_calendar_limit_reached":
      return value;
    default:
      throw new Error(`Unexpected subscription purchase reason: ${value}`);
  }
}

function toPurchasePolicy(value: PlanTransport["purchase_policy"]): AiSubPurchasePolicy {
  let periodType: AiSubPurchasePolicy["period_type"];
  if (value.period_type === "none" || value.period_type === "rolling" || value.period_type === "calendar") {
    periodType = value.period_type;
  } else {
    throw new Error(`Unexpected subscription period type: ${value.period_type}`);
  }
  let calendarUnit: AiSubPurchasePolicy["calendar_unit"];
  if (
    value.calendar_unit === undefined ||
    value.calendar_unit === "day" ||
    value.calendar_unit === "week" ||
    value.calendar_unit === "month"
  ) {
    calendarUnit = value.calendar_unit;
  } else {
    throw new Error(`Unexpected subscription calendar unit: ${value.calendar_unit}`);
  }
  return {
    lifetime_max_purchases: value.lifetime_max_purchases,
    period_type: periodType,
    period_max_purchases: value.period_max_purchases,
    rolling_window_hours: value.rolling_window_hours,
    calendar_unit: calendarUnit,
    calendar_timezone: value.calendar_timezone,
    allow_advance_purchase: value.allow_advance_purchase,
    version: value.version
  };
}

function toEligibility(value: NonNullable<PlanTransport["purchase_eligibility"]>): AiSubPurchaseEligibility {
  return {
    allowed: value.allowed,
    primary_reason: value.primary_reason ? toPurchaseReason(value.primary_reason) : undefined,
    blocking_rules:
      value.blocking_rules?.map((rule) => ({
        reason: toPurchaseReason(rule.reason),
        retry_at: rule.retry_at,
        limit: rule.limit,
        used: rule.used
      })) ?? [],
    retry_at: value.retry_at
  };
}

function toPlan(value: PlanTransport): AiSubPlan {
  return {
    id: value.id,
    name: value.name,
    description: value.description,
    price_micro_usd: value.price_micro_usd,
    duration_days: value.duration_days,
    total_limit_micro_usd: value.total_limit_micro_usd,
    window_5h_limit_micro_usd: value.window_5h_limit_micro_usd,
    window_7d_limit_micro_usd: value.window_7d_limit_micro_usd,
    sale_limit: value.sale_limit,
    sold_count: value.sold_count,
    available_count: value.available_count,
    sold_out: value.sold_out,
    groups: value.groups?.map((group) => ({ ...group })) ?? [],
    purchase_policy: toPurchasePolicy(value.purchase_policy),
    purchase_eligibility: value.purchase_eligibility
      ? toEligibility(value.purchase_eligibility)
      : undefined
  };
}

function toOrder(value: OrderTransport): AiSubOrder {
  return {
    id: value.id,
    order_no: value.order_no,
    tenant_id: value.tenant_id,
    user_id: value.user_id,
    plan_id: value.plan_id,
    plan_name: value.plan_name,
    price_micro_usd: value.price_micro_usd,
    status: value.status,
    debit_reference: value.debit_reference,
    subscription_id: value.subscription_id,
    fail_reason: value.fail_reason,
    purchase_policy_version: value.purchase_policy_version,
    purchase_policy: toPurchasePolicy(value.purchase_policy),
    paid_at: value.paid_at,
    created_at: value.created_at,
    updated_at: value.updated_at
  };
}

function toSubscription(value: SubscriptionTransport): AiSubscription {
  return {
    id: value.id,
    tenant_id: value.tenant_id,
    user_id: value.user_id,
    plan_id: value.plan_id,
    order_id: value.order_id,
    plan_name: value.plan_name,
    duration_days: value.duration_days,
    status: value.status,
    activated_at: value.activated_at,
    expires_at: value.expires_at,
    total_limit_micro_usd: value.total_limit_micro_usd,
    total_used_micro_usd: value.total_used_micro_usd,
    total_remaining_micro_usd: value.total_remaining_micro_usd,
    window_5h: { ...value.window_5h },
    window_7d: { ...value.window_7d },
    groups: value.groups?.map((group) => ({ ...group })) ?? [],
    created_at: value.created_at,
    updated_at: value.updated_at
  };
}

function toPage<TTransport, TView>(
  value: { items: TTransport[] | null; total: number; page: number; size: number },
  mapper: (item: TTransport) => TView
): AiSubPage<TView> {
  return {
    items: value.items?.map(mapper) ?? [],
    total: value.total,
    page: value.page,
    size: value.size
  };
}

export function toPlanPage(value: PlanPageTransport): AiSubPage<AiSubPlan> {
  return toPage(value, toPlan);
}

export function toOrderPage(value: OrderPageTransport): AiSubPage<AiSubOrder> {
  return toPage(value, toOrder);
}

export function toSubscriptionPage(
  value: SubscriptionPageTransport
): AiSubPage<AiSubscription> {
  return toPage(value, toSubscription);
}

export function toPurchaseResult(
  value: OperationResponse<"ai-create-user-self-subscription-order">
): AiSubPurchaseResult {
  return {
    order: toOrder(value.order),
    subscription: value.subscription ? toSubscription(value.subscription) : undefined,
    processing: value.processing
  };
}

export function toCurrentSubscription(
  value: OperationResponse<"ai-get-user-self-current-subscription">
): AiSubscription | null {
  return value ? toSubscription(value) : null;
}

function toChatSession(value: ChatSessionTransport): ChatSession {
  const { $schema: _schema, ...session } = value;
  return session;
}

export function toChatSessions(
  value: OperationResponse<"ai-api-v1-users-me-workspace-chat-sessions">
): { items: ChatSession[]; total: number } {
  return { items: value.items?.map(toChatSession) ?? [], total: value.total };
}

export function toChatModels(
  value: OperationResponse<"ai-api-v1-users-me-workspace-chat-models">
): { items: ChatModel[]; total: number } {
  return {
    items:
      value.items?.map((item) => ({
        ...item,
        available_api_formats: item.available_api_formats ?? []
      })) ?? [],
    total: value.total
  };
}

function toChatMessage(value: ChatSessionDetailTransport["messages"] extends (infer Item)[] | null ? Item : never): ChatMessageDTO {
  return { ...value };
}

type ChatSessionDetailTransport = OperationResponse<"ai-api-v1-users-me-workspace-chat-sessions-sessionid">;

export function toChatSessionDetail(value: ChatSessionDetailTransport): ChatSessionDetail {
  return {
    session: toChatSession(value.session),
    messages: value.messages?.map(toChatMessage) ?? []
  };
}

function toImageJob(value: ImageJobTransport): ConsoleImageJob {
  return {
    ...value,
    operation: value.operation === "generation" || value.operation === "edit" ? value.operation : undefined,
    revised_prompts: value.revised_prompts ?? undefined,
    assets: value.assets?.map((asset) => ({ ...asset })) ?? undefined
  };
}

export function toImageJobs(value: ImageJobsTransport): { items: ConsoleImageJob[]; total: number } {
  return { items: value.items?.map(toImageJob) ?? [], total: value.total };
}

export { toChatSession, toImageJob, toOrder, toSubscription };
