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

describe("AI tenant self-read generated operation facade", () => {
  it("maps available models and upstream resources into non-null domain collections", async () => {
    mocks.request
      .mockResolvedValueOnce({ items: null, total: 0 })
      .mockResolvedValueOnce({
        items: [{
          id: "resource-1",
          resource_kind: "direct_upstream",
          name: "Primary",
          tenant_multiplier: 1.2,
          models: [{
            model_code: "gpt-4o",
            capability_type: "chat",
            api_format: "openai",
            availability: "available",
            price: {
              model_code: "gpt-4o",
              capability_type: "chat",
              token_price_tiers: null,
              image_default_price_usd: 0,
              video_default_price_usd: 0,
              audio_tts_per_1m_chars_usd: 0,
              audio_stt_per_minute_usd: 0,
              source: "catalog",
              manually_edited: false,
              $schema: "ignored"
            }
          }]
        }],
        total: 1
      })
      .mockResolvedValueOnce({
        items: [{
          model_code: "gpt-4o",
          model_name: "GPT-4o",
          capability_type: "chat",
          input_per_1m_usd_min: 1,
          input_per_1m_usd_max: 1,
          output_per_1m_usd_min: 2,
          output_per_1m_usd_max: 2,
          cache_write_per_1m_usd_min: 0,
          cache_write_per_1m_usd_max: 0,
          cache_read_per_1m_usd_min: 0,
          cache_read_per_1m_usd_max: 0,
          has_context_tiers: false,
          image_default_price_usd_min: 0,
          image_default_price_usd_max: 0,
          video_default_price_usd_min: 0,
          video_default_price_usd_max: 0,
          image_prices: null,
          video_prices: null
        }],
        total: 1
      });

    await expect(aiTenantApi.listAvailableModels()).resolves.toEqual({ items: [], total: 0 });
    await expect(aiTenantApi.listUpstreamResources()).resolves.toMatchObject({
      items: [{ models: [{ price: { token_price_tiers: [] } }] }],
      total: 1
    });
    await expect(aiTenantApi.listAvailableModels()).resolves.toMatchObject({
      items: [{ model_code: "gpt-4o", image_prices: undefined, video_prices: undefined }]
    });
  });

  it("maps effective prices, user bindings and user limit policies with scoped paths", async () => {
    mocks.request
      .mockResolvedValueOnce({ group_id: "group-1", retail_price_book_id: "book-1", effective_user_multiplier: 1.1, items: null, total: 0 })
      .mockResolvedValueOnce({ items: null, total: 0 })
      .mockResolvedValueOnce({ included: { tenant_id: "tenant-1", user_id: "user-1" }, items: [{ id: "policy-1", scope_type: "user", scope_id: "user-1", status: "active", concurrency_limit: null, $schema: "ignored" }], total: 1 })
      .mockResolvedValueOnce({ deleted: true })
      .mockResolvedValueOnce({ id: "policy-1", scope_type: "user", scope_id: "user-1", status: "active", concurrency_limit: 4 });

    await expect(aiTenantApi.getMyGroupEffectivePrices("group/1")).resolves.toEqual({
      group_id: "group-1",
      retail_price_book_id: "book-1",
      effective_user_multiplier: 1.1,
      items: [],
      total: 0
    });
    await expect(aiTenantApi.listUserGroups("user/1")).resolves.toEqual({ items: [], total: 0 });
    await expect(aiTenantApi.listUserLimitPolicies("user/1")).resolves.toMatchObject({
      items: [{ id: "policy-1", scope_type: "user", concurrency_limit: null }]
    });
    await expect(aiTenantApi.upsertUserGroup("user/1", "group/1", { multiplier_override: null })).resolves.toEqual({ deleted: true });
    await expect(aiTenantApi.upsertUserLimitPolicy("user/1", { concurrency_limit: 4, status: "active" })).resolves.toMatchObject({ id: "policy-1" });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({ pathParams: { groupID: "group/1" } });
    expect(mocks.request.mock.calls[3]?.[0]).toMatchObject({
      pathParams: { userID: "user/1", groupID: "group/1" },
      body: { multiplier_override: undefined }
    });
    expect(mocks.request.mock.calls[4]?.[0]).toMatchObject({ pathParams: { userID: "user/1" } });
  });

  it("normalizes dashboard lists and rejects invalid limit policy scopes", async () => {
    mocks.request
      .mockResolvedValueOnce({
        total_requests: 10,
        successful_requests: 8,
        failed_requests: 2,
        total_tokens: 100,
        total_prompt_tokens: 60,
        total_completion_tokens: 40,
        total_catalog_base_usd: 1,
        total_tenant_payable_usd: 2,
        total_retail_base_usd: 3,
        total_user_payable_usd: 4,
        total_user_charged_usd: 5,
        avg_latency_ms: 12,
        avg_request_total_ms: 14,
        avg_first_response_byte_ms: 3
      })
      .mockResolvedValueOnce({ items: null, total: 0 })
      .mockResolvedValueOnce({
        items: [{ request_id: "req-1", model_code: "gpt-4o", request_status: "failed", error_code: "timeout", error_message: "timeout", http_status: 504, created_at: 100, protocol_conversion_enabled: false }],
        total: 1
      })
      .mockResolvedValueOnce({ items: [{ id: "policy-1", scope_type: "other", scope_id: "user-1", status: "active" }], total: 1 });

    await expect(aiTenantApi.getDashboardSummary({ date_from: "2026-08-01", date_to: "2026-08-02" })).resolves.toMatchObject({
      total_requests: 10,
      avg_latency_ms: 12
    });
    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      query: { date_from: "2026-08-01", date_to: "2026-08-02" }
    });
    await expect(aiTenantApi.getDashboardTopModels({ limit: 5 })).resolves.toEqual({ items: [], total: 0 });
    await expect(aiTenantApi.listDashboardRecentErrors({ limit: 5 })).resolves.toMatchObject({
      items: [{ request_id: "req-1", error_code: "timeout" }],
      total: 1
    });
    await expect(aiTenantApi.listUserLimitPolicies("user-1")).rejects.toThrow("Unexpected limit policy scope");
  });
});
