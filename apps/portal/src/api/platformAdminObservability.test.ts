import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/api",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { platformAdminApi } from "./platformAdmin";

beforeEach(() => mocks.request.mockReset());

describe("platform admin observability generated operation facade", () => {
  it("normalizes auth audit and JWT key collections and forwards typed queries", async () => {
    mocks.request
      .mockResolvedValueOnce({ items: null, total: 0, page: 1, size: 20, $schema: "ignored" })
      .mockResolvedValueOnce({ keys: null, total: 0, $schema: "ignored" })
      .mockResolvedValueOnce({ message: "rotated", $schema: "ignored" });

    await expect(platformAdminApi.getAuthAuditLogs({ eventType: "login", page: 1, size: 20 })).resolves.toEqual({
      items: [],
      total: 0,
      page: 1,
      size: 20
    });
    await expect(platformAdminApi.listJwtKeys()).resolves.toEqual({ keys: [], total: 0 });
    await expect(platformAdminApi.rotateJwtKey()).resolves.toEqual({ message: "rotated" });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/auth-audit-logs",
      query: { eventType: "login", page: 1, size: 20 }
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({ path: "/api/v1/jwt-keys" });
    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({ path: "/api/v1/jwt-keys/rotate" });
  });

  it("maps analytics and dashboard alerts while normalizing nullable arrays", async () => {
    mocks.request
      .mockResolvedValueOnce({
        currency: "USD",
        tenantRechargePaidMinor: 100,
        tenantRechargeAmountUsd: 1,
        activeTenants: 2,
        tenantTotalBalanceUsd: 5,
        userRechargePaidMinor: 50,
        userRechargeAmountUsd: 0.5,
        newUsers: 3,
        userTotalBalanceUsd: 2,
        $schema: "ignored"
      })
      .mockResolvedValueOnce({ totalUsd: 1.5, dataPoints: null, $schema: "ignored" })
      .mockResolvedValueOnce({ resources: null, $schema: "ignored" })
      .mockResolvedValueOnce({ failedTransactions: null, $schema: "ignored" });

    await expect(platformAdminApi.getGlobalStats({ timeFrom: 100, timeTo: 200 })).resolves.toMatchObject({
      currency: "USD",
      tenantRechargePaidMinor: 100,
      userTotalBalanceUsd: 2
    });
    await expect(platformAdminApi.getConsumptionTrend({ timeFrom: 100, accountType: "tenant" })).resolves.toEqual({
      totalUsd: 1.5,
      dataPoints: []
    });
    await expect(platformAdminApi.getResourceStatistics({ timeFrom: 100, timeTo: 200, tenantId: "tenant-1" })).resolves.toEqual({ resources: [] });
    await expect(platformAdminApi.getDashboardAlerts()).resolves.toEqual({ failedTransactions: [] });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({ query: { timeFrom: 100, timeTo: 200 } });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({ query: { timeFrom: 100, accountType: "tenant" } });
    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({ query: { timeFrom: 100, timeTo: 200, tenantId: "tenant-1" } });
    expect(mocks.request.mock.calls[3]?.[0]).toMatchObject({ path: "/api/v1/dashboard/alerts" });
  });
});
