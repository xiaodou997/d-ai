import { authenticatedRequest, apiHeaders, apiBaseUrl } from "./request";
import {
  createTypedOperationRequest,
  type OperationBody,
  type OperationQuery,
  type OperationResponse
} from ".";
import type {
  AccountBalanceOutput,
  BatchOpResult,
  ConsumptionTrendOutput,
  DashboardAlertsOutput,
  GlobalStatsRow,
  PageBalanceLedgerItem,
  PagePaymentOrderItem,
  PageAdminRechargeOrder,
  AdminRechargeOrder,
  AdminRefundRecord,
  RechargeCreditDetail,
  BalanceLedgerItem,
  PageWithdrawalItem,
  WithdrawalItem,
  PaymentGlobalSettings,
  PaymentOrderItem,
  ResourceStatItem,
  JwtKeyItem,
  DebtStatusOutputBody,
  PageAuditLogItem,
  PageRechargeRecordItem,
  RechargeOutputBody,
  PageAdminUserItem,
  PageEndUserItem,
  PageTenantListItem,
  TopupPackage,
  ActivationCredentialOutput,
  CreateAdminUserOutput,
  TenantDetailOutput,
  WechatConfig,
  WechatConfigWriteInput
} from "./types/admin";
import type { ChangePasswordPayload } from "./types/auth";
import type { components } from "./generated/dai";

function request() {
  return authenticatedRequest();
}

const typedRequest = createTypedOperationRequest(request());

function stripSchema<T>(value: T): Omit<T, "$schema"> {
  const { $schema: _schema, ...rest } = value as T & { $schema?: string };
  return rest as Omit<T, "$schema">;
}

type AdminUserPageTransport = OperationResponse<"admin-list-system-admins">;
type AdminUserTransport = NonNullable<AdminUserPageTransport["items"]>[number];
type TenantPageTransport = OperationResponse<"admin-list-tenants">;
type TenantTransport = NonNullable<TenantPageTransport["items"]>[number];
type EndUserPageTransport = OperationResponse<"admin-list-end-users">;
type EndUserTransport = NonNullable<EndUserPageTransport["items"]>[number];
type AccountBalanceTransport = OperationResponse<"account-balance">;
type RechargeRecordsTransport = OperationResponse<"account-recharge-records">;
type RechargeTransport = OperationResponse<"admin-recharge">;
type ReverseRechargeTransport = OperationResponse<"admin-reverse-recharge">;
type BatchRefundTransport = OperationResponse<"admin-batch-refund-usage">;
type DebtTransport = OperationResponse<"admin-get-debt">;
type AccountBalanceLotTransport = components["schemas"]["AccountBalanceLot"];
type AccountBalanceServiceState = "active" | "blocked_debt";
type DebtOwnerType = "tenant" | "user";
type AuthAuditPageTransport = OperationResponse<"admin-auth-audit-logs">;
type JwtKeysTransport = OperationResponse<"list-jwt-keys">;
type GlobalStatsTransport = OperationResponse<"admin-global-stats">;
type ConsumptionTrendTransport = OperationResponse<"admin-consumption-trend">;
type ResourceStatisticsTransport = OperationResponse<"admin-resource-statistics">;
type DashboardAlertsTransport = OperationResponse<"admin-dashboard-alerts">;
type PaymentSettingsTransport = OperationResponse<"admin-get-payment-settings">;
type PaymentSettingsWriteBody = OperationBody<"admin-update-payment-settings">;
type WechatConfigTransport = OperationResponse<"admin-get-wechat-config">;
type WechatConfigWriteBody = OperationBody<"admin-update-wechat-config">;
type PaymentOrdersTransport = OperationResponse<"admin-list-payment-orders">;
type PaymentOrderTransport = NonNullable<PaymentOrdersTransport["items"]>[number];
type AdminRechargeOrdersTransport = OperationResponse<"admin-list-recharge-orders">;
type AdminRechargeOrderTransport = components["schemas"]["AdminRechargeOrder"];
type BalanceLedgerPageTransport = OperationResponse<"admin-list-balance-ledger">;
type BalanceLedgerTransport = components["schemas"]["CashLedgerItem"];
type ReverseAdminRechargeBody = OperationBody<"admin-reverse-recharge-order-credit">;
type RecordCompletedRefundBody = OperationBody<"admin-record-completed-recharge-refund">;
type WithdrawalPageTransport = OperationResponse<"admin-list-withdrawals">;
type WithdrawalTransport = components["schemas"]["WithdrawalItem"];

function toOperationStatus(value: { success: boolean }): { status: string } {
  return { status: value.success ? "success" : "failed" };
}

function toCredentialState(value: string): "active" | "pending_activation" {
  if (value === "active" || value === "pending_activation") return value;
  throw new Error(`Unexpected credential state: ${value}`);
}

function toAdminUser(value: AdminUserTransport): PageAdminUserItem["items"][number] {
  return {
    userId: value.userId,
    username: value.username,
    email: value.email,
    status: value.status,
    statusText: value.statusText,
    credentialState: toCredentialState(value.credentialState),
    createdTime: value.createdTime
  };
}

