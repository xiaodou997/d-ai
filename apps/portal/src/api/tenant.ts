import { authenticatedRequest, apiHeaders, apiBaseUrl } from "./request";
import type {
  CreateTenantInvitationOutputBody,
  CreateTenantEndUserOutputBody,
  PageTenantBalanceLedgerItem,
  PageTenantEndUserItem,
  PageTenantInvitationItem,
  PageTenantRechargeRecordItem,
  PageTenantTopupOrderItem,
  TenantAiApiKeysOutputBody,
  TenantBalanceAccount,
  TenantClientConsumptionItem,
  TenantOverviewStats,
  TenantPortalBranding,
  TenantRechargeOutputBody,
  TenantPaymentSettings,
  TenantTopupConfig,
  TenantTopupOrderCreated,
  TenantTopupOrderStatus
} from "./types/tenant";

function request() {
  return authenticatedRequest();
}

export const tenantApi = {
  getPortalBranding() {
    return request()<TenantPortalBranding>({
      method: "GET",
      path: "/api/v1/tenant/branding",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  updatePortalBranding(body: Pick<TenantPortalBranding, "tenantName" | "customerSiteName">) {
    return request()<TenantPortalBranding>({
      method: "PUT",
      path: "/api/v1/tenant/branding",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  updatePortalFavicon(dataUrl: string) {
    return request()<TenantPortalBranding>({
      method: "PUT",
      path: "/api/v1/tenant/branding/favicon",
      headers: apiHeaders,
      body: { dataUrl },
      baseUrl: apiBaseUrl
    });
  },
  deletePortalFavicon() {
    return request()<TenantPortalBranding>({
      method: "DELETE",
      path: "/api/v1/tenant/branding/favicon",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  getOverview(params: { timeFrom?: number; timeTo?: number } = {}) {
    return request()<TenantOverviewStats>({
      method: "GET",
      path: "/api/v1/tenants/analytics/overview",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  listClientConsumption(params: { timeFrom?: number; timeTo?: number } = {}) {
    return request()<TenantClientConsumptionItem[]>({
      method: "GET",
      path: "/api/v1/tenants/analytics/app-consumption",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  listRechargeRecords(params: { page?: number; size?: number; username?: string; rechargeType?: string }) {
    return request()<PageTenantRechargeRecordItem>({
      method: "GET",
      path: "/api/v1/account/recharge-records",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  createUserRecharge(body: { userId: string; paidAmountMinor?: number; amountMicroUsd: number; paymentRef?: string; note?: string }) {
    return request()<TenantRechargeOutputBody>({
      method: "POST",
      path: "/api/v1/recharges",
      headers: apiHeaders,
      body: {
        packageType: 2,
        ...body
      },
      baseUrl: apiBaseUrl
    });
  },
  listEndUsers(params: { page?: number; size?: number; keyword?: string }) {
    return request()<PageTenantEndUserItem>({
      method: "GET",
      path: "/api/v1/users",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  createEndUser(body: { username: string; email?: string; phone?: string }) {
    return request()<CreateTenantEndUserOutputBody>({
      method: "POST",
      path: "/api/v1/users",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  updateEndUserStatus(userId: string, status: "active" | "disabled") {
    return request()<{ message: string }>({
      method: "PATCH",
      path: `/api/v1/users/${encodeURIComponent(userId)}/status`,
      headers: apiHeaders,
      body: { status },
      baseUrl: apiBaseUrl
    });
  },
  resetEndUserPassword(userId: string) {
    return request()<{ message: string }>({
      method: "POST",
      path: `/api/v1/users/${encodeURIComponent(userId)}/reset-password`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  listInvitations(params: { page?: number; size?: number }) {
    return request()<PageTenantInvitationItem>({
      method: "GET",
      path: "/api/v1/invitations",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },
  createInvitation(body: { description?: string; maxUses?: number; expireTime?: number | null }) {
    return request()<CreateTenantInvitationOutputBody>({
      method: "POST",
      path: "/api/v1/invitations",
      headers: apiHeaders,
      body: {
        description: body.description,
        max_uses: body.maxUses,
        expire_time: body.expireTime
      },
      baseUrl: apiBaseUrl
    });
  },
  updateInvitation(id: number, body: { status: number; description?: string }) {
    return request()<{ success: boolean }>({
      method: "PUT",
      path: `/api/v1/invitations/${encodeURIComponent(String(id))}`,
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  deleteInvitation(id: number) {
    return request()<{ success: boolean }>({
      method: "DELETE",
      path: `/api/v1/invitations/${encodeURIComponent(String(id))}`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  listAiTenantApiKeys() {
    return request()<TenantAiApiKeysOutputBody>({
      method: "GET",
      path: "/api/v1/tenant-api-keys",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },

  // ===== 在线充值（微信支付） =====
  getTopupConfig() {
    return request()<TenantTopupConfig>({
      method: "GET",
      path: "/api/v1/payments/topup-config",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  createTopupOrder(body: { amountMicroUsd?: number; packageId?: string }) {
    return request()<TenantTopupOrderCreated>({
      method: "POST",
      path: "/api/v1/payments/topup-orders",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  getTopupOrder(orderId: string) {
    return request()<TenantTopupOrderStatus>({
      method: "GET",
      path: `/api/v1/payments/topup-orders/${orderId}`,
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  listTopupOrders(params: { page?: number; size?: number } = {}) {
    return request()<PageTenantTopupOrderItem>({
      method: "GET",
      path: "/api/v1/payments/topup-orders",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  },

  getPaymentSettings() {
    return request()<TenantPaymentSettings>({
      method: "GET",
      path: "/api/v1/tenant/payment-settings",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  updatePaymentSettings(body: TenantPaymentSettings) {
    return request()<TenantPaymentSettings>({
      method: "PUT",
      path: "/api/v1/tenant/payment-settings",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },

  // ===== 统一 USD 余额 =====
  getBalanceAccount() {
    return request()<TenantBalanceAccount>({
      method: "GET",
      path: "/api/v1/tenant/balance",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    });
  },
  listBalanceLedger(params: { txnType?: string; page?: number; size?: number } = {}) {
    return request()<PageTenantBalanceLedgerItem>({
      method: "GET",
      path: "/api/v1/tenant/balance-ledger",
      headers: apiHeaders,
      query: params,
      baseUrl: apiBaseUrl
    });
  }
};
