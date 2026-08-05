import { authenticatedRequest, portalHeaders, portalHeadersFor, serviceBaseUrl } from "./request";
import type {
  CreateTenantInvitationOutputBody,
  CreateTenantEndUserOutputBody,
  PageTenantCashLedgerItem,
  PageTenantEndUserItem,
  PageTenantInvitationItem,
  PageTenantRechargeRecordItem,
  PageTenantTopupOrderItem,
  PageTenantTransactionItem,
  PageTenantWithdrawal,
  TenantAiApiKeysOutputBody,
  TenantBuyCreditsResult,
  TenantCashAccount,
  TenantClientConsumptionItem,
  TenantOverviewStats,
  TenantPortalBranding,
  TenantRechargeOutputBody,
  TenantPaymentSettings,
  TenantTopupConfig,
  TenantTopupOrderCreated,
  TenantTopupOrderStatus,
  TenantWithdrawal
} from "./types/tenant";

function request(service: "urm" | "ai" = "urm") {
  return authenticatedRequest(service);
}

export const tenantApi = {
  getPortalBranding() {
    return request()<TenantPortalBranding>({
      method: "GET",
      path: "/api/v1/tenant/branding",
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  updatePortalBranding(body: Pick<TenantPortalBranding, "tenantName" | "customerSiteName">) {
    return request()<TenantPortalBranding>({
      method: "PUT",
      path: "/api/v1/tenant/branding",
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  updatePortalFavicon(dataUrl: string) {
    return request()<TenantPortalBranding>({
      method: "PUT",
      path: "/api/v1/tenant/branding/favicon",
      headers: portalHeaders,
      body: { dataUrl },
      baseUrl: serviceBaseUrl("urm")
    });
  },
  deletePortalFavicon() {
    return request()<TenantPortalBranding>({
      method: "DELETE",
      path: "/api/v1/tenant/branding/favicon",
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  getOverview(params: { timeFrom?: number; timeTo?: number } = {}) {
    return request()<TenantOverviewStats>({
      method: "GET",
      path: "/api/v1/tenants/analytics/overview",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  listClientConsumption(params: { timeFrom?: number; timeTo?: number } = {}) {
    return request()<TenantClientConsumptionItem[]>({
      method: "GET",
      path: "/api/v1/tenants/analytics/app-consumption",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  listTransactions(params: { page?: number; size?: number; username?: string; clientName?: string; status?: string }) {
    return request()<PageTenantTransactionItem>({
      method: "GET",
      path: "/api/v1/account/transactions",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  listRechargeRecords(params: { page?: number; size?: number; username?: string; rechargeType?: string }) {
    return request()<PageTenantRechargeRecordItem>({
      method: "GET",
      path: "/api/v1/account/recharge-records",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  createUserRecharge(body: { userId: string; paidAmount?: number; creditAmount: number; paymentRef?: string; note?: string }) {
    return request()<TenantRechargeOutputBody>({
      method: "POST",
      path: "/api/v1/recharges",
      headers: portalHeaders,
      body: {
        packageType: 2,
        ...body
      },
      baseUrl: serviceBaseUrl("urm")
    });
  },
  listEndUsers(params: { page?: number; size?: number; keyword?: string }) {
    return request()<PageTenantEndUserItem>({
      method: "GET",
      path: "/api/v1/users",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  createEndUser(body: { username: string; email?: string; phone?: string }) {
    return request()<CreateTenantEndUserOutputBody>({
      method: "POST",
      path: "/api/v1/users",
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  updateEndUserStatus(userId: string, status: "active" | "disabled") {
    return request()<{ message: string }>({
      method: "PATCH",
      path: `/api/v1/users/${encodeURIComponent(userId)}/status`,
      headers: portalHeaders,
      body: { status },
      baseUrl: serviceBaseUrl("urm")
    });
  },
  resetEndUserPassword(userId: string) {
    return request()<{ message: string }>({
      method: "POST",
      path: `/api/v1/users/${encodeURIComponent(userId)}/reset-password`,
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  listInvitations(params: { page?: number; size?: number }) {
    return request()<PageTenantInvitationItem>({
      method: "GET",
      path: "/api/v1/invitations",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  createInvitation(body: { description?: string; maxUses?: number; expireTime?: number | null }) {
    return request()<CreateTenantInvitationOutputBody>({
      method: "POST",
      path: "/api/v1/invitations",
      headers: portalHeaders,
      body: {
        description: body.description,
        max_uses: body.maxUses,
        expire_time: body.expireTime
      },
      baseUrl: serviceBaseUrl("urm")
    });
  },
  updateInvitation(id: number, body: { status: number; description?: string }) {
    return request()<{ success: boolean }>({
      method: "PUT",
      path: `/api/v1/invitations/${encodeURIComponent(String(id))}`,
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  deleteInvitation(id: number) {
    return request()<{ success: boolean }>({
      method: "DELETE",
      path: `/api/v1/invitations/${encodeURIComponent(String(id))}`,
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  listAiTenantApiKeys() {
    return request("ai")<TenantAiApiKeysOutputBody>({
      method: "GET",
      path: "/api/v1/tenant-api-keys",
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai")
    });
  },

  // ===== 在线充值（微信支付） =====
  getTopupConfig() {
    return request()<TenantTopupConfig>({
      method: "GET",
      path: "/api/v1/payments/topup-config",
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  createTopupOrder(body: { amount?: number; packageId?: string }) {
    return request()<TenantTopupOrderCreated>({
      method: "POST",
      path: "/api/v1/payments/topup-orders",
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  getTopupOrder(orderId: string) {
    return request()<TenantTopupOrderStatus>({
      method: "GET",
      path: `/api/v1/payments/topup-orders/${orderId}`,
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  listTopupOrders(params: { page?: number; size?: number } = {}) {
    return request()<PageTenantTopupOrderItem>({
      method: "GET",
      path: "/api/v1/payments/topup-orders",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },

  getPaymentSettings() {
    return request()<TenantPaymentSettings>({
      method: "GET",
      path: "/api/v1/tenant/payment-settings",
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  updatePaymentSettings(body: TenantPaymentSettings) {
    return request()<TenantPaymentSettings>({
      method: "PUT",
      path: "/api/v1/tenant/payment-settings",
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },

  // ===== 现金账户 =====
  getCashAccount() {
    return request()<TenantCashAccount>({
      method: "GET",
      path: "/api/v1/tenant/cash-account",
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  listCashLedger(params: { txnType?: string; page?: number; size?: number } = {}) {
    return request()<PageTenantCashLedgerItem>({
      method: "GET",
      path: "/api/v1/tenant/cash-ledger",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  buyCredits(body: { amount: number }) {
    return request()<TenantBuyCreditsResult>({
      method: "POST",
      path: "/api/v1/tenant/cash/buy-credits",
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  applyWithdrawal(body: { amount: number; accountName: string; bankName: string; accountNo: string; note?: string }) {
    return request()<TenantWithdrawal>({
      method: "POST",
      path: "/api/v1/tenant/withdrawals",
      headers: portalHeaders,
      body,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  listWithdrawals(params: { status?: string; page?: number; size?: number } = {}) {
    return request()<PageTenantWithdrawal>({
      method: "GET",
      path: "/api/v1/tenant/withdrawals",
      headers: portalHeaders,
      query: params,
      baseUrl: serviceBaseUrl("urm")
    });
  },
  cancelWithdrawal(id: string) {
    return request()<{ message: string }>({
      method: "POST",
      path: `/api/v1/tenant/withdrawals/${encodeURIComponent(id)}/cancel`,
      headers: portalHeaders,
      baseUrl: serviceBaseUrl("urm")
    });
  }
};
