import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/api",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));
vi.mock("@/platform", () => ({ redirectPortalToLogin: vi.fn() }));
vi.mock("@/platform/ai/runtime", () => ({
  appendPortalQuery: vi.fn(),
  createPortalRuntimeTransport: vi.fn(() => ({
    request: vi.fn(),
    formRequest: vi.fn(),
    streamChatMessage: vi.fn()
  })),
  portalStatusOptions: []
}));
vi.mock("@/platform/ai/usage", () => ({ formatUSD: vi.fn() }));
vi.mock("@/env", () => ({ portalEnv: { apiBaseUrl: "/api" } }));
vi.mock("@/stores/auth", () => ({ useAuthStore: vi.fn(() => ({ accessToken: "token" })) }));

import { aiCustomerApi } from "./aiCustomer";

beforeEach(() => mocks.request.mockReset());

describe("AI customer typed operation facade", () => {
  it("binds API key operations to generated paths and normalizes the response", async () => {
    mocks.request.mockResolvedValueOnce({
      items: [
        {
          id: "key-1",
          owner_type: "user",
          tenant_id: "tenant-1",
          group_id: "group-1",
          name: "Personal",
          quota_used_micro_usd: 0,
          status: "active"
        }
      ],
      total: 1
    });

    await expect(aiCustomerApi.listApiKeys()).resolves.toMatchObject({
      items: [{ id: "key-1", status: "active" }],
      total: 1
    });
    expect(mocks.request).toHaveBeenCalledWith(
      expect.objectContaining({ method: "GET", path: "/api/v1/user-api-keys" })
    );
  });

  it("rejects invalid status before sending an operation request", async () => {
    expect(() => aiCustomerApi.updateApiKeyStatus("key/1", "revoked")).toThrow(
      "Unexpected API key status: revoked"
    );
    expect(mocks.request).not.toHaveBeenCalled();
  });
});
