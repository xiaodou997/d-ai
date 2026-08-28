import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/api",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { platformAdminApi } from "./platformAdmin";

beforeEach(() => mocks.request.mockReset());

const credit = {
  balanceOrderId: "balance-order-1",
  orderType: "ADMIN_RECHARGE",
  primary: true,
  creditAmountMicroUsd: 1_000_000,
  status: "credited",
  note: "manual",
  balanceExpiresAt: undefined,
  reversedAt: undefined,
  reversedAmountMicroUsd: 0,
  lostAmountMicroUsd: 0,
  grantedAmountMicroUsd: 1_000_000,
  consumedAmountMicroUsd: 100_000,
  remainingAmountMicroUsd: 900_000,
  lotStatus: "active",
  refundAvailableMicroUsd: 0,
  refundNonAvailableMicroUsd: 0,
  refundExpiredMicroUsd: 0,
  refundAccountDebitMicroUsd: 0,
  refundBalanceAfterMicroUsd: 0
};

const refund = {
  refundId: "refund-1",
  method: "wechat",
  refundReference: "refund-ref",
  channelRefundId: "wechat-ref",
  refundAmountMinor: 100,
  status: "completed",
  refundedAt: 200,
  reason: "duplicate",
  note: "done",
  operatorId: "admin-1",
  createdAt: 200
};

const order = {
  orderId: "order-1",
  balanceOrderId: "balance-order-1",
  method: "online",
  targetType: "tenant",
  orderType: "ONLINE_TOPUP",
  tenantId: "tenant-1",
  tenantName: "Tenant One",
  userId: undefined,
  username: undefined,
  paidAmountMinor: 100,
  grossAmountMicroUsd: 1_000_000,
  feeAmountMicroUsd: 10_000,
  giftAmountMicroUsd: 100_000,
  creditedAmountMicroUsd: 1_090_000,
  tenantIncomeMicroUsd: 50_000,
  paymentStatus: "paid",
  fulfillmentStatus: "credited",
  refundStatus: "refunded",
  outTradeNo: "out-1",
  transactionId: "tx-1",
  topupMode: "package",
  packageName: "Starter",
  channel: "wechat",
  note: "note",
  failNote: undefined,
  createdAt: 100,
  paidAt: 110,
  paymentExpiresAt: 120,
  balanceExpiresAt: 130,
  reversedAt: undefined,
  reversedBy: undefined,
  reversalReason: undefined,
  credits: [credit],
  refund,
  $schema: "ignored"
};

describe("platform admin recharge-order generated operation facade", () => {
  it("maps nullable order pages and nested credit/refund view models", async () => {
    mocks.request.mockResolvedValueOnce({ items: [order], total: 1, page: 1, size: 20, $schema: "ignored" });

    await expect(platformAdminApi.listAdminRechargeOrders({
      method: "online",
      targetType: "tenant",
      refundStatus: "refunded",
      page: 1,
      size: 20
    })).resolves.toMatchObject({
      items: [{
        orderId: "order-1",
        method: "online",
        targetType: "tenant",
        paymentStatus: "paid",
        fulfillmentStatus: "credited",
        refundStatus: "refunded",
        credits: [{ balanceOrderId: "balance-order-1", remainingAmountMicroUsd: 900_000 }],
        refund: { method: "wechat", status: "completed" }
      }],
      total: 1,
      page: 1,
      size: 20
    });
    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/admin/recharge-orders",
      query: { method: "online", targetType: "tenant", refundStatus: "refunded", page: 1, size: 20 }
    });
    expect(mocks.request.mock.calls[0]?.[0]).not.toHaveProperty("body");
  });

  it("binds order detail, sync, reversal and refund paths with generated bodies", async () => {
    mocks.request.mockResolvedValueOnce(order).mockResolvedValueOnce(order).mockResolvedValueOnce(order).mockResolvedValueOnce(order);

    await expect(platformAdminApi.getAdminRechargeOrder("order/1")).resolves.toMatchObject({ orderId: "order-1" });
    await expect(platformAdminApi.syncAdminRechargeOrder("order/1")).resolves.toMatchObject({ orderId: "order-1" });
    await expect(platformAdminApi.reverseAdminRechargeOrderCredit("order/1", { reason: "duplicate" })).resolves.toMatchObject({ orderId: "order-1" });
    await expect(platformAdminApi.recordCompletedRechargeRefund("order/1", {
      method: "wechat",
      refundReference: "refund-ref",
      channelRefundId: "wechat-ref",
      refundedAt: 200,
      reason: "duplicate",
      note: "done"
    })).resolves.toMatchObject({ orderId: "order-1" });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/admin/recharge-orders/order%2F1",
      pathParams: { orderId: "order/1" }
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/admin/recharge-orders/order%2F1/sync",
      pathParams: { orderId: "order/1" }
    });
    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({
      path: "/api/v1/admin/recharge-orders/order%2F1/reverse-credit",
      pathParams: { orderId: "order/1" },
      body: { reason: "duplicate" }
    });
    expect(mocks.request.mock.calls[3]?.[0]).toMatchObject({
      path: "/api/v1/admin/recharge-orders/order%2F1/refund-reversal",
      pathParams: { orderId: "order/1" },
      body: { method: "wechat", refundReference: "refund-ref", refundedAt: 200 }
    });
  });

  it("normalizes nullable balance-ledger rows and preserves ledger fields", async () => {
    mocks.request.mockResolvedValueOnce({
      items: null,
      total: 0,
      page: 1,
      size: 20,
      $schema: "ignored"
    });

    await expect(platformAdminApi.listBalanceLedger({ tenantId: "tenant/1", txnType: "topup_income", page: 1, size: 20 })).resolves.toEqual({
      items: [],
      total: 0,
      page: 1,
      size: 20
    });
    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/admin/balance-ledger",
      query: { tenantId: "tenant/1", txnType: "topup_income", page: 1, size: 20 }
    });
  });

  it("rejects unknown order dimensions before exposing them to pages", async () => {
    mocks.request.mockResolvedValueOnce({ items: [{ ...order, method: "transfer" }], total: 1, page: 1, size: 20 });
    await expect(platformAdminApi.listAdminRechargeOrders()).rejects.toThrow("Unexpected recharge method");

    mocks.request.mockReset();
    mocks.request.mockResolvedValueOnce({ items: [{ ...order, refund: { ...refund, status: "pending" } }], total: 1, page: 1, size: 20 });
    await expect(platformAdminApi.listAdminRechargeOrders()).rejects.toThrow("Unexpected refund status");
  });
});
