import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/api",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { aiAdminApi } from "./aiAdmin";

beforeEach(() => mocks.request.mockReset());

const routeWeights = {
  scope: "global",
  weights: { cost: 0.3, latency: 0.2, load: 0.2, health: 0.3, $schema: "ignored" },
  $schema: "ignored"
};

describe("AI admin system generated operation facade", () => {
  it("maps system health snapshots and normalizes nullable records", async () => {
    mocks.request.mockResolvedValueOnce({
      timestamp: 100,
      db: { status: "ok" },
      redis: { status: "ok", error: undefined },
      health: { total_tracked: 2, open_count: 0, half_open_count: 1, records: null },
      $schema: "ignored"
    });

    await expect(aiAdminApi.getSystemStatus()).resolves.toEqual({
      timestamp: 100,
      db: { status: "ok", error: undefined },
      redis: { status: "ok", error: undefined },
      health: { total_tracked: 2, open_count: 0, half_open_count: 1, records: [] }
    });
    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({ path: "/api/v1/system/status", method: "GET" });
  });

  it("binds route weight get/put paths and removes transport schema", async () => {
    mocks.request.mockResolvedValueOnce(routeWeights).mockResolvedValueOnce(routeWeights);

    await expect(aiAdminApi.getRouteWeights("tenant/1")).resolves.toEqual({
      scope: "global",
      weights: { cost: 0.3, latency: 0.2, load: 0.2, health: 0.3 }
    });
    await expect(aiAdminApi.putRouteWeights("tenant/1", { cost: 0.4, latency: 0.2, load: 0.1, health: 0.3 })).resolves.toMatchObject({
      scope: "global"
    });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/route-weights/tenant%2F1",
      pathParams: { scope: "tenant/1" }
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/route-weights/tenant%2F1",
      pathParams: { scope: "tenant/1" },
      body: { cost: 0.4, latency: 0.2, load: 0.1, health: 0.3 }
    });
  });

  it("rejects non-finite route weights before transport", async () => {
    expect(() => aiAdminApi.putRouteWeights("global", { cost: Number.NaN, latency: 0.2, load: 0.2, health: 0.4 })).toThrow(
      "route weights must be finite numbers"
    );
    expect(mocks.request).not.toHaveBeenCalled();
  });
});
