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

const book = {
  id: "book-1",
  owner_type: "tenant",
  owner_tenant_id: "tenant-1",
  writable: true,
  name: "Retail",
  description: "Tenant prices",
  status: "active",
  revision: 3,
  $schema: "ignored"
};

const entry = {
  model_code: "gpt/4o",
  capability_type: "chat",
  token_price_tiers: null,
  image_default_price_usd: 0,
  video_default_price_usd: 0,
  audio_tts_per_1m_chars_usd: 0,
  audio_stt_per_minute_usd: 0,
  source: "manual",
  manually_edited: true,
  $schema: "ignored"
};

describe("AI tenant price book generated operation facade", () => {
  it("normalizes book and entry pages without leaking schema metadata", async () => {
    mocks.request
      .mockResolvedValueOnce({ items: [book], total: 1, $schema: "ignored" })
      .mockResolvedValueOnce({ items: [entry], total: 1, $schema: "ignored" });

    await expect(aiTenantApi.listPriceBooks()).resolves.toEqual({
      items: [{ ...book, $schema: undefined }].map(({ $schema: _schema, ...value }) => value),
      total: 1
    });
    await expect(aiTenantApi.listPriceBookEntries("book/1")).resolves.toMatchObject({
      items: [{ model_code: "gpt/4o", token_price_tiers: [] }],
      total: 1
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/tenants/me/price-books/book%2F1/entries",
      pathParams: { bookID: "book/1" }
    });
  });

  it("binds write/delete/sync operations to generated paths and response types", async () => {
    mocks.request
      .mockResolvedValueOnce(book)
      .mockResolvedValueOnce({
        ...entry,
        token_price_tiers: [{ up_to_input_tokens: null, input_per_1m_usd: 1, output_per_1m_usd: 2, cache_write_per_1m_usd: 0, cache_read_per_1m_usd: 0 }]
      })
      .mockResolvedValueOnce({ deleted: true })
      .mockResolvedValueOnce({ synced: 2, missing: null });

    await aiTenantApi.updatePriceBook("book/1", { name: "Retail", description: "updated", status: "active" });
    await aiTenantApi.upsertPriceBookEntry("book/1", "gpt/4o", {
      capability_type: "chat",
      token_price_tiers: [],
      image_default_price_usd: 0,
      video_default_price_usd: 0,
      audio_tts_per_1m_chars_usd: 0,
      audio_stt_per_minute_usd: 0,
      image_prices: [],
      video_prices: []
    });
    await expect(aiTenantApi.deletePriceBook("book/1")).resolves.toEqual({ deleted: true });
    await expect(aiTenantApi.syncCommonPriceModels("book/1")).resolves.toEqual({ synced: 2, missing: [] });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/tenants/me/price-books/book%2F1",
      pathParams: { bookID: "book/1" }
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/tenants/me/price-books/book%2F1/entries/gpt%2F4o",
      pathParams: { bookID: "book/1", modelCode: "gpt/4o" }
    });
  });

  it("maps transfer bundles and rejects unsupported schema versions", async () => {
    mocks.request.mockResolvedValueOnce(book);
    await aiTenantApi.importPriceBook({
      schema_version: 1,
      name: "Imported",
      entries: [{
        model_code: "gpt-4o",
        capability_type: "chat",
        token_price_tiers: [],
        image_default_price_usd: 0,
        video_default_price_usd: 0,
        audio_tts_per_1m_chars_usd: 0,
        audio_stt_per_minute_usd: 0
      }]
    });
    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      body: {
        schema_version: 1,
        entries: [{ model_code: "gpt-4o", source: "manual", manually_edited: true }]
      }
    });

    mocks.request.mockResolvedValueOnce({ schema_version: 2, name: "bad", entries: [] });
    await expect(aiTenantApi.exportPriceBook("book-1")).rejects.toThrow("Unsupported price book schema version");
  });
});
