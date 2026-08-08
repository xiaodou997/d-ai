// Platform 用户（终端用户 type=4）自助业务 API —— 1:1 还原 v1 platform-customer src/api/account.js 调用，
// 仅 API 层适配 v4 huma 扁平端点（无 {code,data} 信封，列表={items,total,page,size}）。
// 用户端统一使用 D-AI 的 /api/v1 业务端点和 /api/auth 账号端点。
// 终端用户分支由后端按 claims.UserType==4 自动锁定本人范围，前端无需传 userId。
import { authenticatedRequest, apiHeaders, apiBaseUrl } from "./request";
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
} from "./types/platformCustomer";

function platform() {
  return authenticatedRequest();
}

const baseUrl = apiBaseUrl;

export const platformCustomerApi = {
  getPortalBrand() {
    return platform()<CustomerPortalBrand>({
      method: "GET",
      path: "/api/v1/customer/portal-brand",
      headers: apiHeaders,
      baseUrl
    });
  },
  // 我的 USD 余额（含有效期批次明细）
  getBalance(detail = true) {
    return platform()<AccountBalance>({
      method: "GET",
      path: "/api/v1/account/balance",
      headers: apiHeaders,
      query: { detail },
      baseUrl
    });
  },

  // 我的余额流水（分页）
  getTransactions(params: { page?: number; size?: number }) {
    return platform()<Page<AccountTransactionItem>>({
      method: "GET",
      path: "/api/v1/account/transactions",
      headers: apiHeaders,
      query: params,
      baseUrl
    });
  },

  // 我的充值记录（分页）
  getRechargeRecords(params: { page?: number; size?: number }) {
    return platform()<Page<RechargeRecordItem>>({
      method: "GET",
      path: "/api/v1/account/recharge-records",
      headers: apiHeaders,
      query: params,
      baseUrl
    });
  },

  // 修改密码（旧密码校验 + 新密码 ≥6 位）
  changePassword(body: { oldPassword: string; newPassword: string }) {
    return platform()<{ message: string }>({
      method: "PUT",
      path: "/api/auth/password",
      headers: apiHeaders,
      body,
      baseUrl
    });
  },

  // USD 在线充值配置
  getTopupConfig() {
    return platform()<TopupConfig>({
      method: "GET",
      path: "/api/v1/payments/topup-config",
      headers: apiHeaders,
      baseUrl
    });
  },

  // 发起在线充值（微信 Native 扫码）
  createTopupOrder(body: { amountMicroUsd?: number; packageId?: string }) {
    return platform()<TopupOrderCreated>({
      method: "POST",
      path: "/api/v1/payments/topup-orders",
      headers: apiHeaders,
      body,
      baseUrl
    });
  },

  // 查询充值订单状态（轮询用）
  getTopupOrder(orderId: string) {
    return platform()<TopupOrderStatus>({
      method: "GET",
      path: `/api/v1/payments/topup-orders/${orderId}`,
      headers: apiHeaders,
      baseUrl
    });
  },

  // 我的在线充值订单（分页）
  listTopupOrders(params: { page?: number; size?: number } = {}) {
    return platform()<Page<TopupOrderItem>>({
      method: "GET",
      path: "/api/v1/payments/topup-orders",
      headers: apiHeaders,
      query: params,
      baseUrl
    });
  }
};
