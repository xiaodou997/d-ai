import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/api",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { aiAdminApi } from "./aiAdmin";

beforeEach(() => mocks.request.mockReset());

describe("AI admin observability generated operation facade", () => {
  it("maps dashboard summary and forwards the generated query shape", async () => {
    mocks.request.mockResolvedValueOnce({
      total_requests: 12,
      successful_requests: 10,
      failed_requests: 2,
      total_tokens: 100,
      total_prompt_tokens: 60,
      total_completion_tokens: 40,
      total_catalog_base_usd: 1.2,
      total_tenant_payable_usd: 1.5,
      total_retail_base_usd: 2,
      total_user_payable_usd: 2.2,
      total_user_charged_usd: 2.1,
      avg_latency_ms: 12.5,
      avg_request_total_ms: 20,
      avg_first_response_byte_ms: 4,
      $schema: "ignored"
    });

    await expect(aiAdminApi.getDashboardSummary({
      tenant_id: "tenant-1",
      user_id: "user-1",
      date_from: "2026-08-01",
      date_to: "2026-08-02"
    })).resolves.toMatchObject({ total_requests: 12, avg_latency_ms: 12.5 });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/dashboard/summary",
      query: {
        tenant_id: "tenant-1",
        user_id: "user-1",
        date_from: "2026-08-01",
        date_to: "2026-08-02"
      }
    });
  });

  it("normalizes nullable dashboard collections and strips transport schema", async () => {
    mocks.request
      .mockResolvedValueOnce({
        items: [{ model_code: "gpt-4o", request_count: 8, total_tokens: 80, total_tenant_payable_usd: 1.1, $schema: "ignored" }],
        total: 1
      })
      .mockResolvedValueOnce({
        items: [{ tenant_id: "tenant-1", request_count: 6, total_tokens: 60, total_tenant_payable_usd: 0.9 }],
        total: 1,
        included: { users: {}, tenants: { "tenant-1": { tenant_id: "tenant-1", tenant_name: "Tenant One" } } }
      })
      .mockResolvedValueOnce({ items: null, total: 0 });

    await expect(aiAdminApi.listDashboardTopModels({ limit: 8 })).resolves.toEqual({ items: [{ model_code: "gpt-4o", request_count: 8, total_tokens: 80, total_tenant_payable_usd: 1.1 }], total: 1 });
    await expect(aiAdminApi.listDashboardTopTenants({ limit: 8 })).resolves.toMatchObject({
      items: [{ tenant_id: "tenant-1" }],
      total: 1,
      included: { tenants: { "tenant-1": { tenant_name: "Tenant One" } } }
    });
    await expect(aiAdminApi.listDashboardRecentErrors({ limit: 5 })).resolves.toEqual({ items: [], total: 0 });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({ query: { limit: 8 } });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({ query: { limit: 8 } });
    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({ query: { limit: 5 } });
  });

  it("maps audit logs and forwards only the declared limit query", async () => {
    mocks.request.mockResolvedValueOnce({
      items: [{
        id: "audit-1",
        actor: "admin-1",
        action: "update_account",
        object_type: "upstream_account",
        object_id: "account-1",
        request_summary: { status: "active" },
        result: "success",
        http_status: 200,
        created_at: 100,
        $schema: "ignored"
      }],
      total: 1
    });

    await expect(aiAdminApi.listGatewayAuditLogs({ limit: 100 })).resolves.toEqual({
      items: [{
        id: "audit-1",
        actor: "admin-1",
        action: "update_account",
        object_type: "upstream_account",
        object_id: "account-1",
        request_summary: { status: "active" },
        result: "success",
        http_status: 200,
        created_at: 100
      }],
      total: 1
    });
    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({ path: "/api/v1/audit-logs", query: { limit: 100 } });
  });
});
