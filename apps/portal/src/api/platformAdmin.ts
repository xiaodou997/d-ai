import { authenticatedRequest, apiHeaders, apiBaseUrl } from "./request";
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

function request() {
  return authenticatedRequest();
}

export const platformAdminApi = {
  // ---- 账号自助 ----
  // 修改本人密码（后端统一密码策略）
  changePassword(body: { oldPassword: string; newPassword: string }) {
    return request()<{ message: string }>({
      method: "PUT",
      path: "/api/auth/password",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },

  // ---- 系统管理员 ----
  listSystemAdmins(params: { page?: number; size?: number; keyword?: string }) {
    return request()<PageAdminUserItem>({
      method: "GET",
      path: "/api/v1/system-admins",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  createSystemAdmin(body: { username: string; email?: string }) {
    return request()<CreateAdminUserOutput>({
      method: "POST",
      path: "/api/v1/system-admins",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  updateSystemAdmin(id: string, body: { email?: string; status?: number }) {
    return request()<{ status: string }>({
      method: "PUT",
      path: `/api/v1/system-admins/${encodeURIComponent(id)}`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  deleteSystemAdmin(id: string) {
    return request()<{ status: string }>({
      method: "DELETE",
      path: `/api/v1/system-admins/${encodeURIComponent(id)}`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  resetSystemAdminPassword(id: string) {
    return request()<ActivationCredentialOutput>({
      method: "POST",
      path: `/api/v1/system-admins/${encodeURIComponent(id)}/reset-password`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },

  // ---- 租户 ----
  listTenants(params: { page?: number; size?: number; keyword?: string; status?: number }) {
    return request()<PageTenantListItem>({
      method: "GET",
      path: "/api/v1/tenants",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  getAccountBalance(params: { accountType: number; accountId: string; detail?: boolean }) {
    return request()<AccountBalanceOutput>({
      method: "GET",
      path: "/api/v1/account/balance",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  getTenant(id: string) {
    return request()<TenantDetailOutput>({
      method: "GET",
      path: `/api/v1/tenants/${encodeURIComponent(id)}`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  createTenant(body: {
    tenantName: string;
    contactPerson?: string;
    contactEmail?: string;
    status?: number;
    initUsername?: string;
    initEmail?: string;
  }) {
    return request()<{
      tenantId: string;
      initUserId?: string;
      initUsername?: string;
      activationToken?: string;
      activationExpiresIn?: number;
    }>({
      method: "POST",
      path: "/api/v1/tenants",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  updateTenant(
    id: string,
    body: {
      tenantName: string;
      contactPerson?: string;
      contactEmail?: string;
      status?: number;
    }
  ) {
    return request()<{ status: string }>({
      method: "PUT",
      path: `/api/v1/tenants/${encodeURIComponent(id)}`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  deleteTenant(id: string) {
    return request()<{ status: string }>({
      method: "DELETE",
      path: `/api/v1/tenants/${encodeURIComponent(id)}`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  updateTenantStatus(id: string, status: "active" | "disabled") {
    return request()<{ status: string }>({
      method: "PATCH",
      path: `/api/v1/tenants/${encodeURIComponent(id)}/status`,
      headers: apiHeaders,
      body: { status },
      baseUrl: apiBaseUrl
    });
  },

  // ---- 租户用户 ----
  listTenantUsers(params: { page?: number; size?: number; tenantId?: string; keyword?: string }) {
    return request()<PageAdminUserItem>({
      method: "GET",
      path: "/api/v1/tenant-users",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  createTenantUser(body: { tenantId: string; username: string; email?: string }) {
    return request()<CreateAdminUserOutput>({
      method: "POST",
      path: "/api/v1/tenant-users",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  updateTenantUserStatus(id: string, status: "active" | "disabled") {
    return request()<{ status: string }>({
      method: "PATCH",
      path: `/api/v1/tenant-users/${encodeURIComponent(id)}/status`,
      headers: apiHeaders,
      body: { status },
      baseUrl: apiBaseUrl
    });
  },
  updateTenantUser(id: string, body: { email?: string; status?: number }) {
    return request()<{ status: string }>({
      method: "PUT",
      path: `/api/v1/tenant-users/${encodeURIComponent(id)}`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  resetTenantUserPassword(id: string) {
    return request()<ActivationCredentialOutput>({
      method: "POST",
      path: `/api/v1/tenant-users/${encodeURIComponent(id)}/reset-password`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
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
    return request()<PageEndUserItem>({
      method: "GET",
      path: "/api/v1/users",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  createEndUser(body: { username: string; email?: string; phone?: string }) {
    return request()<CreateAdminUserOutput>({
      method: "POST",
      path: "/api/v1/users",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  updateEndUserStatus(id: string, status: "active" | "disabled") {
    return request()<{ message: string }>({
      method: "PATCH",
      path: `/api/v1/users/${encodeURIComponent(id)}/status`,
      headers: apiHeaders,
      body: { status },
      baseUrl: apiBaseUrl
    });
  },
  resetEndUserPassword(id: string) {
    return request()<ActivationCredentialOutput>({
      method: "POST",
      path: `/api/v1/users/${encodeURIComponent(id)}/reset-password`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },

  listRechargeRecords(params: {
    page?: number;
    size?: number;
    tenantName?: string;
    username?: string;
    rechargeType?: string;
  }) {
    return request()<PageRechargeRecordItem>({
      method: "GET",
      path: "/api/v1/account/recharge-records",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  createRecharge(body: {
    packageType: number;
    tenantId?: string;
    userId?: string;
    paidAmountMinor?: number;
    amountMicroUsd: number;
    note?: string;
    paymentRef?: string;
    expireTime?: number | null;
  }) {
    return request()<RechargeOutputBody>({
      method: "POST",
      path: "/api/v1/recharges",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  reverseRecharge(orderId: string, body: { reason: string }) {
    return request()<{
      status: string;
      orderId: string;
      balanceLotId: string;
      reversedAmountUsd: number;
      originalAmountUsd: number;
      lostAmountUsd: number;
      balanceLotStatus: string;
    }>({
      method: "POST",
      path: `/api/v1/recharges/${encodeURIComponent(orderId)}/reverse`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  // ---- AI 使用记录人工退款 ----
  refundUsage(body: { requestId: string; reason?: string }) {
    return request()<{ status: string }>({
      method: "POST",
      path: "/api/v1/ai/usage/refund",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  batchRefundUsage(body: { requestIds: string[]; reason: string }) {
    return request()<BatchOpResult>({
      method: "POST",
      path: "/api/v1/ai/usage/batch-refund",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  getDebtStatus(ownerType: "tenant" | "user", accountId: string) {
	return request()<DebtStatusOutputBody>({
	  method: "GET",
	  path: `/api/v1/admin/debts/${ownerType}/${encodeURIComponent(accountId)}`,
	  headers: apiHeaders,
	  baseUrl: apiBaseUrl
	});
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
