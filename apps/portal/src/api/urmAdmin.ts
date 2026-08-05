import { authenticatedRequest, portalHeaders, serviceBaseUrl } from "./request";
import type {
  AccountBalanceOutput,
  BatchOpResult,
  ConsumptionTrendOutput,
  DashboardAlertsOutput,
  GlobalStatsRow,
  PageCashAccountItem,
  PageCashLedgerItem,
  PagePaymentOrderItem,
  PageWithdrawalItem,
  PaymentGlobalSettings,
  PaymentOrderItem,
  ResourceStatItem,
  JwtKeyItem,
  DebtStatusOutputBody,
  PageAuditLogItem,
  PageRechargeRecordItem,
  PageTransactionItem,
  RechargeOutputBody,
  PageAdminUserItem,
  PageEndUserItem,
  PageTenantListItem,
  ServiceAccessPolicy,
  ServiceRegistryDetail,
  ServiceRegistryItem,
  CreateAdminUserOutput,
  TenantDetailOutput,
  WechatConfig,
  WechatConfigWriteInput
} from "./types/admin";

function request(service: "urm" | "ai" = "urm") {
  return authenticatedRequest(service);
}

export const urmAdminApi = {
  // ---- 账号自助 ----
  // 修改本人密码（旧密码校验 + 新密码 ≥6 位）
  changePassword(body: { oldPassword: string; newPassword: string }) {
    return request()<{ message: string }>({
      method: "PUT",
      path: "/api/oauth2/password",
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },

  // ---- 系统管理员 ----
  listSystemAdmins(params: { page?: number; size?: number; keyword?: string }) {
    return request()<PageAdminUserItem>({
      method: "GET",
      path: "/api/v1/system-admins",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  createSystemAdmin(body: { username: string; email?: string; password?: string; serviceAccess?: { mode: "all" | "selected"; serviceIds: string[] } }) {
    return request()<CreateAdminUserOutput>({
      method: "POST",
      path: "/api/v1/system-admins",
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  updateSystemAdmin(id: string, body: { email?: string; status?: number; password?: string }) {
    return request()<{ status: string }>({
      method: "PUT",
      path: `/api/v1/system-admins/${encodeURIComponent(id)}`,
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  deleteSystemAdmin(id: string) {
    return request()<{ status: string }>({
      method: "DELETE",
      path: `/api/v1/system-admins/${encodeURIComponent(id)}`,
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  getSystemAdminServiceAccess(id: string) {
    return request()<ServiceAccessPolicy>({ method: "GET", path: `/api/v1/system-admins/${encodeURIComponent(id)}/service-access`, headers: portalHeaders, baseUrl: serviceBaseUrl("urm") });
  },
  updateSystemAdminServiceAccess(id: string, body: { mode: "all" | "selected"; serviceIds: string[] }) {
    return request()<ServiceAccessPolicy>({ method: "PUT", path: `/api/v1/system-admins/${encodeURIComponent(id)}/service-access`, headers: portalHeaders, body, baseUrl: serviceBaseUrl("urm") });
  },

  // ---- 租户 ----
  listTenants(params: { page?: number; size?: number; keyword?: string; status?: number }) {
    return request()<PageTenantListItem>({
      method: "GET",
      path: "/api/v1/tenants",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  getAccountBalance(params: { accountType: number; accountId: string; detail?: boolean }) {
    return request()<AccountBalanceOutput>({
      method: "GET",
      path: "/api/v1/account/balance",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  getTenant(id: string) {
    return request()<TenantDetailOutput>({
      method: "GET",
      path: `/api/v1/tenants/${encodeURIComponent(id)}`,
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  createTenant(body: {
    tenantName: string;
    contactPerson?: string;
    contactEmail?: string;
    status?: number;
    initUsername?: string;
    initEmail?: string;
    serviceAccess?: { mode: "all" | "selected"; serviceIds: string[] };
  }) {
    return request()<{ tenantId: string; initUserId?: string; initUsername?: string }>({
      method: "POST",
      path: "/api/v1/tenants",
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
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
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  deleteTenant(id: string) {
    return request()<{ status: string }>({
      method: "DELETE",
      path: `/api/v1/tenants/${encodeURIComponent(id)}`,
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  updateTenantStatus(id: string, status: "active" | "disabled") {
    return request()<{ status: string }>({
      method: "PATCH",
      path: `/api/v1/tenants/${encodeURIComponent(id)}/status`,
      headers: portalHeaders,
      body: { status },
      baseUrl: serviceBaseUrl("urm")
    });
  },
  getTenantServiceAccess(id: string) {
    return request()<ServiceAccessPolicy>({ method: "GET", path: `/api/v1/tenants/${encodeURIComponent(id)}/service-access`, headers: portalHeaders, baseUrl: serviceBaseUrl("urm") });
  },
  updateTenantServiceAccess(id: string, body: { mode: "all" | "selected"; serviceIds: string[] }) {
    return request()<ServiceAccessPolicy>({ method: "PUT", path: `/api/v1/tenants/${encodeURIComponent(id)}/service-access`, headers: portalHeaders, body, baseUrl: serviceBaseUrl("urm") });
  },

  // ---- 租户用户 ----
  listTenantUsers(params: { page?: number; size?: number; tenantId?: string; keyword?: string }) {
    return request()<PageAdminUserItem>({
      method: "GET",
      path: "/api/v1/tenant-users",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  createTenantUser(body: { tenantId: string; username: string; email?: string }) {
    return request()<CreateAdminUserOutput>({
      method: "POST",
      path: "/api/v1/tenant-users",
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  updateTenantUserStatus(id: string, status: "active" | "disabled") {
    return request()<{ status: string }>({
      method: "PATCH",
      path: `/api/v1/tenant-users/${encodeURIComponent(id)}/status`,
      headers: portalHeaders,
      body: { status },
      baseUrl: serviceBaseUrl("urm")
    });
  },
  updateTenantUser(id: string, body: { email?: string; status?: number; password?: string }) {
    return request()<{ status: string }>({
      method: "PUT",
      path: `/api/v1/tenant-users/${encodeURIComponent(id)}`,
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  resetTenantUserPassword(id: string) {
    return request()<{ status: string }>({
      method: "POST",
      path: `/api/v1/tenant-users/${encodeURIComponent(id)}/reset-password`,
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
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
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  createEndUser(body: { username: string; email?: string; phone?: string }) {
    return request()<{ userId: string; tenantId: string; username: string; defaultPassword: string }>({
      method: "POST",
      path: "/api/v1/users",
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  updateEndUserStatus(id: string, status: "active" | "disabled") {
    return request()<{ message: string }>({
      method: "PATCH",
      path: `/api/v1/users/${encodeURIComponent(id)}/status`,
      headers: portalHeaders,
      body: { status },
      baseUrl: serviceBaseUrl("urm")
    });
  },
  resetEndUserPassword(id: string) {
    return request()<{ message: string }>({
      method: "POST",
      path: `/api/v1/users/${encodeURIComponent(id)}/reset-password`,
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },

  // ---- 服务注册 ----
  listServices() {
    return request()<ServiceRegistryItem[]>({ method: "GET", path: "/api/v1/governance/services", headers: portalHeaders, baseUrl: serviceBaseUrl("urm") });
  },
  getService(serviceId: string) {
    return request()<ServiceRegistryDetail>({ method: "GET", path: `/api/v1/governance/services/${encodeURIComponent(serviceId)}`, headers: portalHeaders, baseUrl: serviceBaseUrl("urm") });
  },
  createService(body: { serviceId: string; displayName: string; description?: string; portalEnabled: boolean }) {
    return request()<ServiceRegistryDetail>({ method: "POST", path: "/api/v1/governance/services", headers: portalHeaders, body, baseUrl: serviceBaseUrl("urm") });
  },
  updateService(serviceId: string, body: { displayName?: string; description?: string; status: "active" | "disabled"; portalEnabled: boolean }) {
    return request()<{ status: string }>({ method: "PUT", path: `/api/v1/governance/services/${encodeURIComponent(serviceId)}`, headers: portalHeaders, body, baseUrl: serviceBaseUrl("urm") });
  },
  deleteService(serviceId: string) {
    return request()<{ status: string }>({ method: "DELETE", path: `/api/v1/governance/services/${encodeURIComponent(serviceId)}`, headers: portalHeaders, baseUrl: serviceBaseUrl("urm") });
  },
  createServiceSource(serviceId: string, body: { sourceCidr: string; description?: string }) {
    return request()<{ status: string }>({ method: "POST", path: `/api/v1/governance/services/${encodeURIComponent(serviceId)}/sources`, headers: portalHeaders, body, baseUrl: serviceBaseUrl("urm") });
  },
  updateServiceSource(serviceId: string, sourceId: number, body: { sourceCidr: string; description?: string }) {
    return request()<{ status: string }>({ method: "PUT", path: `/api/v1/governance/services/${encodeURIComponent(serviceId)}/sources/${sourceId}`, headers: portalHeaders, body, baseUrl: serviceBaseUrl("urm") });
  },
  deleteServiceSource(serviceId: string, sourceId: number) {
    return request()<{ status: string }>({ method: "DELETE", path: `/api/v1/governance/services/${encodeURIComponent(serviceId)}/sources/${sourceId}`, headers: portalHeaders, baseUrl: serviceBaseUrl("urm") });
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
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  createRecharge(body: {
    packageType: number;
    tenantId?: string;
    userId?: string;
    paidAmount?: number;
    creditAmount: number;
    note?: string;
    paymentRef?: string;
    expireTime?: number | null;
  }) {
    return request()<RechargeOutputBody>({
      method: "POST",
      path: "/api/v1/recharges",
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  reverseRecharge(orderId: string, body: { reason: string }) {
    return request()<{
      status: string;
      orderId: string;
      packageId: string;
      reversedCredits: number;
      originalCredits: number;
      lostCredits: number;
      packageStatus: string;
    }>({
      method: "POST",
      path: `/api/v1/recharges/${encodeURIComponent(orderId)}/reverse`,
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  listTransactions(params: {
    page?: number;
    size?: number;
    tenantId?: string;
    userId?: string;
    tenantName?: string;
    username?: string;
    clientName?: string;
    status?: string;
    timeFrom?: number;
    timeTo?: number;
  }) {
    return request()<PageTransactionItem>({
      method: "GET",
      path: "/api/v1/account/transactions",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },

  // ---- 计费事件人工干预（交易流水）----
  refund(body: { eventId: string; reason?: string }) {
    return request()<{ status: string }>({
      method: "POST",
      path: "/api/v1/refunds",
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  manualConfirmEvent(eventId: string, body: { actualTenantCredits: number; actualUserCredits: number; note: string }) {
    return request()<{ eventId: string }>({
      method: "POST",
      path: `/api/v1/billing/events/${encodeURIComponent(eventId)}/confirm`,
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  adminDismissEvent(eventId: string, body: { note: string }) {
    return request()<{ eventId: string }>({
      method: "POST",
      path: `/api/v1/billing/events/${encodeURIComponent(eventId)}/dismiss`,
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  batchConfirmEvents(body: { eventIds: string[]; note: string }) {
    return request()<BatchOpResult>({
      method: "POST",
      path: "/api/v1/billing/events/batch-confirm",
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  batchRefundEvents(body: { eventIds: string[]; reason: string }) {
    return request()<BatchOpResult>({
      method: "POST",
      path: "/api/v1/billing/events/batch-refund",
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  getDebtStatus(ownerType: "tenant" | "user", accountId: string) {
	return request()<DebtStatusOutputBody>({
	  method: "GET",
	  path: `/api/v2/admin/debts/${ownerType}/${encodeURIComponent(accountId)}`,
	  headers: portalHeaders,
	  baseUrl: serviceBaseUrl("urm")
	});
  },

  // ---- 认证审计 ----
  getAuthAuditLogs(params: {
    page?: number;
    size?: number;
    eventType?: string;
    principalType?: string;
    decision?: string;
    clientId?: string;
    userId?: string;
  }) {
    return request()<PageAuditLogItem>({
      method: "GET",
      path: "/api/v1/auth-audit-logs",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },

  // ---- JWT 密钥 ----
  listJwtKeys() {
    return request()<{ keys: JwtKeyItem[]; total: number }>({
      method: "GET",
      path: "/api/v1/jwt-keys",
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  rotateJwtKey() {
    return request()<{ message: string }>({
      method: "POST",
      path: "/api/v1/jwt-keys/rotate",
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },


  // ---- 控制概览（数据分析）----
  getGlobalStats(params: { timeFrom?: number; timeTo?: number }) {
    return request()<GlobalStatsRow>({
      method: "GET",
      path: "/api/v1/analytics/global-stats",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  getConsumptionTrend(params: { timeFrom?: number; timeTo?: number; accountType?: string }) {
    return request()<ConsumptionTrendOutput>({
      method: "GET",
      path: "/api/v1/analytics/consumption-trend",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  getResourceStatistics(params: { timeFrom?: number; timeTo?: number }) {
    return request()<{ resources: ResourceStatItem[] }>({
      method: "GET",
      path: "/api/v1/analytics/resource-statistics",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  getDashboardAlerts() {
    return request()<DashboardAlertsOutput>({
      method: "GET",
      path: "/api/v1/dashboard/alerts",
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  cancelPreAuth(eventId: string) {
    return request()<{ status: string }>({
      method: "POST",
      path: `/api/v1/billing/events/${encodeURIComponent(eventId)}/cancel`,
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },

  // ==================== 微信支付在线充值（管理端） ====================

  getPaymentSettings() {
    return request()<PaymentGlobalSettings>({
      method: "GET",
      path: "/api/v1/admin/payment-settings",
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  updatePaymentSettings(body: PaymentGlobalSettings) {
    return request()<PaymentGlobalSettings>({
      method: "PUT",
      path: "/api/v1/admin/payment-settings",
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  getWechatConfig() {
    return request()<WechatConfig>({
      method: "GET",
      path: "/api/v1/admin/wechat-config",
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  updateWechatConfig(body: WechatConfigWriteInput) {
    return request()<WechatConfig>({
      method: "PUT",
      path: "/api/v1/admin/wechat-config",
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  listPaymentOrders(params: { scene?: string; status?: string; tenantId?: string; page?: number; size?: number } = {}) {
    return request()<PagePaymentOrderItem>({
      method: "GET",
      path: "/api/v1/admin/payment-orders",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  syncPaymentOrder(orderId: string) {
    return request()<PaymentOrderItem>({
      method: "POST",
      path: `/api/v1/admin/payment-orders/${encodeURIComponent(orderId)}/sync`,
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  listCashAccounts(params: { page?: number; size?: number } = {}) {
    return request()<PageCashAccountItem>({
      method: "GET",
      path: "/api/v1/admin/cash-accounts",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  listCashLedger(params: { tenantId: string; txnType?: string; page?: number; size?: number }) {
    return request()<PageCashLedgerItem>({
      method: "GET",
      path: "/api/v1/admin/cash-ledger",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  listWithdrawals(params: { status?: string; page?: number; size?: number } = {}) {
    return request()<PageWithdrawalItem>({
      method: "GET",
      path: "/api/v1/admin/withdrawals",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  reviewWithdrawal(id: string, body: { approve: boolean; note?: string }) {
    return request()<{ message: string }>({
      method: "POST",
      path: `/api/v1/admin/withdrawals/${encodeURIComponent(id)}/review`,
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  settleWithdrawal(id: string, body: { paymentRef: string }) {
    return request()<{ message: string }>({
      method: "POST",
      path: `/api/v1/admin/withdrawals/${encodeURIComponent(id)}/settle`,
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  }
};
