import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { platformCustomerApi } from "./platformCustomer";

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

const topupOrder = {
  orderId: "order-1",
  paymentCurrency: "USD",
  paymentAmountMinor: 100,
  grossAmountMicroUsd: 1_000_000,
  feeAmountMicroUsd: 0,
  giftAmountMicroUsd: 0,
  creditedAmountMicroUsd: 1_000_000,
  topupMode: "custom",
  status: "created"
};

beforeEach(() => mocks.request.mockReset());

describe("platform customer generated operation facade", () => {
  it("normalizes nullable transport collections for page models", async () => {
    mocks.request
      .mockResolvedValueOnce({ siteName: "D-AI", faviconPath: "/favicon.svg", $schema: "ignored" })
      .mockResolvedValueOnce(balance)
      .mockResolvedValueOnce({ items: null, total: 0, page: 1, size: 20 })
      .mockResolvedValueOnce({
        enabled: true,
        currency: "USD",
        feeRateBp: 0,
        minMicroUsd: 1_000_000,
        maxMicroUsd: 10_000_000,
        packages: null
      })
      .mockResolvedValueOnce({ items: null, total: 0, page: 1, size: 20 });

    await expect(platformCustomerApi.getPortalBrand()).resolves.toEqual({
      siteName: "D-AI",
      faviconPath: "/favicon.svg"
    });
    await expect(platformCustomerApi.getBalance()).resolves.toEqual({
      ...balance,
      balanceLots: undefined
    });
    await expect(platformCustomerApi.getRechargeRecords({ page: 1, size: 20 })).resolves.toEqual({
      items: [],
      total: 0,
      page: 1,
      size: 20
    });
    await expect(platformCustomerApi.getTopupConfig()).resolves.toMatchObject({ packages: [] });
    await expect(platformCustomerApi.listTopupOrders()).resolves.toMatchObject({ items: [] });
  });

  it("maps create and status operations to constrained page models", async () => {
    mocks.request
      .mockResolvedValueOnce({ ...topupOrder, codeUrl: "weixin://pay", expiresAt: 123 })
      .mockResolvedValueOnce({ ...topupOrder, transactionId: "txn-1" })
      .mockResolvedValueOnce({
        items: [{ ...topupOrder, scene: "user_topup", createdAt: 100 }],
        total: 1,
        page: 1,
        size: 20
      });

    await expect(platformCustomerApi.createTopupOrder({ amountMicroUsd: 1_000_000 })).resolves.toMatchObject({
      orderId: "order-1",
      topupMode: "custom"
    });
    await expect(platformCustomerApi.getTopupOrder("order-1")).resolves.toMatchObject({
      status: "created",
      transactionId: "txn-1"
    });
    await expect(platformCustomerApi.listTopupOrders({ page: 1, size: 20 })).resolves.toMatchObject({
      items: [{ scene: "user_topup", status: "created" }]
    });

    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/payments/topup-orders/order-1",
      pathParams: { orderId: "order-1" }
    });
  });

  it("rejects unknown transport enum values instead of leaking them to the page", async () => {
    mocks.request.mockResolvedValueOnce({ ...topupOrder, status: "unknown" });

    await expect(platformCustomerApi.getTopupOrder("order-1")).rejects.toThrow(
      "Unexpected top-up status: unknown"
    );
  });
});
