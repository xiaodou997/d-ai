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
  it("explicitly refreshes quotas and preserves per-account inspection state", async () => {
    mocks.request.mockResolvedValue({ items: [{ ...credential, quota_inspection: { state: 'failed', observed_at: 123 }, quotas: [{ source: 'quota_api', observed_at: 100, windows: [{ key: 'primary', used_percent: 42 }] }] }], total: 1 });
    const result = await aiAdminApi.listPoolCredentials('pool/1', true);
    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({ pathParams: { poolID: 'pool/1' }, query: { refresh_quota: true } });
    expect(result.items[0]).toMatchObject({ quota_inspection: { state: 'failed' }, quotas: [{ windows: [{ used_percent: 42 }] }] });
  });
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

  it("keeps runtime quota facts and edits credentials without sending blank secrets", async () => {
    mocks.request.mockResolvedValueOnce({ ...credential, quotas: [{ source: "response_headers", observed_at: 200, windows: [{ key: "primary", used_percent: 80, window_minutes: 300, reset_at: 1000 }] }] });
    const result = await aiAdminApi.patchPoolCredential("pool-1", "cred-1", { name: "Renamed", access_token: " ", refresh_token: " new-refresh ", auth_metadata: { account_id: "acct-1" } });
    expect(result.cooldown_until).toBe(400);
    expect(result.quotas?.[0].windows[0].used_percent).toBe(80);
    expect(mocks.request.mock.calls[0]?.[0].body).toMatchObject({ name: "Renamed", access_token: undefined, refresh_token: "new-refresh", auth_metadata: { account_id: "acct-1" } });
  });

  it("keeps account model observations separate and sends an explicit refresh query", async () => {
    mocks.request.mockResolvedValueOnce({ ...credential, model_catalog: { credential_id: 'cred-1', models: null, source: 'unknown' } });
    const account = await aiAdminApi.refreshPoolCredential('pool-1', 'cred-1');
    expect(account.model_catalog).toEqual({ credential_id: 'cred-1', models: [], source: 'unknown' });
    mocks.request.mockResolvedValueOnce({ pool_id: 'pool-1', fixed_provider_type: 'codex', models: ['one'], source: 'live', profile_revision: 'profile', observed_at: 100, accounts: [{ credential_id: 'cred-1', models: ['one'], source: 'live', valid_until: 200 }] });
    const result = await aiAdminApi.getPoolAvailableModels('pool-1', true);
    expect(result.accounts[0]).toMatchObject({ credential_id: 'cred-1', models: ['one'], valid_until: 200 });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({ query: { refresh: true }, pathParams: { poolID: 'pool-1' } });
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