function toAdminUserPage(value: AdminUserPageTransport): PageAdminUserItem {
  return { items: value.items?.map(toAdminUser) ?? [], total: value.total, page: value.page, size: value.size };
}

function toTenant(value: TenantTransport): PageTenantListItem["items"][number] {
  return {
    tenantId: value.tenantId,
    tenantName: value.tenantName,
    contactPerson: value.contactPerson,
    contactEmail: value.contactEmail,
    status: value.status,
    statusDisplay: value.statusDisplay,
    balanceUsd: value.balanceUsd,
    userCount: value.userCount,
    createdTime: value.createdTime
  };
}

function toTenantPage(value: TenantPageTransport): PageTenantListItem {
  return { items: value.items?.map(toTenant) ?? [], total: value.total, page: value.page, size: value.size };
}

function toEndUser(value: EndUserTransport): PageEndUserItem["items"][number] {
  return {
    userId: value.userId,
    tenantId: value.tenantId,
    tenantName: value.tenantName,
    username: value.username,
    nickname: value.nickname,
    email: value.email,
    phone: value.phone,
    status: value.status,
    credentialState: toCredentialState(value.credentialState),
    balanceUsd: value.balanceUsd,
    createdTime: value.createdTime,
    lastLoginTime: value.lastLoginTime
  };
}

function toEndUserPage(value: EndUserPageTransport): PageEndUserItem {
  return { items: value.items?.map(toEndUser) ?? [], total: value.total, page: value.page, size: value.size };
}

function toCreateAdminUser(value: OperationResponse<"admin-create-system-admin">): CreateAdminUserOutput {
  return {
    userId: value.userId,
    username: value.username,
    activationToken: value.activationToken,
    activationExpiresIn: value.activationExpiresIn
  };
}

function toActivation(value: OperationResponse<"admin-reset-system-admin-password">): ActivationCredentialOutput {
  return { activationToken: value.activationToken, activationExpiresIn: value.activationExpiresIn };
}

function toMessage(value: { message: string }): { message: string } {
  return { message: value.message };
}

function toAccountBalanceServiceState(value: string): AccountBalanceServiceState {
  if (value === "active" || value === "blocked_debt") return value;
  throw new Error(`Unexpected account balance service state: ${value}`);
}

function toBalanceLot(value: AccountBalanceLotTransport): NonNullable<AccountBalanceOutput["balanceLots"]>[number] {
  return {
    balanceLotId: value.balanceLotId,
    totalUsd: value.totalUsd,
    remainingUsd: value.remainingUsd,
    createdAt: value.createdAt,
    expiresAt: value.expiresAt ?? null,
    source: value.source
  };
}

function toAccountBalance(value: AccountBalanceTransport): AccountBalanceOutput {
  return {
    currency: value.currency,
    totalUsd: value.totalUsd,
    usedUsd: value.usedUsd,
    remainingUsd: value.remainingUsd,
    availableUsd: value.availableUsd,
    permanentUsd: value.permanentUsd,
    timedUsd: value.timedUsd,
    outstandingDebtMicroUsd: value.outstandingDebtMicroUsd,
    serviceState: toAccountBalanceServiceState(value.serviceState),
    balanceLots: value.balanceLots?.map(toBalanceLot) ?? []
  };
}

function toRechargeRecord(value: components["schemas"]["RechargeRecordRow"]): PageRechargeRecordItem["items"][number] {
  return {
    orderId: value.orderId,
    orderType: value.orderType,
    paidAmountMinor: value.paidAmountMinor,
    amountUsd: value.amountUsd,
    status: value.status,
    note: value.note,
    userId: value.userId,
    username: value.username,
    tenantName: value.tenantName,
    createdTime: value.createdTime ?? undefined
  };
}

function toRechargeRecords(value: RechargeRecordsTransport): PageRechargeRecordItem {
  return {
    items: value.items?.map(toRechargeRecord) ?? [],
    total: value.total,
    page: value.page,
    size: value.size
  };
}

function toRecharge(value: RechargeTransport): RechargeOutputBody {
  return {
    orderId: value.orderId,
    balanceLotId: value.balanceLotId,
    tenantId: value.tenantId,
    userId: value.userId,
    currency: value.currency,
    amountMicroUsd: value.amountMicroUsd,
    paidAmountMinor: value.paidAmountMinor,
    clearedDebtUsd: value.clearedDebtUsd,
    balanceLotUsd: value.balanceLotUsd,
    orderTime: value.orderTime
  };
}

function toReverseRecharge(value: ReverseRechargeTransport): {
  status: string;
  orderId: string;
  balanceLotId: string;
  reversedAmountUsd: number;
  originalAmountUsd: number;
  lostAmountUsd: number;
  balanceLotStatus: string;
} {
  return {
    status: value.status,
    orderId: value.orderId,
    balanceLotId: value.balanceLotId,
    reversedAmountUsd: value.reversedAmountUsd,
    originalAmountUsd: value.originalAmountUsd,
    lostAmountUsd: value.lostAmountUsd,
    balanceLotStatus: value.balanceLotStatus
  };
}

