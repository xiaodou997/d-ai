// Portal 租户自助业务 API。
import { authenticatedRequest, apiHeaders, apiBaseUrl } from "./request";
import {
  createTypedOperationRequest,
  type OperationBody,
  type OperationResponse
} from ".";
import { toAccountBalance, toRechargePage } from "./accountMappers";
import type {
  ActivationCredentialOutput,
  ClientConsumptionItem,
  CreateEndUserOutput,
  CreateInviteCodeOutput,
  EndUserItem,
  InviteCodeItem,
  Page,
  RechargeOutput,
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

type AnalyticsOverviewTransport = OperationResponse<"tenant-analytics-overview">;
type EndUserPageTransport = OperationResponse<"admin-list-end-users">;
type EndUserTransport = NonNullable<EndUserPageTransport["items"]>[number];
type InvitationPageTransport = OperationResponse<"tenant-list-invitations">;
type InvitationTransport = NonNullable<InvitationPageTransport["items"]>[number];
type RechargeUserInput = Omit<
  OperationBody<"admin-recharge">,
  "$schema" | "packageType" | "tenantId"
> & {
  userId: string;
  amountMicroUsd: number;
};

function toAnalyticsOverview(value: AnalyticsOverviewTransport): TenantAnalyticsOverview {
  return {
    endUserCount: value.endUserCount,
    inviteCodeCount: value.inviteCodeCount,
    userDeductionUsd: value.userDeductionUsd,
    userTotalBalanceUsd: value.userTotalBalanceUsd,
    activeUserCount: value.activeUserCount,
    userConsumptionCount: value.userConsumptionCount,
    settlementIncomeMicroUsd: value.settlementIncomeMicroUsd
  };
}

function toEndUserStatus(value: number): EndUserItem["status"] {
  if (value === 1 || value === 2) return value;
  throw new Error(`Unexpected end-user status: ${value}`);
}

function toCredentialState(value: string): EndUserItem["credentialState"] {
  if (value === "active" || value === "pending_activation") return value;
  throw new Error(`Unexpected credential state: ${value}`);
}

function toEndUser(value: EndUserTransport): EndUserItem {
  return {
    userId: value.userId,
    tenantId: value.tenantId,
    username: value.username,
    tenantName: value.tenantName,
    email: value.email,
    phone: value.phone,
    internalNote: value.internalNote,
    nickname: value.nickname,
    avatar: value.avatar,
    status: toEndUserStatus(value.status),
    credentialState: toCredentialState(value.credentialState),
    balanceUsd: value.balanceUsd,
    lastLoginTime: value.lastLoginTime,
    createdTime: value.createdTime
  };
}

function toEndUserPage(value: EndUserPageTransport): Page<EndUserItem> {
  return {
    items: value.items?.map(toEndUser) ?? [],
    total: value.total,
    page: value.page,
    size: value.size
  };
}

function toInviteStatus(value: number): InviteCodeItem["status"] {
  if (value === 1 || value === 2) return value;
  throw new Error(`Unexpected invitation status: ${value}`);
}

function toInviteCode(value: InvitationTransport): InviteCodeItem {
  return {
    id: value.id,
    code: value.code,
    registrationUrl: value.registrationUrl,
    tenantId: value.tenantId,
    createdBy: value.createdBy,
    description: value.description,
    maxUses: value.maxUses,
    usedCount: value.usedCount,
    status: toInviteStatus(value.status),
    expireTime: value.expireTime,
    createdTime: value.createdTime,
    updatedTime: value.updatedTime
  };
}

function toInvitationPage(value: InvitationPageTransport): Page<InviteCodeItem> {
  return {
    items: value.items?.map(toInviteCode) ?? [],
    total: value.total,
    page: value.page,
    size: value.size
  };
}

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
    return typedRequest<"tenant-analytics-overview">({
      method: "GET",
      path: "/api/v1/tenants/analytics/overview",
      headers: apiHeaders,
      query: params,
      baseUrl
    }).then(toAnalyticsOverview);
  },
  getAppConsumption(params: { timeFrom?: number; timeTo?: number } = {}) {
    return typedRequest<"tenant-analytics-app-consumption">({
      method: "GET",
      path: "/api/v1/tenants/analytics/app-consumption",
      headers: apiHeaders,
      query: params,
      baseUrl
    }).then((items): ClientConsumptionItem[] => items?.map((item) => ({ ...item })) ?? []);
  },
  getUserConsumption(params: { timeFrom?: number; timeTo?: number; limit?: number } = {}) {
    return typedRequest<"tenant-analytics-user-consumption">({
      method: "GET",
      path: "/api/v1/tenants/analytics/user-consumption",
      headers: apiHeaders,
      query: params,
      baseUrl
    }).then((items): UserConsumptionItem[] => items?.map((item) => ({ ...item })) ?? []);
  },

  // ===== 统一账户 =====
  getAccountBalance(detail = true) {
    return typedRequest<"account-balance">({
      method: "GET",
      path: "/api/v1/account/balance",
      headers: apiHeaders,
      query: { detail },
      baseUrl
    }).then(toAccountBalance);
  },
  getRechargeRecords(params: {
    page?: number;
    size?: number;
    username?: string;
    rechargeType?: string;
    timeFrom?: number;
    timeTo?: number;
  }) {
    return typedRequest<"account-recharge-records">({
      method: "GET",
      path: "/api/v1/account/recharge-records",
      headers: apiHeaders,
      query: params,
      baseUrl
    }).then(toRechargePage);
  },
  // 用户充值记录：租户充用户（rechargeType=2）
  getUserRechargeRecords(params: { page?: number; size?: number; username?: string; timeFrom?: number; timeTo?: number }) {
    return typedRequest<"account-recharge-records">({
      method: "GET",
      path: "/api/v1/account/recharge-records",
      headers: apiHeaders,
      query: { ...params, rechargeType: "2" },
      baseUrl
    }).then(toRechargePage);
  },

  // ===== 终端用户 =====
  getUsers(params: { keyword?: string; page?: number; size?: number }) {
    return typedRequest<"admin-list-end-users">({
      method: "GET",
      path: "/api/v1/users",
      headers: apiHeaders,
      query: params,
      baseUrl
    }).then(toEndUserPage);
  },
  createEndUser(body: OperationBody<"admin-create-end-user">) {
    return typedRequest<"admin-create-end-user">({
      method: "POST",
      path: "/api/v1/users",
      headers: apiHeaders,
      body,
      baseUrl
    }).then(
      (value): CreateEndUserOutput => ({
        userId: value.userId,
        tenantId: value.tenantId,
        username: value.username,
        activationToken: value.activationToken,
        activationExpiresIn: value.activationExpiresIn
      })
    );
  },
  updateEndUser(userId: string, body: OperationBody<"admin-update-end-user">) {
    return typedRequest<"admin-update-end-user">({
      method: "PATCH",
      path: `/api/v1/users/${encodeURIComponent(userId)}`,
      pathParams: { id: userId },
      headers: apiHeaders,
      body,
      baseUrl
    }).then((value) => ({ message: value.message }));
  },
  updateUserStatus(userId: string, status: OperationBody<"admin-update-end-user-status">["status"]) {
    return typedRequest<"admin-update-end-user-status">({
      method: "PATCH",
      path: `/api/v1/users/${encodeURIComponent(userId)}/status`,
      pathParams: { id: userId },
      headers: apiHeaders,
      body: { status },
      baseUrl
    }).then((value) => ({ message: value.message }));
  },
  resetUserPassword(userId: string) {
    return typedRequest<"admin-reset-end-user-password">({
      method: "POST",
      path: `/api/v1/users/${encodeURIComponent(userId)}/reset-password`,
      pathParams: { id: userId },
      headers: apiHeaders,
      baseUrl
    }).then(
      (value): ActivationCredentialOutput => ({
        activationToken: value.activationToken,
        activationExpiresIn: value.activationExpiresIn
      })
    );
  },
  deleteEndUser(userId: string) {
    return typedRequest<"admin-delete-end-user">({
      method: "DELETE",
      path: `/api/v1/users/${encodeURIComponent(userId)}`,
      pathParams: { id: userId },
      headers: apiHeaders,
      baseUrl
    }).then((value) => ({ success: value.success }));
  },

  // ===== 充值 / 撤销 =====
  rechargeUser(body: RechargeUserInput) {
    return typedRequest<"admin-recharge">({
      method: "POST",
      path: "/api/v1/recharges",
      headers: apiHeaders,
      body: { packageType: 2, ...body },
      baseUrl
    }).then(
      (value): RechargeOutput => ({
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
      })
    );
  },
  reverseRecharge(orderId: string, body: OperationBody<"admin-reverse-recharge">) {
    return typedRequest<"admin-reverse-recharge">({
      method: "POST",
      path: `/api/v1/recharges/${encodeURIComponent(orderId)}/reverse`,
      pathParams: { orderId },
      headers: apiHeaders,
      body,
      baseUrl
    }).then(
      (value): ReverseRechargeOutput => ({
        status: value.status,
        orderId: value.orderId,
        balanceLotId: value.balanceLotId,
        reversedAmountUsd: value.reversedAmountUsd,
        originalAmountUsd: value.originalAmountUsd,
        lostAmountUsd: value.lostAmountUsd,
        balanceLotStatus: value.balanceLotStatus
      })
    );
  },

  // ===== 邀请码 =====
  getInviteCodes(params: { page?: number; size?: number }) {
    return typedRequest<"tenant-list-invitations">({
      method: "GET",
      path: "/api/v1/invitations",
      headers: apiHeaders,
      query: params,
      baseUrl
    }).then(toInvitationPage);
  },
  createInviteCode(body: OperationBody<"tenant-create-invitation">) {
    return typedRequest<"tenant-create-invitation">({
      method: "POST",
      path: "/api/v1/invitations",
      headers: apiHeaders,
      body,
      baseUrl
    }).then(
      (value): CreateInviteCodeOutput => ({
        code: value.code,
        registrationUrl: value.registrationUrl,
        tenantId: value.tenantId,
        description: value.description,
        maxUses: value.maxUses,
        expireTime: value.expireTime
      })
    );
  },
  updateInviteCode(id: number, body: OperationBody<"tenant-update-invitation">) {
    return typedRequest<"tenant-update-invitation">({
      method: "PUT",
      path: `/api/v1/invitations/${encodeURIComponent(String(id))}`,
      pathParams: { id },
      headers: apiHeaders,
      body,
      baseUrl
    }).then((value) => ({ success: value.success }));
  },
  deleteInviteCode(id: number) {
    return typedRequest<"tenant-delete-invitation">({
      method: "DELETE",
      path: `/api/v1/invitations/${encodeURIComponent(String(id))}`,
      pathParams: { id },
      headers: apiHeaders,
      baseUrl
    }).then((value) => ({ success: value.success }));
  }
};
