import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/api",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));
vi.mock("@/platform", () => ({ redirectPortalToLogin: vi.fn() }));
vi.mock("@/platform/ai/runtime", () => ({
  createPortalRuntimeTransport: vi.fn(() => ({ request: vi.fn(), formRequest: vi.fn(), streamChatMessage: vi.fn() })),
  portalStatusOptions: []
}));
vi.mock("@/platform/ai/usage", () => ({ formatUSD: vi.fn() }));
vi.mock("@/platform/ai/images", () => ({}));
vi.mock("@/platform/ai/tasks", () => ({}));
vi.mock("@/env", () => ({ portalEnv: { apiBaseUrl: "/api" } }));
vi.mock("@/stores/auth", () => ({ useAuthStore: vi.fn(() => ({ accessToken: "token" })) }));

import { aiTenantApi } from "./aiTenant";

beforeEach(() => mocks.request.mockReset());

describe("AI tenant control facade", () => {
  it("normalizes API key pages and strips generated schema metadata", async () => {
    mocks.request.mockResolvedValueOnce({
      items: [{ id: "key-1", owner_type: "tenant", tenant_id: "tenant-1", group_id: "group-1", name: "demo", quota_used_micro_usd: 0, status: "active", $schema: "ignored" }],
      total: 1,
      $schema: "ignored"
    });
    await expect(aiTenantApi.listApiKeys()).resolves.toEqual({
      items: [{ id: "key-1", owner_type: "tenant", tenant_id: "tenant-1", group_id: "group-1", name: "demo", quota_used_micro_usd: 0, status: "active" }],
      total: 1
    });

    mocks.request.mockResolvedValueOnce({ items: null, total: 0 });
    await expect(aiTenantApi.listMyGroups()).resolves.toEqual({ items: [], total: 0 });
  });

  it("uses generated body/path contracts and preserves legacy API key input shape", async () => {
    mocks.request
      .mockResolvedValueOnce({ plaintext_key: "secret", key: { id: "key-1", owner_type: "tenant", tenant_id: "tenant-1", group_id: "group-1", name: "demo", quota_used_micro_usd: 0, status: "active" } })
      .mockResolvedValueOnce({ id: "key-1", owner_type: "tenant", tenant_id: "tenant-1", group_id: "group-1", name: "demo", quota_used_micro_usd: 0, status: "active" });

    await aiTenantApi.createApiKey({ name: "demo", group_id: "group-1", limit_policy: null });
    await aiTenantApi.updateApiKey("key/1", { name: "demo", group_id: "group-1", limit_policy: null });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      body: { name: "demo", group_id: "group-1", limit_policy: undefined }
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/tenants/me/api-keys/key%2F1",
      pathParams: { apiKeyID: "key/1" },
      body: { name: "demo", group_id: "group-1", limit_policy: undefined }
    });
  });

  it("rejects invalid status/target values before exposing them to pages", async () => {
    expect(() => aiTenantApi.updateApiKeyStatus("key-1", "revoked")).toThrow("Unexpected API key status");
    expect(mocks.request).not.toHaveBeenCalled();

    mocks.request.mockResolvedValueOnce({
      items: [{ id: "target-1", group_id: "group-1", available: true, priority: 1, status: "active", target_type: "other" }],
      total: 1
    });
    await expect(aiTenantApi.listGroupTargets("group/1")).rejects.toThrow("Unexpected group target type");
    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/tenants/me/groups/group%2F1/targets",
      pathParams: { groupID: "group/1" }
    });
  });
});
