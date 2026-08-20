export interface AuditLogItem {
  id: number;
  eventType: string;
  principalType: string;
  decision: string;
  userId?: string;
  createdAt: number;
  reasonCode?: string;
  reasonMessage?: string;
}

export interface AdminUserItem {
  userId: string;
  username: string;
  email?: string;
  status: number;
  statusText: string;
  credentialState: "active" | "pending_activation";
  createdTime: number;
}

export interface PageAuditLogItem {
  items: AuditLogItem[];
  total: number;
  page: number;
  size: number;
}

export interface PageAdminUserItem {
  items: AdminUserItem[];
  total: number;
  page: number;
  size: number;
}

export interface DashboardAlertItem {
  requestId: string;
  status: string;
  createdTime: number;
  settlementError?: string;
}

export interface DashboardAlertsOutputBody {
  failedTransactions: DashboardAlertItem[];
}

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

export interface TenantListItem {
  tenantId: string;
  tenantName: string;
  contactPerson?: string;
  contactEmail?: string;
  status: number;
  statusDisplay?: string;
  balanceUsd?: number;
  userCount?: number;
  createdTime?: number;
}

export interface AccountBalanceOutput {
  currency: string;
  totalUsd: number;
  usedUsd: number;
  remainingUsd: number;
  availableUsd: number;
  permanentUsd: number;
	timedUsd: number;
	outstandingDebtMicroUsd: number;
	serviceState: "active" | "blocked_debt";
	balanceLots?: Array<{
		balanceLotId: string;
		totalUsd: number;
		remainingUsd: number;
		createdAt: string;
		expiresAt?: string | null;
		source: string;
	}>;
}

export interface PageTenantListItem {
  items: TenantListItem[];
  total: number;
  page: number;
  size: number;
}

export interface EndUserItem {
  userId: string;
  tenantId: string;
  tenantName?: string;
  username: string;
  nickname?: string;
  email?: string;
  phone?: string;
  status: number;
  credentialState: "active" | "pending_activation";
  balanceUsd?: number;
  createdTime?: number;
  lastLoginTime?: number;
}

export interface PageEndUserItem {
  items: EndUserItem[];
  total: number;
  page: number;
  size: number;
}

export interface DashboardTopModelDTO {
  model_code: string;
  request_count: number;
  total_tokens: number;
  total_tenant_payable_usd: number;
}

export interface DashboardTopModelsOutputBody {
  items: DashboardTopModelDTO[];
  total: number;
}

export interface DashboardRecentErrorDTO {
  request_id: string;
  model_code: string;
  requested_model?: string;
  matched_dispatch_rule_summary?: string;
  resolved_logical_model?: string;
  resolved_provider_family?: string;
  client_api_format?: string;
  provider_api_format?: string;
  upstream_model?: string;
  protocol_conversion_enabled: boolean;
  request_status: string;
  tenant_id?: string;
  user_id?: string;
  created_at?: number;
}

export interface DashboardRecentErrorsOutputBody {
  items: DashboardRecentErrorDTO[];
  total: number;
}

export interface PriceBookDTO {
  id: string;
  name: string;
  description: string;
  status: "active" | "disabled";
  created_at?: number;
  updated_at?: number;
}

export interface PriceBooksOutputBody {
  items: PriceBookDTO[];
  total: number;
}

export interface PriceBookWriteRequest {
  name?: string;
  description?: string;
  status?: "active" | "disabled";
}

export interface ResolutionUSDPriceDTO {
  resolution: string;
  price: number;
}

export interface TokenPriceTierDTO {
  up_to_input_tokens: number | null;
  input_per_1m_usd: number;
  output_per_1m_usd: number;
  cache_write_per_1m_usd: number;
  cache_read_per_1m_usd: number;
}

export interface PriceBookEntryDTO {
  model_code: string;
  capability_type: string;
  token_price_tiers: TokenPriceTierDTO[];
  image_default_price_usd: number;
  video_default_price_usd: number;
  image_prices?: ResolutionUSDPriceDTO[];
  video_prices?: ResolutionUSDPriceDTO[];
  audio_tts_per_1m_chars_usd: number;
  audio_stt_per_minute_usd: number;
  source: string;
  manually_edited: boolean;
  updated_at?: number;
}

