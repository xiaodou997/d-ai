import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/api",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { aiAdminApi } from "./aiAdmin";

beforeEach(() => mocks.request.mockReset());

const book = {
  id: "book-1",
  owner_type: "platform",
  owner_tenant_id: undefined,
  writable: true,
  name: "Global",
  description: "Public prices",
  status: "active",
  revision: 4,
  created_at: 100,
  updated_at: 200,
  $schema: "ignored"
};

const entry = {
  model_code: "gpt/4o",
  capability_type: "chat",
  token_price_tiers: null,
  image_default_price_usd: 0,
  video_default_price_usd: 0,
  image_prices: null,
  video_prices: null,
  audio_tts_per_1m_chars_usd: 0,
  audio_stt_per_minute_usd: 0,
  source: "manual",
  manually_edited: true,
  $schema: "ignored"
};

describe("AI admin price book generated operation facade", () => {
  it("normalizes nullable books and entries while preserving page models", async () => {
    mocks.request
      .mockResolvedValueOnce({ items: null, total: 0 })
      .mockResolvedValueOnce({ items: [entry], total: 1 });

    await expect(aiAdminApi.listPriceBooks()).resolves.toEqual({ items: [], total: 0 });
    await expect(aiAdminApi.listPriceBookEntries("book/1")).resolves.toMatchObject({
      items: [{ model_code: "gpt/4o", token_price_tiers: [], image_prices: undefined, video_prices: undefined }],
      total: 1
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/price-books/book%2F1/entries",
      pathParams: { bookID: "book/1" }
    });
  });

  it("binds admin price book CRUD, entry and sync operations to generated paths", async () => {
    mocks.request
      .mockResolvedValueOnce(book)
      .mockResolvedValueOnce(book)
      .mockResolvedValueOnce(book)
      .mockResolvedValueOnce({ deleted: true })
      .mockResolvedValueOnce(entry)
      .mockResolvedValueOnce({ deleted: true })
      .mockResolvedValueOnce({ synced: 2, missing: null });

    await expect(aiAdminApi.createPriceBook({ name: "Global", description: "Public prices" })).resolves.toMatchObject({ id: "book-1" });
    await expect(aiAdminApi.getPriceBook("book/1")).resolves.toMatchObject({ id: "book-1" });
    await expect(aiAdminApi.updatePriceBook("book/1", { name: "Global", status: "active" })).resolves.toMatchObject({ id: "book-1" });
    await expect(aiAdminApi.deletePriceBook("book/1")).resolves.toEqual({ deleted: true });
    await expect(aiAdminApi.upsertPriceBookEntry("book/1", "gpt/4o", { capability_type: "chat", token_price_tiers: [] })).resolves.toMatchObject({
      token_price_tiers: []
    });
    await expect(aiAdminApi.deletePriceBookEntry("book/1", "gpt/4o")).resolves.toEqual({ deleted: true });
    await expect(aiAdminApi.syncCommonModels("book/1")).resolves.toEqual({ synced: 2, missing: [] });

    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/price-books/book%2F1",
      pathParams: { bookID: "book/1" }
    });
    expect(mocks.request.mock.calls[4]?.[0]).toMatchObject({
      path: "/api/v1/price-books/book%2F1/entries/gpt%2F4o",
      pathParams: { bookID: "book/1", modelCode: "gpt/4o" },
      body: { capability_type: "chat", token_price_tiers: [] }
    });
  });

  it("rejects unknown price book status at the transport boundary", async () => {
    mocks.request.mockResolvedValueOnce({ ...book, status: "archived" });
    await expect(aiAdminApi.getPriceBook("book-1")).rejects.toThrow("Unexpected price book status");
  });
});
