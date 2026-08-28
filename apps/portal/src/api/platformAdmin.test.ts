import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { platformAdminApi } from "./platformAdmin";

beforeEach(() => mocks.request.mockReset());

describe("platform admin generated operation facade", () => {
  it("normalizes nullable admin pages and rejects unknown credential states", async () => {
    mocks.request.mockResolvedValueOnce({ items: null, total: 0, page: 1, size: 20 });
    await expect(platformAdminApi.listSystemAdmins({ page: 1, size: 20 })).resolves.toEqual({
      items: [],
      total: 0,
      page: 1,
      size: 20
    });

    mocks.request.mockResolvedValueOnce({
      items: [{ userId: "admin-1", username: "alice", email: "a@example.com", status: 1, statusText: "active", credentialState: "unknown", createdTime: 10 }],
      total: 1,
      page: 1,
      size: 20
    });
    await expect(platformAdminApi.listSystemAdmins({})).rejects.toThrow("Unexpected credential state");
  });

  it("keeps tenant detail compatibility fields while using generated response", async () => {
    mocks.request.mockResolvedValueOnce({
      tenantId: "tenant-1",
      tenantName: "Acme",
      contactPerson: "Alice",
      contactEmail: "a@example.com",
      status: 1,
      statusDisplay: "active",
      createdTime: 10
    });

    await expect(platformAdminApi.getTenant("tenant/1")).resolves.toMatchObject({
      tenantId: "tenant-1",
      isWildcard: false,
      clientIds: []
    });
    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/tenants/tenant%2F1",
      pathParams: { id: "tenant/1" }
    });
  });

  it("maps status and activation operations without leaking transport DTOs", async () => {
    mocks.request
      .mockResolvedValueOnce({ success: true })
      .mockResolvedValueOnce({ activationToken: "token", activationExpiresIn: 3600 })
      .mockResolvedValueOnce({ items: null, total: 0, page: 1, size: 20 });

    await expect(platformAdminApi.updateTenantStatus("tenant/1", "disabled")).resolves.toEqual({ status: "success" });
    await expect(platformAdminApi.resetTenantUserPassword("user/1")).resolves.toEqual({
      activationToken: "token",
      activationExpiresIn: 3600
    });
    await expect(platformAdminApi.listEndUsers({})).resolves.toMatchObject({ items: [] });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/tenants/tenant%2F1/status",
      pathParams: { id: "tenant/1" },
      body: { status: "disabled" }
    });
  });
});