export interface PriceBookEntriesOutputBody {
  items: PriceBookEntryDTO[];
  total: number;
}

export interface PriceBookEntryWriteRequest {
  capability_type?: string;
  token_price_tiers?: TokenPriceTierDTO[];
  image_default_price_usd?: number;
  video_default_price_usd?: number;
  image_prices?: ResolutionUSDPriceDTO[];
  video_prices?: ResolutionUSDPriceDTO[];
  audio_tts_per_1m_chars_usd?: number;
  audio_stt_per_minute_usd?: number;
}

// ---- 上游账号（ai_upstream_accounts；原 provider + endpoint 合并为顶级实体）----
export interface AccountDTO {
  id: string;
  name: string;
  tenant_display_name: string;
  tenant_access_mode: "public" | "restricted";
  base_url: string;
  extra_headers?: unknown;
  default_provider_family: string;
  concurrency_limit?: number;
  price_book_id?: string;
  tenant_multiplier?: number;
  status: "active" | "invalid" | "disabled";
  invalid_reason?: string;
  invalid_at?: number;
  created_at?: number;
  updated_at?: number;
}

export interface AccountsOutputBody {
  items: AccountDTO[];
  total: number;
}

export interface AccountWriteRequest {
  name: string;
  tenant_display_name?: string;
  tenant_access_mode?: "public" | "restricted";
  base_url: string;
  api_key?: string;
  extra_headers?: unknown;
  default_provider_family?: string;
  concurrency_limit?: number | null;
  price_book_id?: string;
  tenant_multiplier?: number;
}

export interface UpstreamAccountTransferBindingDTO {
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
}

export interface UpstreamAccountTransferAccountDTO {
  name: string;
  tenant_display_name: string;
  tenant_access_mode: "public" | "restricted";
  base_url: string;
  api_key: string;
  default_provider_family: string;
  concurrency_limit?: number;
  status: string;
  extra_headers?: unknown;
  model_bindings?: UpstreamAccountTransferBindingDTO[];
}

export interface UpstreamAccountExportRequest {
  account_ids: string[];
  include_model_bindings?: boolean;
}

export interface UpstreamAccountExportOutputBody {
  schema_version: number;
  exported_at: string;
  contains_plaintext_api_keys: boolean;
  accounts: UpstreamAccountTransferAccountDTO[];
}

export interface UpstreamAccountImportRequest {
  accounts: UpstreamAccountTransferAccountDTO[];
  default_price_book_id?: string;
  default_tenant_multiplier?: number;
  duplicate_account_strategy?: "skip";
  duplicate_binding_strategy?: "skip";
}

export interface UpstreamAccountImportPreviewItemDTO {
  name: string;
  base_url: string;
  action: "create" | "skip" | "error";
  reason?: string;
  model_binding_count: number;
  duplicate_model_bindings?: number;
  warnings?: string[];
}

export interface UpstreamAccountImportSummaryDTO {
  create_accounts: number;
  skip_accounts: number;
  create_model_bindings: number;
  skip_model_bindings: number;
  error_accounts: number;
}

export interface UpstreamAccountImportPreviewOutputBody {
  items: UpstreamAccountImportPreviewItemDTO[];
  summary: UpstreamAccountImportSummaryDTO;
}

export interface UpstreamAccountImportOutputBody {
  created_account_ids: string[];
  skipped_accounts: { name: string; reason: string }[];
  created_model_bindings: { account_name: string; model_code: string; binding_id: string }[];
  skipped_model_bindings: { name: string; reason: string }[];
  summary: UpstreamAccountImportSummaryDTO;
}

export interface RechargeRecordItem {
  orderId: string;
  orderType: string;
  paidAmountMinor: number;
  amountUsd: number;
  status: string;
  note: string;
  userId: string;
  username: string;
  tenantName: string;
  createdTime?: number;
}

export interface RechargeOutputBody {
  orderId: string;
  balanceLotId: string;
  tenantId: string;
  userId: string;
  currency: string;
  amountMicroUsd: number;
  paidAmountMinor: number;
  clearedDebtUsd: number;
  balanceLotUsd: number;
  orderTime: number;
}

