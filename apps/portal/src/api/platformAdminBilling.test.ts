import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/api",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { platformAdminApi } from "./platformAdmin";

beforeEach(() => mocks.request.mockReset());

const rechargeOutput = {
  orderId: "order-1",
  balanceLotId: "lot-1",
  tenantId: "tenant-1",
  userId: "",
  currency: "USD",
  amountMicroUsd: 1_000_000,
  paidAmountMinor: 100,
  clearedDebtUsd: 0,
  balanceLotUsd: 1,
  orderTime: 100,
  $schema: "ignored"
};

describe("platform admin billing generated operation facade", () => {
  it("maps account balance and recharge records through generated queries", async () => {
    mocks.request
      .mockResolvedValueOnce({
        currency: "USD",
        totalUsd: 200,
        usedUsd: 80,
        remainingUsd: 120,
        availableUsd: 120,
        permanentUsd: 70,
        timedUsd: 50,
        outstandingDebtMicroUsd: 25_000_000,
        serviceState: "blocked_debt",
        balanceLots: null,
        $schema: "ignored"
      })
      .mockResolvedValueOnce({
        items: [{
          orderId: "order-1",
          orderType: "ADMIN_RECHARGE",
          paidAmountMinor: 100,
          amountUsd: 1,
          status: "completed",
          note: "manual",
          userId: "",
          username: "",
          tenantName: "Tenant One",
          createdTime: null
        }],
        total: 1,
        page: 1,
        size: 20,
        $schema: "ignored"
      });

    await expect(platformAdminApi.getAccountBalance({ accountType: 1, accountId: "tenant/1", detail: true })).resolves.toEqual({
      currency: "USD",
      totalUsd: 200,
      usedUsd: 80,
      remainingUsd: 120,
      availableUsd: 120,
      permanentUsd: 70,
      timedUsd: 50,
      outstandingDebtMicroUsd: 25_000_000,
      serviceState: "blocked_debt",
      balanceLots: []
    });
    await expect(platformAdminApi.listRechargeRecords({ tenantName: "Tenant One", page: 1, size: 20 })).resolves.toEqual({
      items: [{
        orderId: "order-1",
        orderType: "ADMIN_RECHARGE",
        paidAmountMinor: 100,
        amountUsd: 1,
        status: "completed",
        note: "manual",
        userId: "",
        username: "",
        tenantName: "Tenant One",
        createdTime: undefined
      }],
      total: 1,
      page: 1,
      size: 20
    });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/account/balance",
      query: { accountType: 1, accountId: "tenant/1", detail: true }
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/account/recharge-records",
      query: { tenantName: "Tenant One", page: 1, size: 20 }
    });
  });

  it("binds recharge, reversal, refund and debt operations with typed bodies and paths", async () => {
    mocks.request
      .mockResolvedValueOnce(rechargeOutput)
      .mockResolvedValueOnce({
        orderId: "order-1",
        balanceLotId: "lot-1",
        reversedAmountUsd: 0.8,
        originalAmountUsd: 1,
        lostAmountUsd: 0.2,
        balanceLotStatus: "depleted",
        status: "PARTIAL_REVERSAL",
        $schema: "ignored"
      })
      .mockResolvedValueOnce({ message: "退款成功", $schema: "ignored" })
      .mockResolvedValueOnce({
        succeeded: null,
        failed: null,
        totalTenantUsd: 0,
        totalUserUsd: 1.2,
        successCount: 0,
        failCount: 1,
        $schema: "ignored"
      })
      .mockResolvedValueOnce({
        owner_type: "tenant",
        account_id: "tenant-1",
        outstanding_debt_micro_usd: 12,
        service_state: "active",
        $schema: "ignored"
      });

    await expect(platformAdminApi.createRecharge({
      packageType: 1,
      tenantId: "tenant-1",
      amountMicroUsd: 1_000_000,
      paidAmountMinor: 100,
      expireTime: null
    })).resolves.toMatchObject({ orderId: "order-1", amountMicroUsd: 1_000_000 });
    await expect(platformAdminApi.reverseRecharge("order/1", { reason: "duplicate" })).resolves.toMatchObject({
      status: "PARTIAL_REVERSAL",
      lostAmountUsd: 0.2
    });
    await expect(platformAdminApi.refundUsage({ requestId: "request-1", reason: "duplicate" })).resolves.toEqual({ message: "退款成功" });
    await expect(platformAdminApi.batchRefundUsage({ requestIds: ["request-1"], reason: "duplicate" })).resolves.toEqual({
      succeeded: [],
      failed: [],
      totalTenantUsd: 0,
      totalUserUsd: 1.2,
      successCount: 0,
      failCount: 1
    });
    await expect(platformAdminApi.getDebtStatus("tenant", "tenant/1")).resolves.toEqual({
      owner_type: "tenant",
      account_id: "tenant-1",
      outstanding_debt_micro_usd: 12,
      service_state: "active"
    });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/recharges",
      body: { packageType: 1, tenantId: "tenant-1", amountMicroUsd: 1_000_000, expireTime: null }
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/recharges/order%2F1/reverse",
      pathParams: { orderId: "order/1" },
      body: { reason: "duplicate" }
    });
    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({ body: { requestId: "request-1", reason: "duplicate" } });
    expect(mocks.request.mock.calls[3]?.[0]).toMatchObject({ body: { requestIds: ["request-1"], reason: "duplicate" } });
    expect(mocks.request.mock.calls[4]?.[0]).toMatchObject({
      path: "/api/v1/admin/debts/tenant/tenant%2F1",
      pathParams: { owner_type: "tenant", id: "tenant/1" }
    });
  });

  it("rejects malformed balance and debt states at the facade boundary", async () => {
    mocks.request.mockResolvedValueOnce({
      currency: "USD",
      totalUsd: 0,
      usedUsd: 0,
      remainingUsd: 0,
      availableUsd: 0,
      permanentUsd: 0,
      timedUsd: 0,
      outstandingDebtMicroUsd: 0,
      serviceState: "unknown"
    });
    await expect(platformAdminApi.getAccountBalance({ accountType: 1, accountId: "tenant-1" })).rejects.toThrow(
      "Unexpected account balance service state"
    );

    mocks.request.mockReset();
    mocks.request.mockResolvedValueOnce({
      owner_type: "tenant",
      account_id: "tenant-1",
      outstanding_debt_micro_usd: 0,
      service_state: "unknown"
    });
    await expect(platformAdminApi.getDebtStatus("tenant", "tenant-1")).rejects.toThrow("Unexpected debt service state");
  });
});
