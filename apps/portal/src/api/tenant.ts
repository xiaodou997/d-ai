import { authenticatedRequest, apiHeaders, apiBaseUrl } from "./request";
import {
  createTypedOperationRequest,
  type OperationBody,
  type OperationQuery,
  type OperationResponse
} from ".";
import type {
  CreateTenantInvitationOutputBody,
  CreateTenantEndUserOutputBody,
  PageTenantBalanceLedgerItem,
  PageTenantEndUserItem,
  PageTenantInvitationItem,
  PageTenantRechargeRecordItem,
  PageTenantTopupOrderItem,
  TenantAiApiKey,
  TenantAiApiKeysOutputBody,
  TenantBalanceAccount,
  TenantClientConsumptionItem,
  TenantEndUserItem,
  TenantOverviewStats,
  TenantPortalBranding,
  TenantRechargeOutputBody,
  TenantPaymentSettings,
  TenantTopupConfig,
  TenantTopupOrderCreated,
  TenantTopupOrderItem,
  TenantTopupOrderStatus,
  TopupPackage
} from "./types/tenant";
import type { components } from "./generated/dai";

const typedRequest = createTypedOperationRequest(authenticatedRequest());
const baseUrl = apiBaseUrl;

function stripSchema<T>(value: T): Omit<T, "$schema"> {
  const { $schema: _schema, ...rest } = value as T & { $schema?: string };
  return rest as Omit<T, "$schema">;
}

type BrandingTransport = OperationResponse<"tenant-get-branding">;
type AnalyticsOverviewTransport = OperationResponse<"tenant-analytics-overview">;
type ClientConsumptionTransport = OperationResponse<"tenant-analytics-app-consumption">;
type RechargeRecordsTransport = OperationResponse<"account-recharge-records">;
type RechargeRecordTransport = NonNullable<RechargeRecordsTransport["items"]>[number];
type RechargeTransport = OperationResponse<"admin-recharge">;
type EndUserPageTransport = OperationResponse<"admin-list-end-users">;
type EndUserTransport = NonNullable<EndUserPageTransport["items"]>[number];
type InvitationPageTransport = OperationResponse<"tenant-list-invitations">;
type InvitationTransport = NonNullable<InvitationPageTransport["items"]>[number];
type ApiKeyPageTransport = OperationResponse<"ai-list-tenant-self-api-keys">;
type ApiKeyTransport = NonNullable<ApiKeyPageTransport["items"]>[number];
type TopupConfigTransport = OperationResponse<"payment-topup-config">;
type TopupOrderCreatedTransport = OperationResponse<"payment-create-topup-order">;
type TopupOrderStatusTransport = OperationResponse<"payment-get-topup-order">;
type TopupOrderPageTransport = OperationResponse<"payment-list-topup-orders">;
type TopupOrderTransport = NonNullable<TopupOrderPageTransport["items"]>[number];
type TopupPackageTransport = components["schemas"]["TopupPackage"];
type PaymentSettingsTransport = OperationResponse<"tenant-get-payment-settings">;
type PaymentSettingsBody = OperationBody<"tenant-update-payment-settings">;
type TenantAnalyticsQuery = OperationQuery<"tenant-analytics-overview">;
type RechargeRecordsQuery = OperationQuery<"account-recharge-records">;
type EndUsersQuery = OperationQuery<"admin-list-end-users">;
type InvitationsQuery = OperationQuery<"tenant-list-invitations">;
type TopupOrdersQuery = OperationQuery<"payment-list-topup-orders">;

function toBranding(value: BrandingTransport): TenantPortalBranding {
  return {
    tenantName: value.tenantName,
    customerSiteName: value.customerSiteName,
    faviconPath: value.faviconPath
  };
}

function toAnalyticsOverview(value: AnalyticsOverviewTransport): TenantOverviewStats {
  return stripSchema(value);
}

function toClientConsumption(value: ClientConsumptionTransport): TenantClientConsumptionItem[] {
  return value?.map((item) => ({
    clientId: item.clientId,
    clientName: item.clientName,
    amountUsd: item.amountUsd,
    percentage: item.percentage
  })) ?? [];
}

