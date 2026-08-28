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

const policy = {
  lifetime_max_purchases: null,
  period_type: "rolling",
  period_max_purchases: 2,
  rolling_window_hours: 24,
  calendar_unit: undefined,
  allow_advance_purchase: true,
  version: 3
};

const plan = {
  id: "plan-1",
  tenant_id: "tenant-1",
  name: "Starter",
  description: "Starter plan",
  price_micro_usd: 1_000_000,
  duration_days: 7,
  total_limit_micro_usd: 5_000_000,
  window_5h_limit_micro_usd: null,
  window_7d_limit_micro_usd: 4_000_000,
  status: "on_sale",
  sort_order: 1,
  sale_limit: null,
  sold_count: 2,
  reserved_count: 1,
  available_count: null,
  sold_out: false,
  groups: [{ id: "group-1", name: "Default", quota_debit_multiplier: 1 }],
  purchase_policy: policy,
  created_at: "2026-08-01T00:00:00Z",
  updated_at: "2026-08-02T00:00:00Z",
  $schema: "ignored"
};

describe("AI tenant subscription generated operation facade", () => {
  it("maps plan pages and policies while normalizing nullable groups", async () => {
    mocks.request.mockResolvedValueOnce({
      items: [{ ...plan, groups: null }],
      total: 1,
      page: 1,
      size: 20,
      included: { users: {}, tenants: {} },
      $schema: "ignored"
    });

    await expect(aiTenantApi.listSubscriptionPlans({ status: "on_sale", limit: 20, offset: 0 })).resolves.toMatchObject({
      items: [{ id: "plan-1", groups: [], purchase_policy: { period_type: "rolling", version: 3 } }],
      total: 1,
      page: 1,
      size: 20,
      included: { users: {}, tenants: {} }
    });
    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      query: { status: "on_sale", limit: 20, offset: 0 },
      path: "/api/v1/tenants/me/subscription-plans"
    });
  });

  it("uses generated write/path contracts and preserves the 204 reorder facade result", async () => {
    mocks.request
      .mockResolvedValueOnce(plan)
      .mockResolvedValueOnce(plan)
      .mockResolvedValueOnce(undefined)
      .mockResolvedValueOnce(plan);

    const body = {
      name: "Starter",
      description: "Starter plan",
      price_micro_usd: 1_000_000,
      duration_days: 7,
      total_limit_micro_usd: 5_000_000,
      window_5h_limit_micro_usd: null,
      window_7d_limit_micro_usd: null,
      sale_limit: null,
      groups: [{ group_id: "group-1", quota_debit_multiplier: 1 }],
      purchase_policy: {
        lifetime_max_purchases: null,
        period_type: "none" as const,
        period_max_purchases: null,
        rolling_window_hours: null,
        calendar_unit: "" as const,
        allow_advance_purchase: true
      }
    };

    await expect(aiTenantApi.createSubscriptionPlan(body)).resolves.toMatchObject({ id: "plan-1" });
    await expect(aiTenantApi.updateSubscriptionPlan("plan/1", body)).resolves.toMatchObject({ id: "plan-1" });
    await expect(aiTenantApi.reorderSubscriptionPlans(["plan-1", "plan-2"])).resolves.toEqual({});
    await expect(aiTenantApi.setSubscriptionPlanStatus("plan/1", "off_sale")).resolves.toMatchObject({ id: "plan-1" });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      body: { name: "Starter", groups: [{ group_id: "group-1" }], purchase_policy: { period_type: "none", calendar_unit: undefined } }
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/tenants/me/subscription-plans/plan%2F1",
      pathParams: { planID: "plan/1" }
    });
    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({ body: { plan_ids: ["plan-1", "plan-2"] } });
    expect(mocks.request.mock.calls[3]?.[0]).toMatchObject({
      pathParams: { planID: "plan/1" },
      body: { status: "off_sale" }
    });
  });

  it("maps order/subscription pages and policy revisions with nullable collections", async () => {
    const order = {
      id: "order-1",
      order_no: "SO-1",
      tenant_id: "tenant-1",
      user_id: "user-1",
      plan_id: "plan-1",
      plan_name: "Starter",
      price_micro_usd: 1_000_000,
      status: "paid",
      purchase_policy_version: 3,
      purchase_policy: policy,
      created_at: "2026-08-01T00:00:00Z",
      updated_at: "2026-08-01T00:00:00Z"
    };
    const subscription = {
      id: "sub-1",
      tenant_id: "tenant-1",
      user_id: "user-1",
      plan_id: "plan-1",
      order_id: "order-1",
      plan_name: "Starter",
      duration_days: 7,
      status: "active",
      total_limit_micro_usd: 5_000_000,
      total_used_micro_usd: 1_000,
      total_remaining_micro_usd: 4_999_000,
      window_5h: { limit_micro_usd: null, used_micro_usd: 1_000, remaining_micro_usd: null, reset_at: null },
      window_7d: { limit_micro_usd: 4_000_000, used_micro_usd: 1_000, remaining_micro_usd: 3_999_000, reset_at: "2026-08-08T00:00:00Z" },
      groups: null,
      created_at: "2026-08-01T00:00:00Z",
      updated_at: "2026-08-01T00:00:00Z"
    };

    mocks.request
      .mockResolvedValueOnce({ items: [order], total: 1, page: 1, size: 20, included: { users: {}, tenants: {} } })
      .mockResolvedValueOnce({ items: [subscription], total: 1, page: 1, size: 20, included: { users: {}, tenants: {} } })
      .mockResolvedValueOnce({ items: [{ plan_id: "plan-1", version: 3, policy, changed_at: "2026-08-01T00:00:00Z" }] });

    await expect(aiTenantApi.listSubscriptionOrders({ user_id: "user-1", limit: 20, offset: 0 })).resolves.toMatchObject({
      items: [{ id: "order-1", purchase_policy: { version: 3 } }]
    });
    await expect(aiTenantApi.listSubscriptions({ status: "active" })).resolves.toMatchObject({
      items: [{ id: "sub-1", groups: [] }]
    });
    await expect(aiTenantApi.listSubscriptionPlanPurchasePolicyRevisions("plan/1")).resolves.toMatchObject({
      items: [{ plan_id: "plan-1", policy: { period_type: "rolling" } }]
    });
    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({
      path: "/api/v1/tenants/me/subscription-plans/plan%2F1/purchase-policy-revisions",
      pathParams: { planID: "plan/1" }
    });
  });
});
