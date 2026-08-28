import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/api",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { aiAdminApi } from "./aiAdmin";

beforeEach(() => mocks.request.mockReset());

const policy = {
  id: "policy-1",
  scope_type: "tenant",
  scope_id: "tenant-1",
  concurrency_limit: 12,
  status: "active",
  created_by: "admin-1",
  created_at: 100,
  updated_at: 200,
  $schema: "ignored"
};

const access = {
  resource_kind: "direct_upstream",
  resource_id: "account-1",
  internal_name: "Primary",
  tenant_display_name: "Primary",
  access_mode: "restricted",
  status: "active",
  access_granted: true,
  allowed: true,
  default_tenant_multiplier: 1.1,
  tenant_multiplier_override: 1.2,
  effective_tenant_multiplier: 1.2
};

describe("AI admin policy generated operation facade", () => {
  it("normalizes nullable runtime limit policies and identity included", async () => {
    mocks.request.mockResolvedValueOnce({
      items: null,
      total: 0,
      included: { users: {}, tenants: {} },
      $schema: "ignored"
    });

    await expect(aiAdminApi.listRuntimeLimitPolicies()).resolves.toEqual({
      items: [],
      total: 0,
      included: { users: {}, tenants: {} }
    });
    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/limit-policies",
      method: "GET"
    });
    expect(mocks.request.mock.calls[0]?.[0]).not.toHaveProperty("query");
  });

  it("binds runtime policy CRUD/status bodies and policy path parameters", async () => {
    mocks.request
      .mockResolvedValueOnce(policy)
      .mockResolvedValueOnce(policy)
      .mockResolvedValueOnce({ ...policy, status: "disabled" });

    const body = {
      scope_type: "tenant",
      scope_id: "tenant/1",
      concurrency_limit: null,
      status: "active",
      created_by: "admin-1"
    };

    await expect(aiAdminApi.createRuntimeLimitPolicy(body)).resolves.toMatchObject({ id: "policy-1" });
    await expect(aiAdminApi.updateRuntimeLimitPolicy("policy/1", body)).resolves.toMatchObject({ id: "policy-1" });
    await expect(aiAdminApi.updateRuntimeLimitPolicyStatus("policy/1", "disabled")).resolves.toMatchObject({ status: "disabled" });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      body: { scope_type: "tenant", scope_id: "tenant/1", concurrency_limit: undefined }
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/limit-policies/policy%2F1",
      pathParams: { policyID: "policy/1" },
      body: { scope_type: "tenant", status: "active" }
    });
    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({
      path: "/api/v1/limit-policies/policy%2F1/status",
      pathParams: { policyID: "policy/1" },
      body: { status: "disabled" }
    });
  });

  it("maps upstream access and forwards resource policies with encoded tenant paths", async () => {
    mocks.request
      .mockResolvedValueOnce({ items: [access], total: 1, $schema: "ignored" })
      .mockResolvedValueOnce({ updated: true, $schema: "ignored" });

    await expect(aiAdminApi.listTenantUpstreamAccess("tenant/1")).resolves.toMatchObject({
      items: [{ resource_kind: "direct_upstream", access_mode: "restricted", effective_tenant_multiplier: 1.2 }],
      total: 1
    });
    await expect(aiAdminApi.replaceTenantUpstreamAccess("tenant/1", [{
      resource_kind: "oauth_pool",
      resource_id: "pool/1",
      access_granted: true,
      tenant_multiplier_override: 1.3
    }])).resolves.toEqual({ updated: true });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/tenants/tenant%2F1/upstream-access",
      pathParams: { tenantID: "tenant/1" }
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      pathParams: { tenantID: "tenant/1" },
      body: {
        policies: [{ resource_kind: "oauth_pool", resource_id: "pool/1", access_granted: true, tenant_multiplier_override: 1.3 }]
      }
    });
  });

  it("rejects invalid policy scopes, statuses and upstream resource kinds", async () => {
    expect(() => aiAdminApi.createRuntimeLimitPolicy({ scope_id: "tenant-1" })).toThrow("Unexpected runtime limit policy scope");
    expect(() => aiAdminApi.createRuntimeLimitPolicy({ scope_type: "tenant", scope_id: "tenant-1", status: "paused" })).toThrow(
      "Unexpected runtime limit policy status"
    );
    expect(() => aiAdminApi.updateRuntimeLimitPolicyStatus("policy-1", "paused")).toThrow("Unexpected runtime limit policy status");
    expect(mocks.request).not.toHaveBeenCalled();

    mocks.request.mockResolvedValueOnce({ items: [{ ...access, resource_kind: "unknown" }], total: 1 });
    await expect(aiAdminApi.listTenantUpstreamAccess("tenant-1")).rejects.toThrow("Unexpected tenant upstream resource kind");
  });
});