export interface PageRechargeRecordItem {
  items: RechargeRecordItem[];
  total: number;
  page: number;
  size: number;
}

export interface DebtStatusOutputBody {
  owner_type: "tenant" | "user";
  account_id: string;
  outstanding_debt_micro_usd: number;
  service_state: "active" | "blocked_debt";
}

// ---- 新增类型 ----

export interface CreateAdminUserOutput {
  userId: string;
  username: string;
  activationToken: string;
  activationExpiresIn: number;
}

export interface ActivationCredentialOutput {
  activationToken: string;
  activationExpiresIn: number;
}

export interface TenantDetailOutput {
  tenantId: string;
  tenantName: string;
  contactPerson?: string;
  contactEmail?: string;
  status: number;
  statusDisplay: string;
  createdTime: number;
  isWildcard: boolean;
  clientIds: string[];
}

// ---- 控制概览（数据分析）----
export interface GlobalStatsRow {
  currency: string;
  tenantRechargePaidMinor: number;
  tenantRechargeAmountUsd: number;
  activeTenants: number;
  tenantTotalBalanceUsd: number;
  userRechargePaidMinor: number;
  userRechargeAmountUsd: number;
  newUsers: number;
  userTotalBalanceUsd: number;
}
export interface TrendPoint {
  timeLabel: string;
  amountUsd: number;
}
export interface ConsumptionTrendOutput {
  totalUsd: number;
  dataPoints: TrendPoint[];
}
export interface ResourceStatItem {
  clientName: string;
  clientId: string;
  amountUsd: number;
  percentage: string;
}
export interface FailedTxAlert {
  requestId: string;
  settlementError: string;
  status: string;
  createdTime: number;
}
export interface DashboardAlertsOutput {
  failedTransactions: FailedTxAlert[];
}

// ---- 批量使用记录退款结果（batch-refund）----
export interface BatchOpError {
  requestId: string;
  reason: string;
}
export interface BatchOpResult {
  succeeded: string[];
  failed: BatchOpError[];
  totalTenantUsd: number;
  totalUserUsd: number;
  successCount: number;
  failCount: number;
}

// ---- JWT 密钥（GET /api/v1/jwt-keys，对应后端 auth.KeyInfo）----
export interface JwtKeyItem {
  id: number;
  kid: string;
  status: string;
  createdTime: number;
  graceUntil?: number;
  retiredTime?: number;
}

// ==================== 微信支付在线充值（管理端） ====================

export interface PaymentGlobalSettings {
  tenantCustomTopupFeeBp: number;
  tenantWithdrawFeeBp: number;
  tenantCustomValidityDays?: number | null;
  tenantTopupPackages: TopupPackage[];
}

export interface TopupPackage {
  id: string;
  name: string;
  paymentAmountMicroUsd: number;
  giftAmountMicroUsd: number;
  validityDays?: number | null;
  badge?: string;
  enabled: boolean;
  sortOrder: number;
}

export interface WechatConfig {
  enabled: boolean;
  mock: boolean;
  verifyMode: "platform_cert" | "public_key";
  appId: string;
  mchId: string;
  mchCertSerialNo: string;
  notifyBaseUrl: string;
  orderTtlSeconds: number;
  hasPrivateKey: boolean;
  hasApiv3Key: boolean;
  wechatPayPublicKeyId: string;
  hasWechatPayPublicKey: boolean;
}

export interface WechatConfigWriteInput {
  enabled: boolean;
  mock: boolean;
  verifyMode: "platform_cert" | "public_key";
  appId: string;
  mchId: string;
  mchCertSerialNo: string;
  notifyBaseUrl: string;
  orderTtlSeconds: number;
  mchPrivateKey?: string | null;
  apiv3Key?: string | null;
  wechatPayPublicKeyId?: string | null;
  wechatPayPublicKey?: string | null;
}