function toBatchRefund(value: BatchRefundTransport): BatchOpResult {
  return {
    succeeded: value.succeeded ?? [],
    failed: value.failed?.map((item) => ({ requestId: item.requestId, reason: item.reason })) ?? [],
    totalTenantUsd: value.totalTenantUsd,
    totalUserUsd: value.totalUserUsd,
    successCount: value.successCount,
    failCount: value.failCount
  };
}

function toDebtOwnerType(value: string): DebtOwnerType {
  if (value === "tenant" || value === "user") return value;
  throw new Error(`Unexpected debt owner type: ${value}`);
}

function toDebtServiceState(value: string): DebtStatusOutputBody["service_state"] {
  if (value === "active" || value === "blocked_debt") return value;
  throw new Error(`Unexpected debt service state: ${value}`);
}

function toDebtStatus(value: DebtTransport): DebtStatusOutputBody {
  return {
    owner_type: toDebtOwnerType(value.owner_type),
    account_id: value.account_id,
    outstanding_debt_micro_usd: value.outstanding_debt_micro_usd,
    service_state: toDebtServiceState(value.service_state)
  };
}

function toAuthAuditPage(value: AuthAuditPageTransport): PageAuditLogItem {
  return {
    items: value.items?.map((item) => ({
      id: item.id,
      eventType: item.eventType,
      principalType: item.principalType,
      decision: item.decision,
      userId: item.userId,
      createdAt: item.createdAt,
      reasonCode: item.reasonCode,
      reasonMessage: item.reasonMessage
    })) ?? [],
    total: value.total,
    page: value.page,
    size: value.size
  };
}

function toJwtKeys(value: JwtKeysTransport): { keys: JwtKeyItem[]; total: number } {
  return {
    keys: value.keys?.map((item) => ({
      id: item.id,
      kid: item.kid,
      status: item.status,
      createdTime: item.createdTime,
      graceUntil: item.graceUntil,
      retiredTime: item.retiredTime
    })) ?? [],
    total: value.total
  };
}

function toGlobalStats(value: GlobalStatsTransport): GlobalStatsRow {
  return {
    currency: value.currency,
    tenantRechargePaidMinor: value.tenantRechargePaidMinor,
    tenantRechargeAmountUsd: value.tenantRechargeAmountUsd,
    activeTenants: value.activeTenants,
    tenantTotalBalanceUsd: value.tenantTotalBalanceUsd,
    userRechargePaidMinor: value.userRechargePaidMinor,
    userRechargeAmountUsd: value.userRechargeAmountUsd,
    newUsers: value.newUsers,
    userTotalBalanceUsd: value.userTotalBalanceUsd
  };
}

function toConsumptionTrend(value: ConsumptionTrendTransport): ConsumptionTrendOutput {
  return {
    totalUsd: value.totalUsd,
    dataPoints: value.dataPoints?.map((point) => ({ timeLabel: point.timeLabel, amountUsd: point.amountUsd })) ?? []
  };
}

function toResourceStatistics(value: ResourceStatisticsTransport): { resources: ResourceStatItem[] } {
  return {
    resources: value.resources?.map((resource) => ({
      clientName: resource.clientName,
      clientId: resource.clientId,
      amountUsd: resource.amountUsd,
      percentage: resource.percentage
    })) ?? []
  };
}

function toDashboardAlerts(value: DashboardAlertsTransport): DashboardAlertsOutput {
  return {
    failedTransactions: value.failedTransactions?.map((item) => ({
      requestId: item.requestId,
      settlementError: item.settlementError,
      status: item.status,
      createdTime: item.createdTime
    })) ?? []
  };
}

function toTopupPackage(value: components["schemas"]["TopupPackage"]): TopupPackage {
  return {
    id: value.id,
    name: value.name,
    paymentAmountMicroUsd: value.paymentAmountMicroUsd,
    giftAmountMicroUsd: value.giftAmountMicroUsd,
    validityDays: value.validityDays ?? null,
    badge: value.badge,
    enabled: value.enabled,
    sortOrder: value.sortOrder
  };
}

function toPaymentSettings(value: PaymentSettingsTransport): PaymentGlobalSettings {
  return {
    tenantCustomTopupFeeBp: value.tenantCustomTopupFeeBp,
    tenantWithdrawFeeBp: value.tenantWithdrawFeeBp,
    tenantCustomValidityDays: value.tenantCustomValidityDays ?? null,
    tenantTopupPackages: value.tenantTopupPackages?.map(toTopupPackage) ?? []
  };
}

function toPaymentSettingsBody(value: PaymentGlobalSettings): PaymentSettingsWriteBody {
  return {
    tenantCustomTopupFeeBp: value.tenantCustomTopupFeeBp,
    tenantWithdrawFeeBp: value.tenantWithdrawFeeBp,
    tenantCustomValidityDays: value.tenantCustomValidityDays ?? undefined,
    tenantTopupPackages: value.tenantTopupPackages.map((item) => ({
      id: item.id,
      name: item.name,
      paymentAmountMicroUsd: item.paymentAmountMicroUsd,
      giftAmountMicroUsd: item.giftAmountMicroUsd,
      validityDays: item.validityDays ?? undefined,
      badge: item.badge,
      enabled: item.enabled,
      sortOrder: item.sortOrder
    }))
  };
}

