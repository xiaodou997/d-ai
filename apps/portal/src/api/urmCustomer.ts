// URM 用户（终端用户 type=4）自助业务 API —— 1:1 还原 v1 urm-customer src/api/account.js 调用，
// 仅 API 层适配 v4 huma 扁平端点（无 {code,data} 信封，列表={items,total,page,size}）。
// v1 路径 /urm/v1/account/* → v4 /api/v1/account/*；/urm/oauth2/password → /api/oauth2/password。
// 终端用户分支由后端按 claims.UserType==4 自动锁定本人范围，前端无需传 userId。
import { authenticatedRequest, portalHeaders, serviceBaseUrl } from "./request";
import type {
  AccountBalance,
  AccountTransactionItem,
  CustomerPortalBrand,
  Page,
  RechargeRecordItem,
  TopupConfig,
  TopupOrderCreated,
  TopupOrderItem,
  TopupOrderStatus
} from "./types/urmCustomer";

function urm() {
  return authenticatedRequest("urm");
}

const baseUrl = serviceBaseUrl("urm");

export const urmCustomerApi = {
  getPortalBrand() {
    return urm()<CustomerPortalBrand>({
      method: "GET",
      path: "/api/v1/customer/portal-brand",
      headers: portalHeaders,
      baseUrl
    });
  },
  // 我的余额（含积分包明细）
  getBalance(detail = true) {
    return urm()<AccountBalance>({
      method: "GET",
      path: "/api/v1/account/balance",
      headers: portalHeaders,
      query: { detail },
      baseUrl
    });
  },

  // 我的积分流水（分页）
  getTransactions(params: { page?: number; size?: number }) {
    return urm()<Page<AccountTransactionItem>>({
      method: "GET",
      path: "/api/v1/account/transactions",
      headers: portalHeaders,
      query: params,
      baseUrl
    });
  },

  // 我的充值记录（分页）
  getRechargeRecords(params: { page?: number; size?: number }) {
    return urm()<Page<RechargeRecordItem>>({
      method: "GET",
      path: "/api/v1/account/recharge-records",
      headers: portalHeaders,
      query: params,
      baseUrl
    });
  },

  // 修改密码（旧密码校验 + 新密码 ≥6 位）
  changePassword(body: { oldPassword: string; newPassword: string }) {
    return urm()<{ message: string }>({
      method: "PUT",
      path: "/api/oauth2/password",
      headers: portalHeaders,
      body,
      baseUrl
    });
  },

  // 在线充值配置（汇率/限额，不含费率）
  getTopupConfig() {
    return urm()<TopupConfig>({
      method: "GET",
      path: "/api/v1/payments/topup-config",
      headers: portalHeaders,
      baseUrl
    });
  },

  // 发起在线充值（微信 Native 扫码）
  createTopupOrder(body: { amount?: number; packageId?: string }) {
    return urm()<TopupOrderCreated>({
      method: "POST",
      path: "/api/v1/payments/topup-orders",
      headers: portalHeaders,
      body,
      baseUrl
    });
  },

  // 查询充值订单状态（轮询用）
  getTopupOrder(orderId: string) {
    return urm()<TopupOrderStatus>({
      method: "GET",
      path: `/api/v1/payments/topup-orders/${orderId}`,
      headers: portalHeaders,
      baseUrl
    });
  },

  // 我的在线充值订单（分页）
  listTopupOrders(params: { page?: number; size?: number } = {}) {
    return urm()<Page<TopupOrderItem>>({
      method: "GET",
      path: "/api/v1/payments/topup-orders",
      headers: portalHeaders,
      query: params,
      baseUrl
    });
  }
};
