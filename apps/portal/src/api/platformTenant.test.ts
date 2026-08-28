import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { platformTenantApi } from "./platformTenant";

const analytics = {
  endUserCount: 2,
  inviteCodeCount: 1,
  userDeductionUsd: 3,
  userTotalBalanceUsd: 7,
  activeUserCount: 1,
  userConsumptionCount: 4,
  settlementIncomeMicroUsd: 5_000_000
};

const balance = {
  currency: "USD",
  totalUsd: 10,
  usedUsd: 2,
  remainingUsd: 8,
  availableUsd: 8,
  permanentUsd: 8,
  timedUsd: 0,
  outstandingDebtMicroUsd: 0,
  serviceState: "active",
  balanceLots: null
};

const endUser = {
  userId: "user-1",
  tenantId: "tenant-1",
  username: "alice",
  status: 1,
  credentialState: "active",
  balanceUsd: 8,
  createdTime: 100
};

const recharge = {
  orderId: "recharge-1",
  balanceLotId: "lot-1",
  tenantId: "tenant-1",
  userId: "user-1",
  currency: "USD",
  amountMicroUsd: 1_000_000,
  paidAmountMinor: 100,
  clearedDebtUsd: 0,
  balanceLotUsd: 1,
  orderTime: 100
};

const invitation = {
  id: 7,
  code: "INVITE",
  tenantId: "tenant-1",
  createdBy: "admin-1",
  description: "test",
  maxUses: 10,
  usedCount: 0,
  status: 1,
  createdTime: 100,
  updatedTime: 100
};

beforeEach(() => mocks.request.mockReset());

describe("platform tenant generated operation facade", () => {
  it("normalizes analytics and account collections", async () => {
    mocks.request
      .mockResolvedValueOnce({ ...analytics, $schema: "ignored" })
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce(balance)
      .mockResolvedValueOnce({ items: null, total: 0, page: 1, size: 20 })
      .mockResolvedValueOnce({ items: null, total: 0, page: 1, size: 20 });

    await expect(platformTenantApi.getAnalyticsOverview()).resolves.toEqual(analytics);
    await expect(platformTenantApi.getAppConsumption()).resolves.toEqual([]);
    await expect(platformTenantApi.getUserConsumption()).resolves.toEqual([]);
    await expect(platformTenantApi.getAccountBalance()).resolves.toEqual({
      ...balance,
      balanceLots: undefined
    });
    await expect(platformTenantApi.getRechargeRecords({ page: 1, size: 20 })).resolves.toMatchObject({
      items: []
    });
    await expect(platformTenantApi.getUserRechargeRecords({ page: 1, size: 20 })).resolves.toMatchObject({
      items: []
    });

    expect(mocks.request.mock.calls[5]?.[0]).toMatchObject({
      query: { page: 1, size: 20, rechargeType: "2" }
    });
  });

  it("maps scoped end-user operations and path parameters", async () => {
    mocks.request
      .mockResolvedValueOnce({ items: [endUser], total: 1, page: 1, size: 20 })
      .mockResolvedValueOnce({
        userId: "user-1",
        tenantId: "tenant-1",
        username: "alice",
        activationToken: "activate",
        activationExpiresIn: 3600
      })
      .mockResolvedValueOnce({ message: "updated" })
      .mockResolvedValueOnce({ message: "disabled" })
      .mockResolvedValueOnce({ activationToken: "reset", activationExpiresIn: 3600 })
      .mockResolvedValueOnce({ success: true });

    await expect(platformTenantApi.getUsers({ page: 1, size: 20 })).resolves.toMatchObject({
      items: [{ userId: "user-1", credentialState: "active" }]
    });
    await expect(platformTenantApi.createEndUser({ username: "alice" })).resolves.toMatchObject({
      activationToken: "activate"
    });
    await expect(
      platformTenantApi.updateEndUser("user/1", { email: null, phone: null, internalNote: null })
    ).resolves.toEqual({ message: "updated" });
    await expect(platformTenantApi.updateUserStatus("user/1", "disabled")).resolves.toEqual({
      message: "disabled"
    });
    await expect(platformTenantApi.resetUserPassword("user/1")).resolves.toMatchObject({
      activationToken: "reset"
    });
    await expect(platformTenantApi.deleteEndUser("user/1")).resolves.toEqual({ success: true });

    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({
      path: "/api/v1/users/user%2F1",
      pathParams: { id: "user/1" }
    });
  });

  it("forces tenant user recharge scope and maps reversal output", async () => {
    mocks.request
      .mockResolvedValueOnce(recharge)
      .mockResolvedValueOnce({
        status: "reversed",
        orderId: "recharge-1",
        balanceLotId: "lot-1",
        reversedAmountUsd: 1,
        originalAmountUsd: 1,
        lostAmountUsd: 0,
        balanceLotStatus: "reversed"
      });

    await expect(
      platformTenantApi.rechargeUser({ userId: "user-1", amountMicroUsd: 1_000_000 })
    ).resolves.toEqual(recharge);
    await expect(
      platformTenantApi.reverseRecharge("recharge/1", { reason: "duplicate" })
    ).resolves.toMatchObject({ status: "reversed", lostAmountUsd: 0 });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      body: { packageType: 2, userId: "user-1", amountMicroUsd: 1_000_000 }
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      pathParams: { orderId: "recharge/1" }
    });
  });

  it("normalizes invitation pages and uses numeric path parameters", async () => {
    mocks.request
      .mockResolvedValueOnce({ items: null, total: 0, page: 1, size: 20 })
      .mockResolvedValueOnce({
        code: "INVITE",
        tenantId: "tenant-1",
        description: "test",
        maxUses: 10
      })
      .mockResolvedValueOnce({ success: true })
      .mockResolvedValueOnce({ success: true });

    await expect(platformTenantApi.getInviteCodes({ page: 1, size: 20 })).resolves.toMatchObject({
      items: []
    });
    await expect(platformTenantApi.createInviteCode({ max_uses: 10 })).resolves.toMatchObject({
      code: "INVITE"
    });
    await expect(platformTenantApi.updateInviteCode(7, { status: 2 })).resolves.toEqual({ success: true });
    await expect(platformTenantApi.deleteInviteCode(7)).resolves.toEqual({ success: true });

    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({ pathParams: { id: 7 } });
  });

  it("rejects invalid end-user and invitation states", async () => {
    mocks.request
      .mockResolvedValueOnce({
        items: [{ ...endUser, credentialState: "unknown" }],
        total: 1,
        page: 1,
        size: 20
      })
      .mockResolvedValueOnce({
        items: [{ ...invitation, status: 9 }],
        total: 1,
        page: 1,
        size: 20
      });

    await expect(platformTenantApi.getUsers({})).rejects.toThrow(
      "Unexpected credential state: unknown"
    );
    await expect(platformTenantApi.getInviteCodes({})).rejects.toThrow(
      "Unexpected invitation status: 9"
    );
  });
});