function toWechatVerifyMode(value: string): WechatConfig["verifyMode"] {
  if (value === "platform_cert" || value === "public_key") return value;
  throw new Error(`Unexpected WeChat verify mode: ${value}`);
}

function toWechatConfig(value: WechatConfigTransport): WechatConfig {
  return {
    enabled: value.enabled,
    mock: value.mock,
    verifyMode: toWechatVerifyMode(value.verifyMode),
    appId: value.appId,
    mchId: value.mchId,
    mchCertSerialNo: value.mchCertSerialNo,
    notifyBaseUrl: value.notifyBaseUrl,
    orderTtlSeconds: value.orderTtlSeconds,
    hasPrivateKey: value.hasPrivateKey,
    hasApiv3Key: value.hasApiv3Key,
    wechatPayPublicKeyId: value.wechatPayPublicKeyId,
    hasWechatPayPublicKey: value.hasWechatPayPublicKey
  };
}

function toWechatConfigBody(value: WechatConfigWriteInput): WechatConfigWriteBody {
  return {
    enabled: value.enabled,
    mock: value.mock,
    verifyMode: toWechatVerifyMode(value.verifyMode),
    appId: value.appId,
    mchId: value.mchId,
    mchCertSerialNo: value.mchCertSerialNo,
    notifyBaseUrl: value.notifyBaseUrl,
    orderTtlSeconds: value.orderTtlSeconds,
    mchPrivateKey: value.mchPrivateKey ?? null,
    apiv3Key: value.apiv3Key ?? null,
    wechatPayPublicKeyId: value.wechatPayPublicKeyId ?? null,
    wechatPayPublicKey: value.wechatPayPublicKey ?? null
  };
}

function toPaymentOrderScene(value: string): PaymentOrderItem["scene"] {
  if (value === "user_topup" || value === "tenant_topup") return value;
  throw new Error(`Unexpected payment order scene: ${value}`);
}

function toPaymentOrderMode(value: string): PaymentOrderItem["topupMode"] {
  if (value === "custom" || value === "package") return value;
  throw new Error(`Unexpected payment order topup mode: ${value}`);
}

function toPaymentOrder(value: PaymentOrderTransport): PaymentOrderItem {
  return {
    orderId: value.orderId,
    scene: toPaymentOrderScene(value.scene),
    tenantName: value.tenantName,
    username: value.username,
    status: value.status,
    paymentCurrency: value.paymentCurrency,
    paymentAmountMinor: value.paymentAmountMinor,
    grossAmountMicroUsd: value.grossAmountMicroUsd,
    feeAmountMicroUsd: value.feeAmountMicroUsd,
    giftAmountMicroUsd: value.giftAmountMicroUsd,
    creditedAmountMicroUsd: value.creditedAmountMicroUsd,
    topupMode: toPaymentOrderMode(value.topupMode),
    packageName: value.packageName,
    transactionId: value.transactionId,
    createdAt: value.createdAt,
    paidAt: value.paidAt ?? null,
    balanceExpiresAt: value.balanceExpiresAt ?? null
  };
}

function toPaymentOrders(value: PaymentOrdersTransport): PagePaymentOrderItem {
  return { items: value.items?.map(toPaymentOrder) ?? [], total: value.total, page: value.page, size: value.size };
}

function toRechargeMethod(value: string): AdminRechargeOrder["method"] {
  if (value === "manual" || value === "online") return value;
  throw new Error(`Unexpected recharge method: ${value}`);
}

function toRechargeTargetType(value: string): AdminRechargeOrder["targetType"] {
  if (value === "tenant" || value === "user") return value;
  throw new Error(`Unexpected recharge target type: ${value}`);
}

function toRechargePaymentStatus(value: string): AdminRechargeOrder["paymentStatus"] {
  if (value === "not_required" || value === "created" || value === "paying" || value === "paid" || value === "closed" || value === "expired") return value;
  throw new Error(`Unexpected recharge payment status: ${value}`);
}

function toRechargeFulfillmentStatus(value: string): AdminRechargeOrder["fulfillmentStatus"] {
  if (value === "pending" || value === "credited" || value === "partially_reversed" || value === "reversed") return value;
  throw new Error(`Unexpected recharge fulfillment status: ${value}`);
}

function toRechargeRefundStatus(value: string): AdminRechargeOrder["refundStatus"] {
  if (value === "none" || value === "refunded" || value === "not_applicable") return value;
  throw new Error(`Unexpected recharge refund status: ${value}`);
}

