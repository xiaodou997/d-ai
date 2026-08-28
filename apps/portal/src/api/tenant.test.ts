import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/api",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { tenantApi } from "./tenant";

beforeEach(() => mocks.request.mockReset());

const branding = {
  tenantName: "Tenant One",
  customerSiteName: "Customer Portal",
  faviconPath: "/favicon.png",
  $schema: "ignored"
};

const endUser = {
  userId: "user-1",
  tenantId: "tenant-1",
  tenantName: "Tenant One",
  username: "alice",
  email: "alice@example.com",
  phone: "123",
  nickname: "Alice",
  avatar: "/avatar.png",
  status: 1,
  credentialState: "active",
  balanceUsd: 3,
  lastLoginTime: 120,
  createdTime: 100
};

describe("tenant generated operation facade", () => {
  it("maps branding, analytics, recharge, user and invitation operations", async () => {
    mocks.request
      .mockResolvedValueOnce(branding)
      .mockResolvedValueOnce(branding)
      .mockResolvedValueOnce({ ...branding, faviconPath: "/new.png" })
      .mockResolvedValueOnce({ ...branding, faviconPath: undefined })
      .mockResolvedValueOnce({
        endUserCount: 2,
        inviteCodeCount: 1,
        userDeductionUsd: 4,
        userTotalBalanceUsd: 8,
        activeUserCount: 1,
        userConsumptionCount: 3,
        settlementIncomeMicroUsd: 5_000_000,
        $schema: "ignored"
      })
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce({
        items: [{
          orderId: "order-1",
          orderType: "TENANT_RECHARGE",
          paidAmountMinor: 100,
          amountUsd: 1,
          status: "completed",
          note: "manual",
          userId: "user-1",
          username: "alice",
          tenantName: "Tenant One",
          createdTime: null
        }],
        total: 1,
        page: 1,
        size: 20
      })
      .mockResolvedValueOnce({
        orderId: "order-2",
        balanceLotId: "lot-1",
        tenantId: "tenant-1",
        userId: "user-1",
        currency: "USD",
        amountMicroUsd: 1_000_000,
        paidAmountMinor: 100,
        clearedDebtUsd: 0,
        balanceLotUsd: 1,
        orderTime: 130,
        $schema: "ignored"
      })
      .mockResolvedValueOnce({ items: [endUser], total: 1, page: 1, size: 20 })
      .mockResolvedValueOnce({
        userId: "user-2",
        tenantId: "tenant-1",
        username: "bob",
        activationToken: "activate",
        activationExpiresIn: 3600
      })
      .mockResolvedValueOnce({ message: "disabled", $schema: "ignored" })
      .mockResolvedValueOnce({ activationToken: "reset", activationExpiresIn: 1800 })
      .mockResolvedValueOnce({
        items: [{
          id: 7,
          code: "INVITE",
          tenantId: "tenant-1",
          createdBy: "admin-1",
          description: "test",
          maxUses: 10,
          usedCount: 1,
          status: 1,
          createdTime: 100,
          updatedTime: 110
        }],
        total: 1,
        page: 1,
        size: 20
      })
      .mockResolvedValueOnce({
        code: "INVITE-2",
        tenantId: "tenant-1",
        description: "new",
        maxUses: 5,
        expireTime: 500
      })
      .mockResolvedValueOnce({ success: true })
      .mockResolvedValueOnce({ success: true });

    await expect(tenantApi.getPortalBranding()).resolves.toEqual({
      tenantName: "Tenant One",
      customerSiteName: "Customer Portal",
      faviconPath: "/favicon.png"
    });
    await expect(tenantApi.updatePortalBranding({ tenantName: "Tenant One", customerSiteName: "Portal" })).resolves.toMatchObject({
      customerSiteName: "Customer Portal"
    });
    await expect(tenantApi.updatePortalFavicon("data:image/png;base64,abc")).resolves.toMatchObject({ faviconPath: "/new.png" });
    await expect(tenantApi.deletePortalFavicon()).resolves.toMatchObject({ faviconPath: undefined });
    await expect(tenantApi.getOverview({ timeFrom: 10, timeTo: 20 })).resolves.toMatchObject({ endUserCount: 2 });
    await expect(tenantApi.listClientConsumption()).resolves.toEqual([]);
    await expect(tenantApi.listRechargeRecords({ username: "alice", page: 1, size: 20 })).resolves.toEqual({
      items: [{
        orderId: "order-1",
        orderType: "TENANT_RECHARGE",
        paidAmountMinor: 100,
        amountUsd: 1,
        status: "completed",
        note: "manual",
        userId: "user-1",
        username: "alice",
        tenantName: "Tenant One",
        createdTime: undefined
      }],
      total: 1,
      page: 1,
      size: 20
    });
    await expect(tenantApi.createUserRecharge({ userId: "user-1", amountMicroUsd: 1_000_000 })).resolves.toMatchObject({ orderId: "order-2" });
    await expect(tenantApi.listEndUsers({ keyword: "alice", page: 1, size: 20 })).resolves.toMatchObject({
      items: [{ userId: "user-1", credentialState: "active" }]
    });
    await expect(tenantApi.createEndUser({ username: "bob" })).resolves.toMatchObject({ activationToken: "activate" });
    await expect(tenantApi.updateEndUserStatus("user/1", "disabled")).resolves.toEqual({ message: "disabled" });
    await expect(tenantApi.resetEndUserPassword("user/1")).resolves.toEqual({ activationToken: "reset", activationExpiresIn: 1800 });
    await expect(tenantApi.listInvitations({ page: 1, size: 20 })).resolves.toMatchObject({ items: [{ id: 7 }] });
    await expect(tenantApi.createInvitation({ description: "new", maxUses: 5, expireTime: 500 })).resolves.toMatchObject({ code: "INVITE-2" });
    await expect(tenantApi.updateInvitation(7, { status: 2, description: "updated" })).resolves.toEqual({ success: true });
    await expect(tenantApi.deleteInvitation(7)).resolves.toEqual({ success: true });

    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({
      path: "/api/v1/tenant/branding/favicon",
      body: { dataUrl: "data:image/png;base64,abc" }
    });
    expect(mocks.request.mock.calls[7]?.[0]).toMatchObject({
      path: "/api/v1/recharges",
      body: { packageType: 2, userId: "user-1", amountMicroUsd: 1_000_000 }
    });
    expect(mocks.request.mock.calls[10]?.[0]).toMatchObject({
      path: "/api/v1/users/user%2F1/status",
      pathParams: { id: "user/1" }
    });
    expect(mocks.request.mock.calls[15]?.[0]).toMatchObject({
      path: "/api/v1/invitations/7",
      pathParams: { id: 7 }
    });
  });

  it("maps tenant keys, online topups, payment settings and balance ledgers", async () => {
    mocks.request
      .mockResolvedValueOnce({
        items: [{
          id: "key-1",
          owner_type: "tenant",
          tenant_id: "tenant-1",
          group_id: "group-1",
          last_four: "1234",
          name: "primary",
          quota_used_micro_usd: 10,
          status: "active",
          limit_policy: {
            id: "limit-1",
            scope_type: "api_key",
            scope_id: "key-1",
            concurrency_limit: 2,
            status: "active",
            $schema: "nested"
          },
          $schema: "ignored"
        }],
        total: 1
      })
      .mockResolvedValueOnce({
        enabled: true,
        currency: "USD",
        feeRateBp: 160,
        minMicroUsd: 1_000_000,
        maxMicroUsd: 100_000_000,
        validityDays: undefined,
        packages: [{
          id: "p10",
          name: "$10",
          paymentAmountMicroUsd: 10_000_000,
          giftAmountMicroUsd: 0,
          enabled: true,
          sortOrder: 10,
          $schema: "ignored"
        }]
      })
      .mockResolvedValueOnce({
        orderId: "order-1",
        codeUrl: "weixin://pay/1",
        paymentCurrency: "CNY",
        paymentAmountMinor: 100,
        grossAmountMicroUsd: 10_000_000,
        feeAmountMicroUsd: 160_000,
        giftAmountMicroUsd: 0,
        creditedAmountMicroUsd: 9_840_000,
        topupMode: "custom",
        status: "created",
        expiresAt: 200
      })
      .mockResolvedValueOnce({
        orderId: "order-1",
        status: "paid",
        paymentCurrency: "CNY",
        paymentAmountMinor: 100,
        grossAmountMicroUsd: 10_000_000,
        feeAmountMicroUsd: 160_000,
        giftAmountMicroUsd: 0,
        creditedAmountMicroUsd: 9_840_000,
        topupMode: "custom",
        transactionId: "tx-1",
        paidAt: 210
      })
      .mockResolvedValueOnce({
        items: [{
          orderId: "order-1",
          scene: "tenant_topup",
          status: "paid",
          paymentCurrency: "CNY",
          paymentAmountMinor: 100,
          grossAmountMicroUsd: 10_000_000,
          feeAmountMicroUsd: 160_000,
          giftAmountMicroUsd: 0,
          creditedAmountMicroUsd: 9_840_000,
          topupMode: "custom",
          createdAt: 100
        }],
        total: 1,
        page: 1,
        size: 20
      })
      .mockResolvedValueOnce({
        userCustomTopupFeeBp: 160,
        userCustomValidityDays: undefined,
        userTopupPackages: null
      })
      .mockResolvedValueOnce({
        userCustomTopupFeeBp: 200,
        userCustomValidityDays: 30,
        userTopupPackages: []
      })
      .mockResolvedValueOnce({ currency: "USD", balanceMicroUsd: 12_000_000, $schema: "ignored" })
      .mockResolvedValueOnce({
        items: null,
        total: 0,
        page: 1,
        size: 20,
        $schema: "ignored"
      });

    await expect(tenantApi.listAiTenantApiKeys()).resolves.toEqual({
      items: [{
        id: "key-1",
        owner_type: "tenant",
        tenant_id: "tenant-1",
        user_id: undefined,
        group_id: "group-1",
        last_four: "1234",
        name: "primary",
        quota_limit_micro_usd: undefined,
        quota_used_micro_usd: 10,
        status: "active",
        expires_at: undefined,
        last_used_at: undefined,
        limit_policy: {
          id: "limit-1",
          scope_type: "api_key",
          scope_id: "key-1",
          concurrency_limit: 2,
          status: "active"
        },
        created_by: undefined,
        created_at: undefined,
        updated_at: undefined
      }],
      total: 1
    });
    await expect(tenantApi.getTopupConfig()).resolves.toMatchObject({ validityDays: null, packages: [{ id: "p10", validityDays: null }] });
    await expect(tenantApi.createTopupOrder({ amountMicroUsd: 10_000_000 })).resolves.toMatchObject({
      orderId: "order-1",
      topupMode: "custom"
    });
    await expect(tenantApi.getTopupOrder("order/1")).resolves.toMatchObject({ status: "paid" });
    await expect(tenantApi.listTopupOrders({ page: 1, size: 20 })).resolves.toMatchObject({
      items: [{ scene: "tenant_topup", topupMode: "custom" }]
    });
    await expect(tenantApi.getPaymentSettings()).resolves.toEqual({
      userCustomTopupFeeBp: 160,
      userCustomValidityDays: null,
      userTopupPackages: []
    });
    await expect(tenantApi.updatePaymentSettings({
      userCustomTopupFeeBp: 200,
      userCustomValidityDays: null,
      userTopupPackages: []
    })).resolves.toMatchObject({ userCustomTopupFeeBp: 200, userCustomValidityDays: 30 });
    await expect(tenantApi.getBalanceAccount()).resolves.toEqual({ currency: "USD", balanceMicroUsd: 12_000_000 });
    await expect(tenantApi.listBalanceLedger({ txnType: "topup", page: 1, size: 20 })).resolves.toEqual({
      items: [],
      total: 0,
      page: 1,
      size: 20
    });

    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({
      path: "/api/v1/payments/topup-orders",
      body: { amountMicroUsd: 10_000_000 }
    });
    expect(mocks.request.mock.calls[3]?.[0]).toMatchObject({
      path: "/api/v1/payments/topup-orders/order%2F1",
      pathParams: { orderId: "order/1" }
    });
    expect(mocks.request.mock.calls[6]?.[0]).toMatchObject({
      path: "/api/v1/tenant/payment-settings",
      body: { userCustomTopupFeeBp: 200, userCustomValidityDays: undefined, userTopupPackages: [] }
    });
  });

  it("rejects malformed generated enum values at the facade boundary", async () => {
    mocks.request.mockResolvedValueOnce({
      items: [{ ...endUser, credentialState: "unknown" }],
      total: 1,
      page: 1,
      size: 20
    });
    await expect(tenantApi.listEndUsers({})).rejects.toThrow("Unexpected credential state: unknown");

    mocks.request.mockReset();
    mocks.request.mockResolvedValueOnce({
      items: [{
        orderId: "order-1",
        scene: "tenant_topup",
        status: "paid",
        paymentCurrency: "CNY",
        paymentAmountMinor: 100,
        grossAmountMicroUsd: 1,
        feeAmountMicroUsd: 0,
        giftAmountMicroUsd: 0,
        creditedAmountMicroUsd: 1,
        topupMode: "other",
        createdAt: 1
      }],
      total: 1,
      page: 1,
      size: 20
    });
    await expect(tenantApi.listTopupOrders()).rejects.toThrow("Unexpected topup mode: other");
  });
});
