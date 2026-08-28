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
  getAuthAuditLogs(params: {
    page?: number;
    size?: number;
    eventType?: string;
    principalType?: string;
    decision?: string;
    userId?: string;
  }) {
    return request()<PageAuditLogItem>({
      method: "GET",
      path: "/api/v1/auth-audit-logs",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },

  // ---- JWT 密钥 ----
  listJwtKeys() {
    return request()<{ keys: JwtKeyItem[]; total: number }>({
      method: "GET",
      path: "/api/v1/jwt-keys",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  rotateJwtKey() {
    return request()<{ message: string }>({
      method: "POST",
      path: "/api/v1/jwt-keys/rotate",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },


  // ---- 控制概览（数据分析）----
  getGlobalStats(params: { timeFrom?: number; timeTo?: number }) {
    return request()<GlobalStatsRow>({
      method: "GET",
      path: "/api/v1/analytics/global-stats",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  getConsumptionTrend(params: { timeFrom?: number; timeTo?: number; accountType?: string }) {
    return request()<ConsumptionTrendOutput>({
      method: "GET",
      path: "/api/v1/analytics/consumption-trend",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  getResourceStatistics(params: { timeFrom?: number; timeTo?: number }) {
    return request()<{ resources: ResourceStatItem[] }>({
      method: "GET",
      path: "/api/v1/analytics/resource-statistics",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  getDashboardAlerts() {
    return request()<DashboardAlertsOutput>({
      method: "GET",
      path: "/api/v1/dashboard/alerts",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  // ==================== 微信支付在线充值（管理端） ====================

  getPaymentSettings() {
    return request()<PaymentGlobalSettings>({
      method: "GET",
      path: "/api/v1/admin/payment-settings",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  updatePaymentSettings(body: PaymentGlobalSettings) {
    return request()<PaymentGlobalSettings>({
      method: "PUT",
      path: "/api/v1/admin/payment-settings",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  getWechatConfig() {
    return request()<WechatConfig>({
      method: "GET",
      path: "/api/v1/admin/wechat-config",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  updateWechatConfig(body: WechatConfigWriteInput) {
    return request()<WechatConfig>({
      method: "PUT",
      path: "/api/v1/admin/wechat-config",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  listPaymentOrders(params: { scene?: string; status?: string; tenantId?: string; page?: number; size?: number } = {}) {
    return request()<PagePaymentOrderItem>({
      method: "GET",
      path: "/api/v1/admin/payment-orders",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  syncPaymentOrder(orderId: string) {
    return request()<PaymentOrderItem>({
      method: "POST",
      path: `/api/v1/admin/payment-orders/${encodeURIComponent(orderId)}/sync`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  listAdminRechargeOrders(params: {
    keyword?: string;
    method?: "manual" | "online";
    targetType?: "tenant" | "user";
    paymentStatus?: string;
    fulfillmentStatus?: string;
    refundStatus?: string;
    timeFrom?: number;
    timeTo?: number;
    page?: number;
    size?: number;
  } = {}) {
    return request()<PageAdminRechargeOrder>({
      method: "GET",
      path: "/api/v1/admin/recharge-orders",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  getAdminRechargeOrder(orderId: string) {
    return request()<AdminRechargeOrder>({
      method: "GET",
      path: `/api/v1/admin/recharge-orders/${encodeURIComponent(orderId)}`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  syncAdminRechargeOrder(orderId: string) {
    return request()<AdminRechargeOrder>({
      method: "POST",
      path: `/api/v1/admin/recharge-orders/${encodeURIComponent(orderId)}/sync`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  reverseAdminRechargeOrderCredit(orderId: string, body: { reason: string }) {
    return request()<AdminRechargeOrder>({
      method: "POST",
      path: `/api/v1/admin/recharge-orders/${encodeURIComponent(orderId)}/reverse-credit`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  recordCompletedRechargeRefund(orderId: string, body: {
    method: "wechat" | "offline";
    refundReference: string;
    channelRefundId?: string;
    refundedAt: number;
    reason: string;
    note?: string;
  }) {
    return request()<AdminRechargeOrder>({
      method: "POST",
      path: `/api/v1/admin/recharge-orders/${encodeURIComponent(orderId)}/refund-reversal`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  listBalanceLedger(params: { tenantId: string; txnType?: string; page?: number; size?: number }) {
    return request()<PageBalanceLedgerItem>({
      method: "GET",
      path: "/api/v1/admin/balance-ledger",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  listWithdrawals(params: { status?: string; page?: number; size?: number } = {}) {
    return request()<PageWithdrawalItem>({
      method: "GET",
      path: "/api/v1/admin/withdrawals",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  createWithdrawal(body: {
    tenantId: string;
    amountMicroUsd: number;
    accountName?: string;
    bankName?: string;
    accountNo?: string;
    note?: string;
    paymentRef?: string;
  }) {
    return request()<WithdrawalItem>({
      method: "POST",
      path: "/api/v1/admin/withdrawals",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  }
};