function toRechargeCredit(value: components["schemas"]["RechargeCreditDetail"]): RechargeCreditDetail {
  return {
    balanceOrderId: value.balanceOrderId,
    orderType: value.orderType,
    primary: value.primary,
    creditAmountMicroUsd: value.creditAmountMicroUsd,
    status: value.status,
    note: value.note,
    balanceExpiresAt: value.balanceExpiresAt ?? null,
    reversedAt: value.reversedAt ?? null,
    reversedBy: value.reversedBy,
    reversalReason: value.reversalReason,
    reversedAmountMicroUsd: value.reversedAmountMicroUsd,
    lostAmountMicroUsd: value.lostAmountMicroUsd,
    lotId: value.lotId,
    grantedAmountMicroUsd: value.grantedAmountMicroUsd,
    consumedAmountMicroUsd: value.consumedAmountMicroUsd,
    remainingAmountMicroUsd: value.remainingAmountMicroUsd,
    lotStatus: value.lotStatus,
    refundId: value.refundId,
    refundAvailableMicroUsd: value.refundAvailableMicroUsd,
    refundNonAvailableMicroUsd: value.refundNonAvailableMicroUsd,
    refundExpiredMicroUsd: value.refundExpiredMicroUsd,
    refundAccountDebitMicroUsd: value.refundAccountDebitMicroUsd,
    refundBalanceAfterMicroUsd: value.refundBalanceAfterMicroUsd
  };
}

function toAdminRefund(value: components["schemas"]["AdminRefundRecord"]): AdminRefundRecord {
  if (value.method !== "wechat" && value.method !== "offline") throw new Error(`Unexpected refund method: ${value.method}`);
  if (value.status !== "completed") throw new Error(`Unexpected refund status: ${value.status}`);
  return {
    refundId: value.refundId,
    method: value.method,
    refundReference: value.refundReference,
    channelRefundId: value.channelRefundId,
    refundAmountMinor: value.refundAmountMinor,
    status: value.status,
    refundedAt: value.refundedAt,
    reason: value.reason,
    note: value.note,
    operatorId: value.operatorId,
    createdAt: value.createdAt
  };
}

function toAdminRechargeOrder(value: AdminRechargeOrderTransport): AdminRechargeOrder {
  return {
    orderId: value.orderId,
    balanceOrderId: value.balanceOrderId,
    method: toRechargeMethod(value.method),
    targetType: toRechargeTargetType(value.targetType),
    orderType: value.orderType,
    tenantId: value.tenantId,
    tenantName: value.tenantName,
    userId: value.userId,
    username: value.username,
    paidAmountMinor: value.paidAmountMinor,
    grossAmountMicroUsd: value.grossAmountMicroUsd,
    feeAmountMicroUsd: value.feeAmountMicroUsd,
    giftAmountMicroUsd: value.giftAmountMicroUsd,
    creditedAmountMicroUsd: value.creditedAmountMicroUsd,
    tenantIncomeMicroUsd: value.tenantIncomeMicroUsd,
    paymentStatus: toRechargePaymentStatus(value.paymentStatus),
    fulfillmentStatus: toRechargeFulfillmentStatus(value.fulfillmentStatus),
    refundStatus: toRechargeRefundStatus(value.refundStatus),
    outTradeNo: value.outTradeNo,
    transactionId: value.transactionId,
    topupMode: value.topupMode,
    packageName: value.packageName,
    channel: value.channel,
    note: value.note,
    failNote: value.failNote,
    createdAt: value.createdAt,
    paidAt: value.paidAt ?? null,
    paymentExpiresAt: value.paymentExpiresAt ?? null,
    balanceExpiresAt: value.balanceExpiresAt ?? null,
    reversedAt: value.reversedAt ?? null,
    reversedBy: value.reversedBy,
    reversalReason: value.reversalReason,
    credits: value.credits?.map(toRechargeCredit),
    refund: value.refund ? toAdminRefund(value.refund) : undefined
  };
}

function toAdminRechargeOrders(value: AdminRechargeOrdersTransport): PageAdminRechargeOrder {
  return { items: value.items?.map(toAdminRechargeOrder) ?? [], total: value.total, page: value.page, size: value.size };
}

function toBalanceLedger(value: BalanceLedgerTransport): BalanceLedgerItem {
  return {
    txnId: value.txnId,
    txnType: value.txnType,
    currency: value.currency,
    amountMicroUsd: value.amountMicroUsd,
    balanceAfterMicroUsd: value.balanceAfterMicroUsd,
    refType: value.refType,
    refId: value.refId,
    note: value.note,
    createdAt: value.createdAt
  };
}

function toBalanceLedgerPage(value: BalanceLedgerPageTransport): PageBalanceLedgerItem {
  return { items: value.items?.map(toBalanceLedger) ?? [], total: value.total, page: value.page, size: value.size };
}

function toWithdrawal(value: WithdrawalTransport): WithdrawalItem {
  return {
    withdrawalId: value.withdrawalId,
    currency: value.currency,
    amountMicroUsd: value.amountMicroUsd,
    feeAmountMicroUsd: value.feeAmountMicroUsd,
    payoutAmountMicroUsd: value.payoutAmountMicroUsd,
    accountName: value.accountName,
    bankName: value.bankName,
    accountNo: value.accountNo,
    status: value.status,
    applyNote: value.applyNote,
    reviewNote: value.reviewNote,
    paymentRef: value.paymentRef,
    paidAt: value.paidAt ?? null,
    createdAt: value.createdAt
  };
}