function toRechargeRecord(value: RechargeRecordTransport): PageTenantRechargeRecordItem["items"][number] {
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

function toRechargeRecords(value: RechargeRecordsTransport): PageTenantRechargeRecordItem {
  return {
    items: value.items?.map(toRechargeRecord) ?? [],
    total: value.total,
    page: value.page,
    size: value.size
  };
}

function toRecharge(value: RechargeTransport): TenantRechargeOutputBody {
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

function toCredentialState(value: string): TenantEndUserItem["credentialState"] {
  if (value === "active" || value === "pending_activation") return value;
  throw new Error(`Unexpected credential state: ${value}`);
}

function toEndUser(value: EndUserTransport): TenantEndUserItem {
  return {
    userId: value.userId,
    tenantId: value.tenantId,
    tenantName: value.tenantName,
    username: value.username,
    email: value.email,
    phone: value.phone,
    nickname: value.nickname,
    avatar: value.avatar,
    status: value.status,
    credentialState: toCredentialState(value.credentialState),
    balanceUsd: value.balanceUsd,
    lastLoginTime: value.lastLoginTime,
    createdTime: value.createdTime
  };
}

function toEndUserPage(value: EndUserPageTransport): PageTenantEndUserItem {
  return {
    items: value.items?.map(toEndUser) ?? [],
    total: value.total,
    page: value.page,
    size: value.size
  };
}

function toCreatedEndUser(value: OperationResponse<"admin-create-end-user">): CreateTenantEndUserOutputBody {
  return {
    userId: value.userId,
    tenantId: value.tenantId,
    username: value.username,
    activationToken: value.activationToken,
    activationExpiresIn: value.activationExpiresIn
  };
}

function toActivation(value: OperationResponse<"admin-reset-end-user-password">) {
  return {
    activationToken: value.activationToken,
    activationExpiresIn: value.activationExpiresIn
  };
}

function toInvitation(value: InvitationTransport): PageTenantInvitationItem["items"][number] {
  return {
    id: value.id,
    code: value.code,
    tenantId: value.tenantId,
    createdBy: value.createdBy,
    description: value.description,
    maxUses: value.maxUses,
    usedCount: value.usedCount,
    status: value.status,
    expireTime: value.expireTime,
    createdTime: value.createdTime,
    updatedTime: value.updatedTime
  };
}

function toInvitationPage(value: InvitationPageTransport): PageTenantInvitationItem {
  return {
    items: value.items?.map(toInvitation) ?? [],
    total: value.total,
    page: value.page,
    size: value.size
  };
}

function toCreatedInvitation(value: OperationResponse<"tenant-create-invitation">): CreateTenantInvitationOutputBody {
  return {
    code: value.code,
    tenantId: value.tenantId,
    description: value.description,
    maxUses: value.maxUses,
    expireTime: value.expireTime
  };
}

function toApiKey(value: ApiKeyTransport): TenantAiApiKey {
  return {
    id: value.id,
    owner_type: value.owner_type,
    tenant_id: value.tenant_id,
    user_id: value.user_id,
    group_id: value.group_id,
    last_four: value.last_four,
    name: value.name,
    quota_limit_micro_usd: value.quota_limit_micro_usd,
    quota_used_micro_usd: value.quota_used_micro_usd,
    status: value.status,
    expires_at: value.expires_at,
    last_used_at: value.last_used_at,
    limit_policy: value.limit_policy
      ? {
          id: value.limit_policy.id,
          scope_type: value.limit_policy.scope_type,
          scope_id: value.limit_policy.scope_id,
          concurrency_limit: value.limit_policy.concurrency_limit,
          status: value.limit_policy.status
        }
      : undefined,
    created_by: value.created_by,
    created_at: value.created_at,
    updated_at: value.updated_at
  };
}

function toApiKeyPage(value: ApiKeyPageTransport): TenantAiApiKeysOutputBody {
  return {
    items: value.items?.map(toApiKey) ?? [],
    total: value.total
  };
}

function toTopupPackage(value: TopupPackageTransport): TopupPackage {
  return {
    id: value.id,
    name: value.name,
    paymentAmountMicroUsd: value.paymentAmountMicroUsd,
    giftAmountMicroUsd: value.giftAmountMicroUsd,
    validityDays: value.validityDays ?? null,
    badge: value.badge,
    enabled: value.enabled,
    sortOrder: value.sortOrder
  };
}

function toTopupConfig(value: TopupConfigTransport): TenantTopupConfig {
  return {
    enabled: value.enabled,
    currency: value.currency,
    feeRateBp: value.feeRateBp,
    minMicroUsd: value.minMicroUsd,
    maxMicroUsd: value.maxMicroUsd,
    validityDays: value.validityDays ?? null,
    packages: value.packages?.map(toTopupPackage) ?? []
  };
}

function toTopupMode(value: string): TenantTopupOrderItem["topupMode"] {
  if (value === "custom" || value === "package") return value;
  throw new Error(`Unexpected topup mode: ${value}`);
}

function toTopupStatus(value: string): TenantTopupOrderStatus["status"] {
  if (value === "created" || value === "paying" || value === "paid" || value === "closed" || value === "expired") {
    return value;
  }
  throw new Error(`Unexpected topup status: ${value}`);
}

function toTopupScene(value: string): NonNullable<TenantTopupOrderItem["scene"]> {
  if (value === "user_topup" || value === "tenant_topup") return value;
  throw new Error(`Unexpected topup scene: ${value}`);
}

function toTopupOrderCreated(value: TopupOrderCreatedTransport): TenantTopupOrderCreated {
  return {
    orderId: value.orderId,
    codeUrl: value.codeUrl,
    paymentCurrency: value.paymentCurrency,
    paymentAmountMinor: value.paymentAmountMinor,
    grossAmountMicroUsd: value.grossAmountMicroUsd,
    feeAmountMicroUsd: value.feeAmountMicroUsd,
    giftAmountMicroUsd: value.giftAmountMicroUsd,
    creditedAmountMicroUsd: value.creditedAmountMicroUsd,
    topupMode: toTopupMode(value.topupMode),
    packageName: value.packageName,
    status: value.status,
    expiresAt: value.expiresAt,
    balanceExpiresAt: value.balanceExpiresAt
  };
}

function toTopupOrderStatus(value: TopupOrderStatusTransport): TenantTopupOrderStatus {
  return {
    orderId: value.orderId,
    status: toTopupStatus(value.status),
    paymentCurrency: value.paymentCurrency,
    paymentAmountMinor: value.paymentAmountMinor,
    grossAmountMicroUsd: value.grossAmountMicroUsd,
    feeAmountMicroUsd: value.feeAmountMicroUsd,
    giftAmountMicroUsd: value.giftAmountMicroUsd,
    creditedAmountMicroUsd: value.creditedAmountMicroUsd,
    topupMode: toTopupMode(value.topupMode),
    packageName: value.packageName,
    transactionId: value.transactionId,
    paidAt: value.paidAt,
    balanceExpiresAt: value.balanceExpiresAt
  };
}

function toTopupOrder(value: TopupOrderTransport): TenantTopupOrderItem {
  return {
    orderId: value.orderId,
    scene: toTopupScene(value.scene),
    status: value.status,
    paymentCurrency: value.paymentCurrency,
    paymentAmountMinor: value.paymentAmountMinor,
    grossAmountMicroUsd: value.grossAmountMicroUsd,
    feeAmountMicroUsd: value.feeAmountMicroUsd,
    giftAmountMicroUsd: value.giftAmountMicroUsd,
    creditedAmountMicroUsd: value.creditedAmountMicroUsd,
    topupMode: toTopupMode(value.topupMode),
    packageName: value.packageName,
    transactionId: value.transactionId,
    createdAt: value.createdAt,
    paidAt: value.paidAt,
    balanceExpiresAt: value.balanceExpiresAt
  };
}

function toTopupOrderPage(value: TopupOrderPageTransport): PageTenantTopupOrderItem {
  return {
    items: value.items?.map(toTopupOrder) ?? [],
    total: value.total,
    page: value.page,
    size: value.size
  };
}

function toPaymentSettings(value: PaymentSettingsTransport): TenantPaymentSettings {
  return {
    userCustomTopupFeeBp: value.userCustomTopupFeeBp,
    userCustomValidityDays: value.userCustomValidityDays ?? null,
    userTopupPackages: value.userTopupPackages?.map(toTopupPackage) ?? []
  };
}

function toPaymentSettingsBody(value: TenantPaymentSettings): PaymentSettingsBody {
  return {
    userCustomTopupFeeBp: value.userCustomTopupFeeBp,
    userCustomValidityDays: value.userCustomValidityDays ?? undefined,
    userTopupPackages: value.userTopupPackages.map((item) => ({
      id: item.id,
      name: item.name,
      paymentAmountMicroUsd: item.paymentAmountMicroUsd,
      giftAmountMicroUsd: item.giftAmountMicroUsd,
      validityDays: item.validityDays ?? undefined,
      badge: item.badge,
      enabled: item.enabled,
      sortOrder: item.sortOrder
    }))
  };
}

function toBalance(value: OperationResponse<"tenant-balance">): TenantBalanceAccount {
  return { currency: value.currency, balanceMicroUsd: value.balanceMicroUsd };
}

function toBalanceLedger(value: OperationResponse<"tenant-balance-ledger">): PageTenantBalanceLedgerItem {
  return {
    items: value.items?.map((item) => ({
      txnId: item.txnId,
      txnType: item.txnType,
      currency: item.currency,
      amountMicroUsd: item.amountMicroUsd,
      balanceAfterMicroUsd: item.balanceAfterMicroUsd,
      refType: item.refType,
      refId: item.refId,
      note: item.note,
      createdAt: item.createdAt
    })) ?? [],
    total: value.total,
    page: value.page,
    size: value.size
  };
}

export const tenantApi = {
  getPortalBranding() {
    return typedRequest<"tenant-get-branding">({
      method: "GET",
      path: "/api/v1/tenant/branding",
      headers: apiHeaders,
      baseUrl
    }).then(toBranding);
  },
  updatePortalBranding(body: Pick<TenantPortalBranding, "tenantName" | "customerSiteName">) {
    return typedRequest<"tenant-update-branding">({
      method: "PUT",
      path: "/api/v1/tenant/branding",
      headers: apiHeaders,
      body,
      baseUrl
    }).then(toBranding);
  },
  updatePortalFavicon(dataUrl: string) {
    return typedRequest<"tenant-update-branding-favicon">({
      method: "PUT",
      path: "/api/v1/tenant/branding/favicon",
      headers: apiHeaders,
      body: { dataUrl },
      baseUrl
    }).then(toBranding);
  },
  deletePortalFavicon() {
    return typedRequest<"tenant-delete-branding-favicon">({
      method: "DELETE",
      path: "/api/v1/tenant/branding/favicon",
      headers: apiHeaders,
      baseUrl
    }).then(toBranding);
  },
  getOverview(params: TenantAnalyticsQuery = {}) {
    return typedRequest<"tenant-analytics-overview">({
      method: "GET",
      path: "/api/v1/tenants/analytics/overview",
      headers: apiHeaders,
      query: params,
      baseUrl
    }).then(toAnalyticsOverview);
  },
  listClientConsumption(params: TenantAnalyticsQuery = {}) {
    return typedRequest<"tenant-analytics-app-consumption">({
      method: "GET",
      path: "/api/v1/tenants/analytics/app-consumption",
      headers: apiHeaders,
      query: params,
      baseUrl
    }).then(toClientConsumption);
  },
  listRechargeRecords(
    params: Pick<RechargeRecordsQuery, "page" | "size" | "username" | "rechargeType"> = {}
  ) {
    return typedRequest<"account-recharge-records">({
      method: "GET",
      path: "/api/v1/account/recharge-records",
      headers: apiHeaders,
      query: params,
      baseUrl
    }).then(toRechargeRecords);
  },
  createUserRecharge(body: {
    userId: string;
    paidAmountMinor?: number;
    amountMicroUsd: number;
    paymentRef?: string;
    note?: string;
  }) {
    const requestBody: OperationBody<"admin-recharge"> = {
      packageType: 2,
      userId: body.userId,
      paidAmountMinor: body.paidAmountMinor,
      amountMicroUsd: body.amountMicroUsd,
      paymentRef: body.paymentRef,
      note: body.note
    };
    return typedRequest<"admin-recharge">({
      method: "POST",
      path: "/api/v1/recharges",
      headers: apiHeaders,
      body: requestBody,
      baseUrl
    }).then(toRecharge);
  },
  listEndUsers(params: Pick<EndUsersQuery, "page" | "size" | "keyword">) {
    return typedRequest<"admin-list-end-users">({
      method: "GET",
      path: "/api/v1/users",
      headers: apiHeaders,
      query: params,
      baseUrl
    }).then(toEndUserPage);
  },
  createEndUser(body: { username: string; email?: string; phone?: string }) {
    const requestBody: OperationBody<"admin-create-end-user"> = {
      username: body.username,
      email: body.email,
      phone: body.phone
    };
    return typedRequest<"admin-create-end-user">({
      method: "POST",
      path: "/api/v1/users",
      headers: apiHeaders,
      body: requestBody,
      baseUrl
    }).then(toCreatedEndUser);
  },
  updateEndUserStatus(userId: string, status: OperationBody<"admin-update-end-user-status">["status"]) {
    return typedRequest<"admin-update-end-user-status">({
      method: "PATCH",
      path: `/api/v1/users/${encodeURIComponent(userId)}/status`,
      pathParams: { id: userId },
      headers: apiHeaders,
      body: { status },
      baseUrl
    }).then((value) => ({ message: value.message }));
  },
  resetEndUserPassword(userId: string) {
    return typedRequest<"admin-reset-end-user-password">({
      method: "POST",
      path: `/api/v1/users/${encodeURIComponent(userId)}/reset-password`,
      pathParams: { id: userId },
      headers: apiHeaders,
      baseUrl
    }).then(toActivation);
  },
  listInvitations(params: InvitationsQuery) {
    return typedRequest<"tenant-list-invitations">({
      method: "GET",
      path: "/api/v1/invitations",
      headers: apiHeaders,
      query: params,
      baseUrl
    }).then(toInvitationPage);
  },
  createInvitation(body: { description?: string; maxUses?: number; expireTime?: number | null }) {
    const requestBody: OperationBody<"tenant-create-invitation"> = {
      description: body.description,
      max_uses: body.maxUses,
      expire_time: body.expireTime
    };
    return typedRequest<"tenant-create-invitation">({
      method: "POST",
      path: "/api/v1/invitations",
      headers: apiHeaders,
      body: requestBody,
      baseUrl
    }).then(toCreatedInvitation);
  },
  updateInvitation(id: number, body: Pick<OperationBody<"tenant-update-invitation">, "status" | "description">) {
    return typedRequest<"tenant-update-invitation">({
      method: "PUT",
      path: `/api/v1/invitations/${encodeURIComponent(String(id))}`,
      pathParams: { id },
      headers: apiHeaders,
      body,
      baseUrl
    }).then((value) => ({ success: value.success }));
  },
  deleteInvitation(id: number) {
    return typedRequest<"tenant-delete-invitation">({
      method: "DELETE",
      path: `/api/v1/invitations/${encodeURIComponent(String(id))}`,
      pathParams: { id },
      headers: apiHeaders,
      baseUrl
    }).then((value) => ({ success: value.success }));
  },
  listAiTenantApiKeys() {
    return typedRequest<"ai-list-tenant-self-api-keys">({
      method: "GET",
      path: "/api/v1/tenant-api-keys",
      headers: apiHeaders,
      baseUrl
    }).then(toApiKeyPage);
  },

  // ===== 在线充值（微信支付） =====
  getTopupConfig() {
    return typedRequest<"payment-topup-config">({
      method: "GET",
      path: "/api/v1/payments/topup-config",
      headers: apiHeaders,
      baseUrl
    }).then(toTopupConfig);
  },
  createTopupOrder(body: OperationBody<"payment-create-topup-order">) {
    return typedRequest<"payment-create-topup-order">({
      method: "POST",
      path: "/api/v1/payments/topup-orders",
      headers: apiHeaders,
      body,
      baseUrl
    }).then(toTopupOrderCreated);
  },
  getTopupOrder(orderId: string) {
    return typedRequest<"payment-get-topup-order">({
      method: "GET",
      path: `/api/v1/payments/topup-orders/${encodeURIComponent(orderId)}`,
      pathParams: { orderId },
      headers: apiHeaders,
      baseUrl
    }).then(toTopupOrderStatus);
  },
  listTopupOrders(params: Pick<TopupOrdersQuery, "page" | "size"> = {}) {
    return typedRequest<"payment-list-topup-orders">({
      method: "GET",
      path: "/api/v1/payments/topup-orders",
      headers: apiHeaders,
      query: params,
      baseUrl
    }).then(toTopupOrderPage);
  },

  getPaymentSettings() {
    return typedRequest<"tenant-get-payment-settings">({
      method: "GET",
      path: "/api/v1/tenant/payment-settings",
      headers: apiHeaders,
      baseUrl
    }).then(toPaymentSettings);
  },
  updatePaymentSettings(body: TenantPaymentSettings) {
    return typedRequest<"tenant-update-payment-settings">({
      method: "PUT",
      path: "/api/v1/tenant/payment-settings",
      headers: apiHeaders,
      body: toPaymentSettingsBody(body),
      baseUrl
    }).then(toPaymentSettings);
  },

  // ===== 统一 USD 余额 =====
  getBalanceAccount() {
    return typedRequest<"tenant-balance">({
      method: "GET",
      path: "/api/v1/tenant/balance",
      headers: apiHeaders,
      baseUrl
    }).then(toBalance);
  },
  listBalanceLedger(
    params: Pick<OperationQuery<"tenant-balance-ledger">, "txnType" | "page" | "size"> = {}
  ) {
    return typedRequest<"tenant-balance-ledger">({
      method: "GET",
      path: "/api/v1/tenant/balance-ledger",
      headers: apiHeaders,
      query: params,
      baseUrl
    }).then(toBalanceLedger);
  }
};
