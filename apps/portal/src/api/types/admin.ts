export interface GlobalStatsRow {
  activeTenants: number;
  newUsers: number;
  tenantRechargeAmount: number;
  tenantRechargeCredits: number;
  tenantTotalCredits: number;
  userRechargeAmount: number;
  userRechargeCredits: number;
  userTotalCredits: number;
}

export interface AuditLogItem {
  id: number;
  eventType: string;
  principalType: string;
  decision: string;
  userId?: string;
  scopes?: string[];
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
  eventId: string;
  status: string;
  createdTime: number;
  terminalNote?: string;
}

export interface DashboardAlertsOutputBody {
  timeoutPreAuths: DashboardAlertItem[];
  failedTransactions: DashboardAlertItem[];
}

export interface DashboardSummaryDTO {
  total_requests: number;
  successful_requests: number;
  failed_requests: number;
  total_tokens: number;
  total_prompt_tokens: number;
  total_completion_tokens: number;
  total_catalog_base_credits: number;
  total_tenant_payable_credits: number;
  total_retail_base_credits: number;
  total_user_payable_credits: number;
  total_user_charged_credits: number;
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
  credits?: number;
  userCount?: number;
  createdTime?: number;
}

export interface AccountBalanceOutput {
  totalCredits: number;
  usedCredits: number;
  remainingCredits: number;
  frozenCredits: number;
  availableCredits: number;
  permanentCredits: number;
	timedCredits: number;
	outstandingDebtMicro: number;
	serviceState: "active" | "blocked_debt";
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
  credits?: number;
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
  total_tenant_payable_credits: number;
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

export interface CreditsPerUSDOutputBody {
  credits_per_usd: number;
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
  paidAmount: number;
  creditAmount: number;
  status: string;
  note: string;
  userId: string;
  username: string;
  tenantName: string;
  createdTime?: number;
}

export interface RechargeOutputBody {
  orderId: string;
  packageId: string;
  tenantId: string;
  userId: string;
  creditAmount: number;
  paidAmount: number;
  clearedOverdraft: number;
  packageCredits: number;
  orderTime: number;
}

export interface TransactionItem {
  eventId: string;
  userId: string;
  description: string;
  tenantCredits: number;
  userCredits: number;
  status: string;
  terminalNote: string;
  metadata: string;
  createdTime?: number;
  finishedTime?: number;
  username: string;
  tenantName: string;
  clientId: string;
}

export interface PageRechargeRecordItem {
  items: RechargeRecordItem[];
  total: number;
  page: number;
  size: number;
}

export interface PageTransactionItem {
  items: TransactionItem[];
  total: number;
  page: number;
  size: number;
}

export interface DebtStatusOutputBody {
  owner_type: "tenant" | "user";
  account_id: string;
  outstanding_debt_micro: number;
  service_state: "active" | "blocked_debt";
}

// ---- 新增类型 ----

export interface CreateAdminUserOutput {
  userId: string;
  username: string;
  defaultPassword: boolean;
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
  tenantRechargeAmount: number;
  tenantRechargeCredits: number;
  activeTenants: number;
  tenantTotalCredits: number;
  userRechargeAmount: number;
  userRechargeCredits: number;
  newUsers: number;
  userTotalCredits: number;
}
export interface TrendPoint {
  timeLabel: string;
  credits: number;
}
export interface ConsumptionTrendOutput {
  totalCredits: number;
  dataPoints: TrendPoint[];
}
export interface ResourceStatItem {
  clientName: string;
  clientId: string;
  credits: number;
  percentage: string;
}
export interface PreAuthAlert {
  eventId: string;
  tenantId: string;
  userId: string;
  createdTime: number;
}
export interface FailedTxAlert {
  eventId: string;
  terminalNote: string;
  status: string;
  createdTime: number;
}
export interface DashboardAlertsOutput {
  timeoutPreAuths: PreAuthAlert[];
  failedTransactions: FailedTxAlert[];
}

// ---- 批量计费事件操作结果（batch-confirm / batch-refund）----
export interface BatchOpError {
  eventId: string;
  reason: string;
}
export interface BatchOpResult {
  succeeded: string[];
  failed: BatchOpError[];
  totalTenantCredits: number;
  totalUserCredits: number;
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
  creditsPerCny: number;
  tenantCustomTopupFeeBp: number;
  tenantWithdrawFeeBp: number;
  tenantTopupPackages: TopupPackage[];
}

export interface TopupPackage {
  id: string;
  name: string;
  amount: number;
  credits: number;
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
  status: string;
  amount: number;
  creditAmount: number;
  grossCredits: number;
  feeCredits: number;
  topupMode: "custom" | "package";
  packageName?: string;
  transactionId?: string;
  createdAt: number;
  paidAt?: number | null;
}

export interface PagePaymentOrderItem {
  items: PaymentOrderItem[];
  total: number;
  page: number;
  size: number;
}

export interface CashAccountItem {
  tenantId: string;
  tenantName?: string;
  balance: number;
  frozen: number;
  available: number;
}

export interface PageCashAccountItem {
  items: CashAccountItem[];
  total: number;
  page: number;
  size: number;
}

export interface CashLedgerItem {
  txnId: string;
  txnType: string;
  amount: number;
  balanceAfter: number;
  refType?: string;
  refId?: string;
  note?: string;
  createdAt: number;
}

export interface PageCashLedgerItem {
  items: CashLedgerItem[];
  total: number;
  page: number;
  size: number;
}

export interface WithdrawalItem {
  withdrawalId: string;
  amount: number;
  feeAmount: number;
  payoutAmount: number;
  accountName: string;
  bankName: string;
  accountNo: string;
  status: string;
  applyNote?: string;
  reviewNote?: string;
  createdAt: number;
}

export interface PageWithdrawalItem {
  items: WithdrawalItem[];
  total: number;
  page: number;
  size: number;
}