function toWithdrawals(value: WithdrawalPageTransport): PageWithdrawalItem {
  return { items: value.items?.map(toWithdrawal) ?? [], total: value.total, page: value.page, size: value.size };
}

export const platformAdminApi = {
  // ---- 账号自助 ----
  // 修改本人密码（后端统一密码策略）
  changePassword(body: ChangePasswordPayload) {
    return typedRequest<"auth-change-password">({
      method: "PUT",
      path: "/api/auth/password",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },

  // ---- 系统管理员 ----
  listSystemAdmins(params: { page?: number; size?: number; keyword?: string }) {
    return typedRequest<"admin-list-system-admins">({
      method: "GET",
      path: "/api/v1/system-admins",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    }).then(toAdminUserPage);
  },
  createSystemAdmin(body: OperationBody<"admin-create-system-admin">) {
    return typedRequest<"admin-create-system-admin">({
      method: "POST",
      path: "/api/v1/system-admins",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toCreateAdminUser);
  },
  updateSystemAdmin(id: string, body: OperationBody<"admin-update-system-admin">) {
    return typedRequest<"admin-update-system-admin">({
      method: "PUT",
      path: `/api/v1/system-admins/${encodeURIComponent(id)}`,
      pathParams: { id },
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toOperationStatus);
  },
  deleteSystemAdmin(id: string) {
    return typedRequest<"admin-delete-system-admin">({
      method: "DELETE",
      path: `/api/v1/system-admins/${encodeURIComponent(id)}`,
      pathParams: { id },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toOperationStatus);
  },
  resetSystemAdminPassword(id: string) {
    return typedRequest<"admin-reset-system-admin-password">({
      method: "POST",
      path: `/api/v1/system-admins/${encodeURIComponent(id)}/reset-password`,
      pathParams: { id },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toActivation);
  },

  // ---- 租户 ----
  listTenants(params: { page?: number; size?: number; keyword?: string; status?: number }) {
    return typedRequest<"admin-list-tenants">({
      method: "GET",
      path: "/api/v1/tenants",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    }).then(toTenantPage);
  },
  getAccountBalance(params: OperationQuery<"account-balance">) {
    return typedRequest<"account-balance">({
      method: "GET",
      path: "/api/v1/account/balance",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    }).then(toAccountBalance);
  },
  getTenant(id: string) {
    return typedRequest<"admin-get-tenant">({
      method: "GET",
      path: `/api/v1/tenants/${encodeURIComponent(id)}`,
      pathParams: { id },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then((value): TenantDetailOutput => ({
      tenantId: value.tenantId,
      tenantName: value.tenantName,
      contactPerson: value.contactPerson,
      contactEmail: value.contactEmail,
      status: value.status,
      statusDisplay: value.statusDisplay,
      createdTime: value.createdTime,
      isWildcard: false,
      clientIds: []
    }));
  },
  createTenant(body: OperationBody<"admin-create-tenant">) {
    return typedRequest<"admin-create-tenant">({
      method: "POST",
      path: "/api/v1/tenants",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then((value) => ({
      tenantId: value.tenantId,
      initUserId: value.initUserId,
      initUsername: value.initUsername,
      activationToken: value.activationToken,
      activationExpiresIn: value.activationExpiresIn
    }));
  },
  updateTenant(
    id: string,
    body: OperationBody<"admin-update-tenant">
  ) {
    return typedRequest<"admin-update-tenant">({
      method: "PUT",
      path: `/api/v1/tenants/${encodeURIComponent(id)}`,
      pathParams: { id },
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toOperationStatus);
  },
  deleteTenant(id: string) {
    return typedRequest<"admin-delete-tenant">({
      method: "DELETE",
      path: `/api/v1/tenants/${encodeURIComponent(id)}`,
      pathParams: { id },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toOperationStatus);
  },
  updateTenantStatus(id: string, status: OperationBody<"admin-update-tenant-status">["status"]) {
    return typedRequest<"admin-update-tenant-status">({
      method: "PATCH",
      path: `/api/v1/tenants/${encodeURIComponent(id)}/status`,
      pathParams: { id },
      headers: apiHeaders,
      body: { status },
      baseUrl: apiBaseUrl
    }).then(toOperationStatus);
  },

  // ---- 租户用户 ----
  listTenantUsers(params: { page?: number; size?: number; tenantId?: string; keyword?: string }) {
    return typedRequest<"admin-list-tenant-users">({
      method: "GET",
      path: "/api/v1/tenant-users",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    }).then(toAdminUserPage);
  },
  createTenantUser(body: OperationBody<"admin-create-tenant-user">) {
    return typedRequest<"admin-create-tenant-user">({
      method: "POST",
      path: "/api/v1/tenant-users",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then((value): CreateAdminUserOutput => ({
      userId: value.userId,
      username: value.username,
      activationToken: value.activationToken,
      activationExpiresIn: value.activationExpiresIn
    }));
  },
  updateTenantUserStatus(id: string, status: OperationBody<"admin-update-tenant-user-status">["status"]) {
    return typedRequest<"admin-update-tenant-user-status">({
      method: "PATCH",
      path: `/api/v1/tenant-users/${encodeURIComponent(id)}/status`,
      pathParams: { id },
      headers: apiHeaders,
      body: { status },
      baseUrl: apiBaseUrl
    }).then(toOperationStatus);
  },
  updateTenantUser(id: string, body: OperationBody<"admin-update-tenant-user">) {
    return typedRequest<"admin-update-tenant-user">({
      method: "PUT",
      path: `/api/v1/tenant-users/${encodeURIComponent(id)}`,
      pathParams: { id },
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toOperationStatus);
  },
  resetTenantUserPassword(id: string) {
    return typedRequest<"admin-reset-tenant-user-password">({
      method: "POST",
      path: `/api/v1/tenant-users/${encodeURIComponent(id)}/reset-password`,
      pathParams: { id },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then((value) => ({ activationToken: value.activationToken, activationExpiresIn: value.activationExpiresIn }));
  },

  // ---- 终端用户 ----
  listEndUsers(params: {
    page?: number;
    size?: number;
    tenantId?: string;
    keyword?: string;
    tenantName?: string;
    username?: string;
    status?: number;
  }) {
    return typedRequest<"admin-list-end-users">({
      method: "GET",
      path: "/api/v1/users",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    }).then(toEndUserPage);
  },
  createEndUser(body: OperationBody<"admin-create-end-user">) {
    return typedRequest<"admin-create-end-user">({
      method: "POST",
      path: "/api/v1/users",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then((value): CreateAdminUserOutput => ({
      userId: value.userId,
      username: value.username,
      activationToken: value.activationToken,
      activationExpiresIn: value.activationExpiresIn
    }));
  },
  updateEndUserStatus(id: string, status: OperationBody<"admin-update-end-user-status">["status"]) {
    return typedRequest<"admin-update-end-user-status">({
      method: "PATCH",
      path: `/api/v1/users/${encodeURIComponent(id)}/status`,
      pathParams: { id },
      headers: apiHeaders,
      body: { status },
      baseUrl: apiBaseUrl
    }).then(toMessage);
  },
  resetEndUserPassword(id: string) {
    return typedRequest<"admin-reset-end-user-password">({
      method: "POST",
      path: `/api/v1/users/${encodeURIComponent(id)}/reset-password`,
      pathParams: { id },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then((value) => ({ activationToken: value.activationToken, activationExpiresIn: value.activationExpiresIn }));
  },

  listRechargeRecords(params: OperationQuery<"account-recharge-records">) {
    return typedRequest<"account-recharge-records">({
      method: "GET",
      path: "/api/v1/account/recharge-records",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    }).then(toRechargeRecords);
  },
  createRecharge(body: OperationBody<"admin-recharge">) {
    return typedRequest<"admin-recharge">({
      method: "POST",
      path: "/api/v1/recharges",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toRecharge);
  },
  reverseRecharge(orderId: string, body: OperationBody<"admin-reverse-recharge">) {
    return typedRequest<"admin-reverse-recharge">({
      method: "POST",
      path: `/api/v1/recharges/${encodeURIComponent(orderId)}/reverse`,
      pathParams: { orderId },
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toReverseRecharge);
  },
  // ---- AI 使用记录人工退款 ----
  refundUsage(body: OperationBody<"admin-refund-usage">) {
    return typedRequest<"admin-refund-usage">({
      method: "POST",
      path: "/api/v1/ai/usage/refund",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toMessage);
  },
  batchRefundUsage(body: OperationBody<"admin-batch-refund-usage">) {
    return typedRequest<"admin-batch-refund-usage">({
      method: "POST",
      path: "/api/v1/ai/usage/batch-refund",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toBatchRefund);
  },
  getDebtStatus(ownerType: "tenant" | "user", accountId: string) {
	return typedRequest<"admin-get-debt">({
	  method: "GET",
	  path: `/api/v1/admin/debts/${ownerType}/${encodeURIComponent(accountId)}`,
	  pathParams: { owner_type: ownerType, id: accountId },
	  headers: apiHeaders,
	  baseUrl: apiBaseUrl
	}).then(toDebtStatus);
	},

  // ---- 认证审计 ----
  getAuthAuditLogs(params: OperationQuery<"admin-auth-audit-logs">) {
    return typedRequest<"admin-auth-audit-logs">({
      method: "GET",
      path: "/api/v1/auth-audit-logs",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    }).then(toAuthAuditPage);
  },

  // ---- JWT 密钥 ----
  listJwtKeys() {
    return typedRequest<"list-jwt-keys">({
      method: "GET",
      path: "/api/v1/jwt-keys",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toJwtKeys);
  },
  rotateJwtKey() {
    return typedRequest<"rotate-jwt-key">({
      method: "POST",
      path: "/api/v1/jwt-keys/rotate",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toMessage);
  },


  // ---- 控制概览（数据分析）----
  getGlobalStats(params: OperationQuery<"admin-global-stats">) {
    return typedRequest<"admin-global-stats">({
      method: "GET",
      path: "/api/v1/analytics/global-stats",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    }).then(toGlobalStats);
  },
  getConsumptionTrend(params: OperationQuery<"admin-consumption-trend">) {
    return typedRequest<"admin-consumption-trend">({
      method: "GET",
      path: "/api/v1/analytics/consumption-trend",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    }).then(toConsumptionTrend);
  },
  getResourceStatistics(params: OperationQuery<"admin-resource-statistics">) {
    return typedRequest<"admin-resource-statistics">({
      method: "GET",
      path: "/api/v1/analytics/resource-statistics",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    }).then(toResourceStatistics);
  },
  getDashboardAlerts() {
    return typedRequest<"admin-dashboard-alerts">({
      method: "GET",
      path: "/api/v1/dashboard/alerts",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toDashboardAlerts);
  },
  // ==================== 微信支付在线充值（管理端） ====================

  getPaymentSettings() {
    return typedRequest<"admin-get-payment-settings">({
      method: "GET",
      path: "/api/v1/admin/payment-settings",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toPaymentSettings);
  },
  updatePaymentSettings(body: PaymentGlobalSettings) {
    return typedRequest<"admin-update-payment-settings">({
      method: "PUT",
      path: "/api/v1/admin/payment-settings",
      headers: apiHeaders,
      body: toPaymentSettingsBody(body),
      baseUrl: apiBaseUrl
    }).then(toPaymentSettings);
  },
  getWechatConfig() {
    return typedRequest<"admin-get-wechat-config">({
      method: "GET",
      path: "/api/v1/admin/wechat-config",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toWechatConfig);
  },
  updateWechatConfig(body: WechatConfigWriteInput) {
    return typedRequest<"admin-update-wechat-config">({
      method: "PUT",
      path: "/api/v1/admin/wechat-config",
      headers: apiHeaders,
      body: toWechatConfigBody(body),
      baseUrl: apiBaseUrl
    }).then(toWechatConfig);
  },
  listPaymentOrders(params: OperationQuery<"admin-list-payment-orders"> = {}) {
    return typedRequest<"admin-list-payment-orders">({
      method: "GET",
      path: "/api/v1/admin/payment-orders",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    }).then(toPaymentOrders);
  },
  syncPaymentOrder(orderId: string) {
    return typedRequest<"admin-sync-payment-order">({
      method: "POST",
      path: `/api/v1/admin/payment-orders/${encodeURIComponent(orderId)}/sync`,
      pathParams: { orderId },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(stripSchema);
  },
  listAdminRechargeOrders(params: OperationQuery<"admin-list-recharge-orders"> = {}) {
    return typedRequest<"admin-list-recharge-orders">({
      method: "GET",
      path: "/api/v1/admin/recharge-orders",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    }).then(toAdminRechargeOrders);
  },
  getAdminRechargeOrder(orderId: string) {
    return typedRequest<"admin-get-recharge-order">({
      method: "GET",
      path: `/api/v1/admin/recharge-orders/${encodeURIComponent(orderId)}`,
      pathParams: { orderId },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toAdminRechargeOrder);
  },
  syncAdminRechargeOrder(orderId: string) {
    return typedRequest<"admin-sync-recharge-order">({
      method: "POST",
      path: `/api/v1/admin/recharge-orders/${encodeURIComponent(orderId)}/sync`,
      pathParams: { orderId },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toAdminRechargeOrder);
  },
  reverseAdminRechargeOrderCredit(orderId: string, body: ReverseAdminRechargeBody) {
    return typedRequest<"admin-reverse-recharge-order-credit">({
      method: "POST",
      path: `/api/v1/admin/recharge-orders/${encodeURIComponent(orderId)}/reverse-credit`,
      pathParams: { orderId },
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toAdminRechargeOrder);
  },
  recordCompletedRechargeRefund(orderId: string, body: RecordCompletedRefundBody) {
    return typedRequest<"admin-record-completed-recharge-refund">({
      method: "POST",
      path: `/api/v1/admin/recharge-orders/${encodeURIComponent(orderId)}/refund-reversal`,
      pathParams: { orderId },
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toAdminRechargeOrder);
  },
  listBalanceLedger(params: OperationQuery<"admin-list-balance-ledger">) {
    return typedRequest<"admin-list-balance-ledger">({
      method: "GET",
      path: "/api/v1/admin/balance-ledger",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    }).then(toBalanceLedgerPage);
  },
  listWithdrawals(params: OperationQuery<"admin-list-withdrawals"> = {}) {
    return typedRequest<"admin-list-withdrawals">({
      method: "GET",
      path: "/api/v1/admin/withdrawals",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    }).then(toWithdrawals);
  },
  createWithdrawal(body: OperationBody<"admin-create-withdrawal">) {
    return typedRequest<"admin-create-withdrawal">({
      method: "POST",
      path: "/api/v1/admin/withdrawals",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toWithdrawal);
  }
};
