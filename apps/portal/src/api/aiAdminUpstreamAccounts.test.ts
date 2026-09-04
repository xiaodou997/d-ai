import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/api",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { aiAdminApi } from "./aiAdmin";

beforeEach(() => mocks.request.mockReset());

const account = {
  id: "account-1",
  name: "Primary",
  tenant_display_name: "Primary",
  tenant_access_mode: "public",
  endpoints: [{ id: "endpoint-1", account_id: "account-1", api_format: "openai_responses", base_url: "https://api.example.com", auth_scheme: "format_default", extra_headers: {}, status: "active", health_status: "unknown" }],
  concurrency_limit: 4,
  price_book_id: "book-1",
  tenant_multiplier: 1.1,
  status: "active",
  created_at: 100,
  updated_at: 200,
  $schema: "ignored"
};

describe("AI admin upstream account generated operation facade", () => {
  it("normalizes account lists and rejects unknown response status", async () => {
    mocks.request.mockResolvedValueOnce({ items: [account], total: 1 });
    await expect(aiAdminApi.listUpstreamAccounts()).resolves.toMatchObject({
      items: [{ id: "account-1", status: "active", tenant_access_mode: "public" }],
      total: 1
    });

    mocks.request.mockResolvedValueOnce({ items: [{ ...account, status: "retired" }], total: 1 });
    await expect(aiAdminApi.listUpstreamAccounts()).rejects.toThrow("Unexpected upstream account status");
  });

  it("binds CRUD/status operations with encoded account paths and typed bodies", async () => {
    mocks.request
      .mockResolvedValueOnce(account)
      .mockResolvedValueOnce(account)
      .mockResolvedValueOnce(account)
      .mockResolvedValueOnce({ deleted: true });

    const body = {
      name: "Primary",
      tenant_display_name: "Primary",
      tenant_access_mode: "public" as const,
      api_key: "secret",
      endpoints: [{ api_format: "openai_responses" as const, base_url: "https://api.example.com" }],
      concurrency_limit: null,
      price_book_id: "book-1",
      tenant_multiplier: 1.1,
    };

    await expect(aiAdminApi.createUpstreamAccount(body)).resolves.toMatchObject({ id: "account-1" });
    await expect(aiAdminApi.updateUpstreamAccount("account/1", body)).resolves.toMatchObject({ id: "account-1" });
    await expect(aiAdminApi.updateUpstreamAccountStatus("account/1", "disabled")).resolves.toMatchObject({ id: "account-1" });
    await expect(aiAdminApi.deleteUpstreamAccount("account/1")).resolves.toEqual({ deleted: true });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      body: { name: "Primary", api_key: "secret", concurrency_limit: undefined }
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/upstream-accounts/account%2F1",
      pathParams: { accountID: "account/1" },
      body: { api_key: "secret" }
    });
    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({
      pathParams: { accountID: "account/1" },
      body: { status: "disabled" }
    });
  });

  it("maps nullable transfer responses and forwards import/export contracts", async () => {
    mocks.request
      .mockResolvedValueOnce({ schema_version: 5, exported_at: "2026-08-28T00:00:00Z", contains_plaintext_api_keys: true, accounts: null })
      .mockResolvedValueOnce({
        items: [{ name: "Imported", endpoint_count: 1, action: "create", model_binding_count: 0, warnings: null }],
        summary: { create_accounts: 1, skip_accounts: 0, create_model_bindings: 0, skip_model_bindings: 0, error_accounts: 0 }
      })
      .mockResolvedValueOnce({ created_account_ids: null, skipped_accounts: null, created_model_bindings: null, skipped_model_bindings: null, summary: { create_accounts: 0, skip_accounts: 1, create_model_bindings: 0, skip_model_bindings: 0, error_accounts: 0 } });

    await expect(aiAdminApi.exportUpstreamAccounts({ account_ids: ["account-1"] })).resolves.toMatchObject({ accounts: [] });
    await expect(aiAdminApi.previewImportUpstreamAccounts({
      accounts: [{ name: "Imported", tenant_display_name: "Imported", tenant_access_mode: "public", api_key: "secret", endpoints: [{ api_format: "openai_responses", base_url: "https://api.example.com", status: "active" }], status: "active" }]
    })).resolves.toMatchObject({ items: [{ action: "create", warnings: [] }] });
    await expect(aiAdminApi.importUpstreamAccounts({
      accounts: [{ name: "Imported", tenant_display_name: "Imported", tenant_access_mode: "public", api_key: "secret", endpoints: [{ api_format: "openai_responses", base_url: "https://api.example.com", status: "active" }], status: "active" }]
    })).resolves.toMatchObject({ created_account_ids: [], skipped_accounts: [] });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({ body: { account_ids: ["account-1"], include_model_bindings: false } });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({ body: { accounts: [{ name: "Imported", model_bindings: [] }] } });
  });

  it("manages exact account endpoints through nested resources", async () => {
    const endpoint = {
      id: "endpoint-1", account_id: "account-1", api_format: "openai_responses",
      base_url: "https://api.example.com", auth_scheme: "format_default",
      extra_headers: {}, status: "active", health_status: "unknown"
    };
    mocks.request
      .mockResolvedValueOnce({ items: [endpoint], total: 1 })
      .mockResolvedValueOnce(endpoint)
      .mockResolvedValueOnce({ ...endpoint, path_override: "/custom/responses" })
      .mockResolvedValueOnce({ deleted: true });

    await expect(aiAdminApi.listUpstreamAccountEndpoints("account/1")).resolves.toMatchObject({ total: 1 });
    await aiAdminApi.createUpstreamAccountEndpoint("account/1", { api_format: "openai_responses", base_url: "https://api.example.com" });
    await aiAdminApi.updateUpstreamAccountEndpoint("account/1", "endpoint/1", {
      api_format: "openai_responses", base_url: "https://api.example.com", path_override: "/custom/responses"
    });
    await expect(aiAdminApi.deleteUpstreamAccountEndpoint("account/1", "endpoint/1")).resolves.toEqual({ deleted: true });

    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({
      path: "/api/v1/upstream-accounts/account%2F1/endpoints/endpoint%2F1",
      pathParams: { accountID: "account/1", endpointID: "endpoint/1" }
    });
  });

  it("rejects an unknown import preview action at the domain boundary", async () => {
    mocks.request.mockResolvedValueOnce({
      items: [{ name: "Imported", endpoint_count: 1, action: "merge", model_binding_count: 0 }],
      summary: { create_accounts: 0, skip_accounts: 0, create_model_bindings: 0, skip_model_bindings: 0, error_accounts: 1 }
    });
    await expect(aiAdminApi.previewImportUpstreamAccounts({ accounts: [] })).rejects.toThrow("Unexpected upstream import action");
  });
});
