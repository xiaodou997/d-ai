// URM 租户自助业务 API —— 集中承载 Portal 租户端页面调用，
// 仅 API 层适配 v4 huma 扁平端点（无 {code,data} 信封，列表={items,total,page,size}）。
// v1 路径 /urm/v1/* → v4 /api/v1/*。所有端点 service=urm，headers=portalHeaders。
import { authenticatedRequest, portalHeaders, serviceBaseUrl } from "./request";
import type {
  AccountBalance,
  AccountTransactionItem,
  ClientConsumptionItem,
  CreateEndUserOutput,
  CreateInviteCodeOutput,
  EndUserItem,
  InviteCodeItem,
  Page,
  RechargeOutput,
  RechargeRecordItem,
  ReverseRechargeOutput,
  TenantAnalyticsOverview,
  UserConsumptionItem
} from "./types/urmTenant";

function urm() {
  return authenticatedRequest("urm");
}

const baseUrl = serviceBaseUrl("urm");

export const urmTenantApi = {
  // ===== 账号自助 =====
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

  // ===== 统计 / 概览 =====
  getAnalyticsOverview(params: { timeFrom?: number; timeTo?: number } = {}) {
    return urm()<TenantAnalyticsOverview>({
      method: "GET",
      path: "/api/v1/tenants/analytics/overview",
      headers: portalHeaders,
      query: params,
      baseUrl
    });
  },
  getAppConsumption(params: { timeFrom?: number; timeTo?: number } = {}) {
    return urm()<ClientConsumptionItem[]>({
      method: "GET",
      path: "/api/v1/tenants/analytics/app-consumption",
      headers: portalHeaders,
      query: params,
      baseUrl
    });
  },
  getUserConsumption(params: { timeFrom?: number; timeTo?: number; limit?: number } = {}) {
    return urm()<UserConsumptionItem[]>({
      method: "GET",
      path: "/api/v1/tenants/analytics/user-consumption",
      headers: portalHeaders,
      query: params,
      baseUrl
    });
  },

  // ===== 统一账户 =====
  getAccountBalance(detail = true) {
    return urm()<AccountBalance>({
      method: "GET",
      path: "/api/v1/account/balance",
      headers: portalHeaders,
      query: { detail },
      baseUrl
    });
  },
  getTransactions(params: {
    page?: number;
    size?: number;
    username?: string;
    clientName?: string;
    status?: string;
    timeFrom?: number;
    timeTo?: number;
  }) {
    return urm()<Page<AccountTransactionItem>>({
      method: "GET",
      path: "/api/v1/account/transactions",
      headers: portalHeaders,
      query: params,
      baseUrl
    });
  },
  getRechargeRecords(params: {
    page?: number;
    size?: number;
    username?: string;
    rechargeType?: string;
    timeFrom?: number;
    timeTo?: number;
  }) {
    return urm()<Page<RechargeRecordItem>>({
      method: "GET",
      path: "/api/v1/account/recharge-records",
      headers: portalHeaders,
      query: params,
      baseUrl
    });
  },
  // 用户充值记录：租户充用户（rechargeType=2）
  getUserRechargeRecords(params: { page?: number; size?: number; username?: string; timeFrom?: number; timeTo?: number }) {
    return urm()<Page<RechargeRecordItem>>({
      method: "GET",
      path: "/api/v1/account/recharge-records",
      headers: portalHeaders,
      query: { ...params, rechargeType: "2" },
      baseUrl
    });
  },

  // ===== 终端用户 =====
  getUsers(params: { keyword?: string; page?: number; size?: number }) {
    return urm()<Page<EndUserItem>>({
      method: "GET",
      path: "/api/v1/users",
      headers: portalHeaders,
      query: params,
      baseUrl
    });
  },
  createEndUser(body: { username: string; email?: string; phone?: string }) {
    return urm()<CreateEndUserOutput>({
      method: "POST",
      path: "/api/v1/users",
      headers: portalHeaders,
      body,
      baseUrl
    });
  },
  updateUserStatus(userId: string, status: "active" | "disabled") {
    return urm()<{ message: string }>({
      method: "PATCH",
      path: `/api/v1/users/${encodeURIComponent(userId)}/status`,
      headers: portalHeaders,
      body: { status },
      baseUrl
    });
  },
  resetUserPassword(userId: string) {
    return urm()<{ message: string }>({
      method: "POST",
      path: `/api/v1/users/${encodeURIComponent(userId)}/reset-password`,
      headers: portalHeaders,
      baseUrl
    });
  },
  deleteEndUser(userId: string) {
    return urm()<{ success: boolean }>({
      method: "DELETE",
      path: `/api/v1/users/${encodeURIComponent(userId)}`,
      headers: portalHeaders,
      baseUrl
    });
  },

  // ===== 充值 / 撤销 =====
  rechargeUser(body: {
    userId: string;
    paidAmount?: number;
    creditAmount: number;
    note?: string;
    expireTime?: number | null;
  }) {
    return urm()<RechargeOutput>({
      method: "POST",
      path: "/api/v1/recharges",
      headers: portalHeaders,
      body: { packageType: 2, ...body },
      baseUrl
    });
  },
  reverseRecharge(orderId: string, body: { reason: string }) {
    return urm()<ReverseRechargeOutput>({
      method: "POST",
      path: `/api/v1/recharges/${encodeURIComponent(orderId)}/reverse`,
      headers: portalHeaders,
      body,
      baseUrl
    });
  },

  // ===== 邀请码 =====
  getInviteCodes(params: { page?: number; size?: number }) {
    return urm()<Page<InviteCodeItem>>({
      method: "GET",
      path: "/api/v1/invitations",
      headers: portalHeaders,
      query: params,
      baseUrl
    });
  },
  createInviteCode(body: { description?: string; max_uses?: number; expire_time?: number | null }) {
    return urm()<CreateInviteCodeOutput>({
      method: "POST",
      path: "/api/v1/invitations",
      headers: portalHeaders,
      body,
      baseUrl
    });
  },
  updateInviteCode(id: number, body: { status: number; description?: string }) {
    return urm()<{ success: boolean }>({
      method: "PUT",
      path: `/api/v1/invitations/${encodeURIComponent(String(id))}`,
      headers: portalHeaders,
      body,
      baseUrl
    });
  },
  deleteInviteCode(id: number) {
    return urm()<{ success: boolean }>({
      method: "DELETE",
      path: `/api/v1/invitations/${encodeURIComponent(String(id))}`,
      headers: portalHeaders,
      baseUrl
    });
  }
};
