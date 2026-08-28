// Platform 用户（终端用户 type=4）自助业务 API —— 1:1 还原 v1 platform-customer src/api/account.js 调用，
// 仅 API 层适配 v4 huma 扁平端点（无 {code,data} 信封，列表={items,total,page,size}）。
// 用户端统一使用 D-AI 的 /api/v1 业务端点和 /api/auth 账号端点。
// 终端用户分支由后端按 claims.UserType==4 自动锁定本人范围，前端无需传 userId。
import { authenticatedRequest, apiHeaders, apiBaseUrl } from "./request";
import {
  createTypedOperationRequest,
  type OperationBody,
  type OperationResponse
} from ".";
import { toAccountBalance, toRechargePage } from "./accountMappers";
import type {
  AccountBalance,
  CustomerPortalBrand,
  Page,
  RechargeRecordItem,
  TopupConfig,
  TopupOrderCreated,
  TopupOrderItem,
  TopupOrderStatus
} from "./types/platformCustomer";
import type { ChangePasswordPayload, ProfileUpdateInput, UpdateProfilePayload } from "./types/auth";

function platform() {
  return authenticatedRequest();
}

const baseUrl = apiBaseUrl;
const typedRequest = createTypedOperationRequest(platform());

type PortalBrandTransport = OperationResponse<"customer-get-portal-brand">;
type TopupConfigTransport = OperationResponse<"payment-topup-config">;
type TopupOrderCreatedTransport = OperationResponse<"payment-create-topup-order">;
type TopupOrderStatusTransport = OperationResponse<"payment-get-topup-order">;
type TopupOrderPageTransport = OperationResponse<"payment-list-topup-orders">;
type TopupOrderItemTransport = NonNullable<TopupOrderPageTransport["items"]>[number];

function toPortalBrand(value: PortalBrandTransport): CustomerPortalBrand {
  return { siteName: value.siteName, faviconPath: value.faviconPath };
}

function toTopupMode(value: string): TopupOrderCreated["topupMode"] {
  if (value === "custom" || value === "package") return value;
  throw new Error(`Unexpected top-up mode: ${value}`);
}

function toTopupStatus(value: string): TopupOrderStatus["status"] {
  switch (value) {
    case "created":
    case "paying":
    case "paid":
    case "closed":
    case "expired":
      return value;
    default:
      throw new Error(`Unexpected top-up status: ${value}`);
  }
}

function toTopupScene(value: string): NonNullable<TopupOrderItem["scene"]> {
  if (value === "user_topup" || value === "tenant_topup") return value;
  throw new Error(`Unexpected top-up scene: ${value}`);
}

function toTopupConfig(value: TopupConfigTransport): TopupConfig {
  return {
    enabled: value.enabled,
    currency: value.currency,
    feeRateBp: value.feeRateBp,
    minMicroUsd: value.minMicroUsd,
    maxMicroUsd: value.maxMicroUsd,
    validityDays: value.validityDays,
    packages: value.packages?.map((item) => ({ ...item })) ?? []
  };
}

function toTopupOrderCreated(value: TopupOrderCreatedTransport): TopupOrderCreated {
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

function toTopupOrderStatus(value: TopupOrderStatusTransport): TopupOrderStatus {
  return {
    orderId: value.orderId,
    paymentCurrency: value.paymentCurrency,
    paymentAmountMinor: value.paymentAmountMinor,
    grossAmountMicroUsd: value.grossAmountMicroUsd,
    feeAmountMicroUsd: value.feeAmountMicroUsd,
    giftAmountMicroUsd: value.giftAmountMicroUsd,
    creditedAmountMicroUsd: value.creditedAmountMicroUsd,
    topupMode: toTopupMode(value.topupMode),
    packageName: value.packageName,
    status: toTopupStatus(value.status),
    balanceExpiresAt: value.balanceExpiresAt,
    transactionId: value.transactionId,
    paidAt: value.paidAt
  };
}

function toTopupOrderItem(value: TopupOrderItemTransport): TopupOrderItem {
  return {
    orderId: value.orderId,
    paymentCurrency: value.paymentCurrency,
    paymentAmountMinor: value.paymentAmountMinor,
    grossAmountMicroUsd: value.grossAmountMicroUsd,
    feeAmountMicroUsd: value.feeAmountMicroUsd,
    giftAmountMicroUsd: value.giftAmountMicroUsd,
    creditedAmountMicroUsd: value.creditedAmountMicroUsd,
    topupMode: toTopupMode(value.topupMode),
    packageName: value.packageName,
    status: toTopupStatus(value.status),
    balanceExpiresAt: value.balanceExpiresAt,
    transactionId: value.transactionId,
    paidAt: value.paidAt,
    scene: toTopupScene(value.scene),
    createdAt: value.createdAt
  };
}

function toTopupOrderPage(value: TopupOrderPageTransport): Page<TopupOrderItem> {
  return {
    items: value.items?.map(toTopupOrderItem) ?? [],
    total: value.total,
    page: value.page,
    size: value.size
  };
}

export const platformCustomerApi = {
  getPortalBrand() {
    return typedRequest<"customer-get-portal-brand">({
      method: "GET",
      path: "/api/v1/customer/portal-brand",
      headers: apiHeaders,
      baseUrl
    }).then(toPortalBrand);
  },
  // 我的 USD 余额（含有效期批次明细）
  getBalance(detail = true) {
    return typedRequest<"account-balance">({
      method: "GET",
      path: "/api/v1/account/balance",
      headers: apiHeaders,
      query: { detail },
      baseUrl
    }).then(toAccountBalance);
  },

  // 我的充值记录（分页）
  getRechargeRecords(params: { page?: number; size?: number }) {
    return typedRequest<"account-recharge-records">({
      method: "GET",
      path: "/api/v1/account/recharge-records",
      headers: apiHeaders,
      query: params,
      baseUrl
    }).then(toRechargePage);
  },

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

  // USD 在线充值配置
  getTopupConfig() {
    return typedRequest<"payment-topup-config">({
      method: "GET",
      path: "/api/v1/payments/topup-config",
      headers: apiHeaders,
      baseUrl
    }).then(toTopupConfig);
  },

  // 发起在线充值（微信 Native 扫码）
  createTopupOrder(body: OperationBody<"payment-create-topup-order">) {
    return typedRequest<"payment-create-topup-order">({
      method: "POST",
      path: "/api/v1/payments/topup-orders",
      headers: apiHeaders,
      body,
      baseUrl
    }).then(toTopupOrderCreated);
  },

  // 查询充值订单状态（轮询用）
  getTopupOrder(orderId: string) {
    return typedRequest<"payment-get-topup-order">({
      method: "GET",
      path: `/api/v1/payments/topup-orders/${encodeURIComponent(orderId)}`,
      pathParams: { orderId },
      headers: apiHeaders,
      baseUrl
    }).then(toTopupOrderStatus);
  },

  // 我的在线充值订单（分页）
  listTopupOrders(params: { page?: number; size?: number } = {}) {
    return typedRequest<"payment-list-topup-orders">({
      method: "GET",
      path: "/api/v1/payments/topup-orders",
      headers: apiHeaders,
      query: params,
      baseUrl
    }).then(toTopupOrderPage);
  }
};