export interface PaymentOrderItem {
  orderId: string;
  scene: "user_topup" | "tenant_topup";
  tenantName?: string;
  username?: string;
  status: string;
  paymentCurrency: string;
  paymentAmountMinor: number;
  grossAmountMicroUsd: number;
  feeAmountMicroUsd: number;
  giftAmountMicroUsd: number;
  creditedAmountMicroUsd: number;
  topupMode: "custom" | "package";
  packageName?: string;
  transactionId?: string;
  createdAt: number;
  paidAt?: number | null;
  balanceExpiresAt?: number | null;
}

export interface PagePaymentOrderItem {
  items: PaymentOrderItem[];
  total: number;
  page: number;
  size: number;
}

export type RechargePaymentStatus = "not_required" | "created" | "paying" | "paid" | "closed" | "expired";
export type RechargeFulfillmentStatus = "pending" | "credited" | "partially_reversed" | "reversed";
export type RechargeRefundStatus = "none" | "refunded" | "not_applicable";

export interface RechargeCreditDetail {
  balanceOrderId: string;
  orderType: string;
  primary: boolean;
  creditAmountMicroUsd: number;
  status: string;
  note?: string;
  balanceExpiresAt?: number | null;
  reversedAt?: number | null;
  reversedBy?: string;
  reversalReason?: string;
  reversedAmountMicroUsd: number;
  lostAmountMicroUsd: number;
  lotId?: string;
  grantedAmountMicroUsd: number;
  consumedAmountMicroUsd: number;
  remainingAmountMicroUsd: number;
  lotStatus: string;
  refundId?: string;
  refundAvailableMicroUsd: number;
  refundNonAvailableMicroUsd: number;
  refundExpiredMicroUsd: number;
  refundAccountDebitMicroUsd: number;
  refundBalanceAfterMicroUsd: number;
}

export interface AdminRefundRecord {
  refundId: string;
  method: "wechat" | "offline";
  refundReference: string;
  channelRefundId?: string;
  refundAmountMinor: number;
  status: "completed";
  refundedAt: number;
  reason: string;
  note?: string;
  operatorId: string;
  createdAt: number;
}

export interface AdminRechargeOrder {
  orderId: string;
  balanceOrderId?: string;
  method: "manual" | "online";
  targetType: "tenant" | "user";
  orderType: string;
  tenantId: string;
  tenantName: string;
  userId?: string;
  username?: string;
  paidAmountMinor: number;
  grossAmountMicroUsd: number;
  feeAmountMicroUsd: number;
  giftAmountMicroUsd: number;
  creditedAmountMicroUsd: number;
  tenantIncomeMicroUsd: number;
  paymentStatus: RechargePaymentStatus;
  fulfillmentStatus: RechargeFulfillmentStatus;
  refundStatus: RechargeRefundStatus;
  outTradeNo?: string;
  transactionId?: string;
  topupMode?: string;
  packageName?: string;
  channel?: string;
  note?: string;
  failNote?: string;
  createdAt: number;
  paidAt?: number | null;
  paymentExpiresAt?: number | null;
  balanceExpiresAt?: number | null;
  reversedAt?: number | null;
  reversedBy?: string;
  reversalReason?: string;
  credits?: RechargeCreditDetail[];
  refund?: AdminRefundRecord;
}

export interface PageAdminRechargeOrder {
  items: AdminRechargeOrder[];
  total: number;
  page: number;
  size: number;
}

export interface BalanceLedgerItem {
  txnId: string;
  txnType: string;
  currency: string;
  amountMicroUsd: number;
  balanceAfterMicroUsd: number;
  refType?: string;
  refId?: string;
  note?: string;
  createdAt: number;
}

export interface PageBalanceLedgerItem {
  items: BalanceLedgerItem[];
  total: number;
  page: number;
  size: number;
}

export interface WithdrawalItem {
  withdrawalId: string;
  currency: string;
  amountMicroUsd: number;
  feeAmountMicroUsd: number;
  payoutAmountMicroUsd: number;
  accountName: string;
  bankName: string;
  accountNo: string;
  status: string;
  applyNote?: string;
  reviewNote?: string;
  paymentRef?: string;
  paidAt?: number | null;
  createdAt: number;
}

export interface PageWithdrawalItem {
  items: WithdrawalItem[];
  total: number;
  page: number;
  size: number;
}
