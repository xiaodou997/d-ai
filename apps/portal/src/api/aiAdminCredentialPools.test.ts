import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/api",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { aiAdminApi } from "./aiAdmin";

beforeEach(() => mocks.request.mockReset());

const pool = {
  id: "pool-1",
  name: "Codex pool",
  tenant_display_name: "Codex",
  tenant_access_mode: "restricted",
  fixed_provider_type: "codex",
  oauth_strategy: "weighted",
  notes: "primary",
  status: "active",
  price_book_id: "book-1",
  tenant_multiplier: 1.2,
  created_at: 100,
  updated_at: 200,
  $schema: "ignored"
};

const credential = {
  id: "cred-1",
  pool_id: "pool-1",
  name: "alice",
  provider_type: "codex",
  email: "alice@example.com",
  token_type: "bearer",
  scope: "all",
  expires_at: 300,
  auth_metadata: { account_id: "acct-1" },
  weight: 100,
  status: "active",
  invalid_reason: undefined,
  last_used_at: 100,
  last_refreshed_at: 200,
  last_failed_at: undefined,
  consecutive_fail_count: 0,
  success_count: 8,
  fail_count: 1,
  created_at: 100,
  updated_at: 200,
  cooldown_until: 400,
  $schema: "ignored"
};

describe("AI admin credential pool generated operation facade", () => {
  it("normalizes pool lists and maps pool CRUD/status/delete operations", async () => {
    mocks.request
      .mockResolvedValueOnce({ items: null, total: 0, $schema: "ignored" })
      .mockResolvedValueOnce(pool)
      .mockResolvedValueOnce(pool)
      .mockResolvedValueOnce({ ...pool, status: "disabled" })
      .mockResolvedValueOnce({ deleted: true, $schema: "ignored" });

    await expect(aiAdminApi.listCredentialPools()).resolves.toEqual({ items: [], total: 0 });
    await expect(aiAdminApi.createCredentialPool({
      name: "Codex pool",
      tenant_display_name: "Codex",
      tenant_access_mode: "restricted",
      fixed_provider_type: "codex",
      oauth_strategy: "weighted",
      notes: "primary",
      price_book_id: "book-1",
      tenant_multiplier: 1.2
    })).resolves.toMatchObject({ id: "pool-1", fixed_provider_type: "codex" });
    await expect(aiAdminApi.patchCredentialPool("pool/1", { name: "Codex pool" })).resolves.toMatchObject({ id: "pool-1" });
    await expect(aiAdminApi.updateCredentialPoolStatus("pool/1", "disabled")).resolves.toMatchObject({ status: "disabled" });
    await expect(aiAdminApi.deleteCredentialPool("pool/1")).resolves.toEqual({ deleted: true });

    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      body: { name: "Codex pool", fixed_provider_type: "codex", oauth_strategy: "weighted" }
    });
    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({
      path: "/api/v1/credential-pools/pool%2F1",
      pathParams: { poolID: "pool/1" },
      body: { name: "Codex pool" }
    });
    expect(mocks.request.mock.calls[3]?.[0]).toMatchObject({
      pathParams: { poolID: "pool/1" },
      body: { status: "disabled" }
    });
    expect(mocks.request.mock.calls[4]?.[0]).toMatchObject({
      pathParams: { poolID: "pool/1" }
    });
  });

  it("normalizes credential lists and binds import, patch, refresh and delete paths", async () => {
    mocks.request
      .mockResolvedValueOnce({ items: null, total: 0 })
      .mockResolvedValueOnce(credential)
      .mockResolvedValueOnce({ ...credential, status: "disabled" })
      .mockResolvedValueOnce(credential)
      .mockResolvedValueOnce({ deleted: true });

    await expect(aiAdminApi.listPoolCredentials("pool/1")).resolves.toEqual({ items: [], total: 0 });
    await expect(aiAdminApi.createPoolCredential("pool/1", {
      access_token: "secret",
      provider_type: "codex",
      email: "alice@example.com",
      account_id: "acct-1",
      weight: 100
    })).resolves.toMatchObject({ id: "cred-1", provider_type: "codex" });
    await expect(aiAdminApi.patchPoolCredential("pool/1", "cred/1", { status: "disabled", weight: 0 })).resolves.toMatchObject({ status: "disabled" });
    await expect(aiAdminApi.refreshPoolCredential("pool/1", "cred/1")).resolves.toMatchObject({ id: "cred-1" });
    await expect(aiAdminApi.deletePoolCredential("pool/1", "cred/1")).resolves.toEqual({ deleted: true });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/credential-pools/pool%2F1/credentials",
      pathParams: { poolID: "pool/1" }
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      body: { access_token: "secret", provider_type: "codex", account_id: "acct-1" }
    });
    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({
      path: "/api/v1/credential-pools/pool%2F1/credentials/cred%2F1",
      pathParams: { poolID: "pool/1", credID: "cred/1" },
      body: { status: "disabled", weight: 0 }
    });
    expect(mocks.request.mock.calls[3]?.[0]).toMatchObject({
      path: "/api/v1/credential-pools/pool%2F1/credentials/cred%2F1/refresh",
      pathParams: { poolID: "pool/1", credID: "cred/1" }
    });
  });

  it("maps OAuth pool health while keeping transport-only cooling fields out of the page model", async () => {
    mocks.request.mockResolvedValueOnce({
      items: [{
        pool_id: "pool-1",
        pool_name: "Codex pool",
        fixed_provider_type: "codex",
        oauth_strategy: "round_robin",
        total: 3,
        active: 2,
        invalid: 0,
        disabled: 1,
        expiring_soon: 1,
        cooling_down: 1
      }],
      total: 1,
      $schema: "ignored"
    });

    const result = await aiAdminApi.getOAuthPoolHealth();
    expect(result).toEqual({
      items: [{ pool_id: "pool-1", pool_name: "Codex pool", fixed_provider_type: "codex", oauth_strategy: "round_robin", total: 3, active: 2, invalid: 0, disabled: 1, expiring_soon: 1 }],
      total: 1
    });
    expect(result.items[0]).not.toHaveProperty("cooling_down");
  });

  it("rejects unsupported pool and credential values before transport", async () => {
    expect(() => aiAdminApi.createCredentialPool({ name: "Pool", fixed_provider_type: "unknown" })).toThrow(
      "Unexpected credential pool provider"
    );
    expect(() => aiAdminApi.createCredentialPool({ name: "Pool", oauth_strategy: "random" })).toThrow(
      "Unexpected credential pool OAuth strategy"
    );
    expect(() => aiAdminApi.createPoolCredential("pool-1", { access_token: "secret", provider_type: "unknown" })).toThrow(
      "Unexpected credential pool provider"
    );
    expect(() => aiAdminApi.patchPoolCredential("pool-1", "cred-1", { status: "pending" })).toThrow(
      "Unexpected credential pool status"
    );
    expect(mocks.request).not.toHaveBeenCalled();

    mocks.request.mockResolvedValueOnce({ items: [{ ...pool, status: "retired" }], total: 1 });
    await expect(aiAdminApi.listCredentialPools()).rejects.toThrow("Unexpected credential pool status");
  });
});
