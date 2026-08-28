// Portal 租户自助业务 API。
import { authenticatedRequest, apiHeaders, apiBaseUrl } from "./request";
import { createTypedOperationRequest } from ".";
import type {
  AccountBalance,
  ActivationCredentialOutput,
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
} from "./types/platformTenant";
import type { ChangePasswordPayload, ProfileUpdateInput, UpdateProfilePayload } from "./types/auth";

function platform() {
  return authenticatedRequest();
}

const baseUrl = apiBaseUrl;
const typedRequest = createTypedOperationRequest(platform());

export const platformTenantApi = {
  // ===== 账号自助 =====
  // 修改密码（后端统一密码策略）
  changePassword(body: ChangePasswordPayload) {
    return typedRequest<"auth-change-password">({
      method: "PUT",
      path: "/api/auth/password",
      headers: apiHeaders,
      body,
      baseUrl
    });
  },

  updateProfile(body: ProfileUpdateInput) {
    return typedRequest<"auth-update-profile">({
      method: "PUT",
      path: "/api/auth/profile",
      headers: apiHeaders,
      body: {
        username: body.username ?? null,
        email: body.email ?? null
      } satisfies UpdateProfilePayload,
      baseUrl
    });
  },

  // ===== 统计 / 概览 =====
  getAnalyticsOverview(params: { timeFrom?: number; timeTo?: number } = {}) {
    return platform()<TenantAnalyticsOverview>({
      method: "GET",
      path: "/api/v1/tenants/analytics/overview",
      headers: apiHeaders,
      query: params,
      baseUrl
    });
  },
  getAppConsumption(params: { timeFrom?: number; timeTo?: number } = {}) {
    return platform()<ClientConsumptionItem[]>({
      method: "GET",
      path: "/api/v1/tenants/analytics/app-consumption",
      headers: apiHeaders,
      query: params,
      baseUrl
    });
  },
  getUserConsumption(params: { timeFrom?: number; timeTo?: number; limit?: number } = {}) {
    return platform()<UserConsumptionItem[]>({
      method: "GET",
      path: "/api/v1/tenants/analytics/user-consumption",
      headers: apiHeaders,
      query: params,
      baseUrl
    });
  },

  // ===== 统一账户 =====
  getAccountBalance(detail = true) {
    return platform()<AccountBalance>({
      method: "GET",
      path: "/api/v1/account/balance",
      headers: apiHeaders,
      query: { detail },
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
    return platform()<Page<RechargeRecordItem>>({
      method: "GET",
      path: "/api/v1/account/recharge-records",
      headers: apiHeaders,
      query: params,
      baseUrl
    });
  },
  // 用户充值记录：租户充用户（rechargeType=2）
  getUserRechargeRecords(params: { page?: number; size?: number; username?: string; timeFrom?: number; timeTo?: number }) {
    return platform()<Page<RechargeRecordItem>>({
      method: "GET",
      path: "/api/v1/account/recharge-records",
      headers: apiHeaders,
      query: { ...params, rechargeType: "2" },
      baseUrl
    });
  },

  // ===== 终端用户 =====
  getUsers(params: { keyword?: string; page?: number; size?: number }) {
    return platform()<Page<EndUserItem>>({
      method: "GET",
      path: "/api/v1/users",
      headers: apiHeaders,
      query: params,
      baseUrl
    });
  },
  createEndUser(body: { username: string; email?: string; phone?: string; internalNote?: string }) {
    return platform()<CreateEndUserOutput>({
      method: "POST",
      path: "/api/v1/users",
      headers: apiHeaders,
      body,
      baseUrl
    });
  },
  updateEndUser(userId: string, body: { email: string; phone: string; internalNote: string }) {
    return platform()<{ message: string }>({
      method: "PATCH",
      path: `/api/v1/users/${encodeURIComponent(userId)}`,
      headers: apiHeaders,
      body,
      baseUrl
    });
  },
  updateUserStatus(userId: string, status: "active" | "disabled") {
    return platform()<{ message: string }>({
      method: "PATCH",
      path: `/api/v1/users/${encodeURIComponent(userId)}/status`,
      headers: apiHeaders,
      body: { status },
      baseUrl
    });
  },
  resetUserPassword(userId: string) {
    return platform()<ActivationCredentialOutput>({
      method: "POST",
      path: `/api/v1/users/${encodeURIComponent(userId)}/reset-password`,
      headers: apiHeaders,
      baseUrl
    });
  },
  deleteEndUser(userId: string) {
    return platform()<{ success: boolean }>({
      method: "DELETE",
      path: `/api/v1/users/${encodeURIComponent(userId)}`,
      headers: apiHeaders,
      baseUrl
    });
  },

  // ===== 充值 / 撤销 =====
  rechargeUser(body: {
    userId: string;
    paidAmountMinor?: number;
    amountMicroUsd: number;
    note?: string;
    expireTime?: number | null;
  }) {
    return platform()<RechargeOutput>({
      method: "POST",
      path: "/api/v1/recharges",
      headers: apiHeaders,
      body: { packageType: 2, ...body },
      baseUrl
    });
  },
  reverseRecharge(orderId: string, body: { reason: string }) {
    return platform()<ReverseRechargeOutput>({
      method: "POST",
      path: `/api/v1/recharges/${encodeURIComponent(orderId)}/reverse`,
      headers: apiHeaders,
      body,
      baseUrl
    });
  },

  // ===== 邀请码 =====
  getInviteCodes(params: { page?: number; size?: number }) {
    return platform()<Page<InviteCodeItem>>({
      method: "GET",
      path: "/api/v1/invitations",
      headers: apiHeaders,
      query: params,
      baseUrl
    });
  },
  createInviteCode(body: { description?: string; max_uses?: number; expire_time?: number | null }) {
    return platform()<CreateInviteCodeOutput>({
      method: "POST",
      path: "/api/v1/invitations",
      headers: apiHeaders,
      body,
      baseUrl
    });
  },
  updateInviteCode(id: number, body: { status: number; description?: string }) {
    return platform()<{ success: boolean }>({
      method: "PUT",
      path: `/api/v1/invitations/${encodeURIComponent(String(id))}`,
      headers: apiHeaders,
      body,
      baseUrl
    });
  },
  deleteInviteCode(id: number) {
    return platform()<{ success: boolean }>({
      method: "DELETE",
      path: `/api/v1/invitations/${encodeURIComponent(String(id))}`,
      headers: apiHeaders,
      baseUrl
    });
  }
};
