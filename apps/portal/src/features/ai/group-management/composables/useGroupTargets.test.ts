import { nextTick, shallowRef } from "vue";
import { describe, expect, it, vi } from "vitest";

vi.mock("@/api/aiTenant", () => ({ aiTenantApi: {} }));

import type {
  TenantAiGroupTarget,
  TenantAiUpstreamResource
} from "@/api/types/aiTenant";
import { useGroupTargets, type GroupTargetsApi } from "./useGroupTargets";

const resources: TenantAiUpstreamResource[] = [
  {
    id: "account-1",
    resource_kind: "direct_upstream",
    name: "主 API",
    tenant_multiplier: 0.8,
    models: [{ model_code: "chat-1", capability_type: "chat", api_format: "openai_chat", availability: "available" }]
  },
  {
    id: "pool-1",
    resource_kind: "oauth_pool",
    name: "OAuth 池",
    tenant_multiplier: 1.2,
    models: [{ model_code: "chat-2", capability_type: "chat", api_format: "openai_chat", availability: "available" }]
  },
  {
    id: "account-unpriced",
    resource_kind: "direct_upstream",
    name: "无价格 API",
    tenant_multiplier: 1,
    models: [{ model_code: "missing", capability_type: "chat", api_format: "openai_chat", availability: "no_price_configured" }]
  }
];

function target(id = "binding-1"): TenantAiGroupTarget {
  return {
    id,
    group_id: "group-1",
    account_id: "account-1",
    target_type: "account",
    account_name: "主 API",
    status: "active"
  };
}

function createApi(initial: TenantAiGroupTarget[] = [target()]) {
  let stored = [...initial];
  const api: GroupTargetsApi = {
    listTargets: vi.fn(async () => ({ items: stored, total: stored.length })),
    listResources: vi.fn(async () => ({ items: resources, total: resources.length })),
    add: vi.fn(async (_groupId, body) => {
      const saved: TenantAiGroupTarget = {
        id: `binding-${stored.length + 1}`,
        group_id: "group-1",
        account_id: body.account_id,
        credential_pool_id: body.credential_pool_id,
        target_type: body.account_id ? "account" : "pool",
        status: body.status ?? "active"
      };
      stored = [...stored, saved];
      return saved;
    }),
    update: vi.fn(async (_groupId, bindingId, body) => {
      const saved = { ...stored.find((item) => item.id === bindingId)!, ...body };
      stored = stored.map((item) => item.id === bindingId ? saved : item);
      return saved;
    }),
    remove: vi.fn(async (_groupId, bindingId) => {
      stored = stored.filter((item) => item.id !== bindingId);
    })
  };
  return api;
}

describe("useGroupTargets", () => {
  it("maps API and OAuth resources, blocks unpriced additions, and saves status", async () => {
    const api = createApi();
    const state = useGroupTargets({ groupId: () => "group-1", api });
    await state.load();

    expect(state.rows.value.map((row) => [row.key, row.tenantMultiplier])).toEqual([
      ["direct_upstream:account-1", 0.8],
      ["direct_upstream:account-unpriced", 1],
      ["oauth_pool:pool-1", 1.2]
    ]);
    state.setSelected("direct_upstream:account-unpriced", true);
    expect(state.rows.value.find((row) => row.targetId === "account-unpriced")?.selected).toBe(false);

    state.updateDraft("oauth_pool:pool-1", { status: "disabled" });
    state.setSelected("oauth_pool:pool-1", true);
    await expect(state.save()).resolves.toMatchObject({ added: 1, failures: [] });
    expect(api.add).toHaveBeenCalledWith("group-1", {
      credential_pool_id: "pool-1",
      status: "disabled"
    });
  });

  it("updates status", async () => {
    const api = createApi();
    const state = useGroupTargets({ groupId: () => "group-1", api });
    await state.load();

    state.updateDraft("direct_upstream:account-1", { status: "disabled" });
    await expect(state.save()).resolves.toMatchObject({ updated: 1, failures: [] });
    expect(api.update).toHaveBeenCalledWith("group-1", "binding-1", {
      status: "disabled"
    });
  });

  it("uses one versioned replacement and reports the captured diff", async () => {
    const api = createApi();
    api.listTargets = vi.fn(async () => ({ items: [target()], total: 1, route_policy_version: 7 }));
    api.replace = vi.fn(async (_groupId: string, body: Parameters<NonNullable<GroupTargetsApi["replace"]>>[1]) => ({
      items: [
        target(),
        {
          id: "binding-2",
          group_id: "group-1",
          credential_pool_id: body.targets.find((item) => item.credential_pool_id)?.credential_pool_id,
          target_type: "pool" as const,
          pool_name: "OAuth 池",
          status: "active" as const
        }
      ],
      total: 2,
      route_policy_version: 8
    }));
    const state = useGroupTargets({ groupId: () => "group-1", api });
    await state.load();
    state.updateDraft("direct_upstream:account-1", { status: "disabled" });
    state.setSelected("oauth_pool:pool-1", true);

    await expect(state.save()).resolves.toEqual({ added: 1, updated: 1, removed: 0, failures: [] });
    expect(api.replace).toHaveBeenCalledWith("group-1", expect.objectContaining({
      expected_version: 7,
      targets: expect.arrayContaining([
        expect.objectContaining({ account_id: "account-1", status: "disabled" }),
        expect.objectContaining({ credential_pool_id: "pool-1" })
      ])
    }));
    expect(state.routePolicyVersion.value).toBe(8);
  });

  it("keeps successful removals and failed additions as retryable row changes", async () => {
    const api = createApi();
    api.add = vi.fn(async () => { throw new Error("资源不可用"); });
    const state = useGroupTargets({ groupId: () => "group-1", api });
    await state.load();
    state.setSelected("direct_upstream:account-1", false);
    state.setSelected("oauth_pool:pool-1", true);

    await expect(state.save()).resolves.toEqual({
      added: 0,
      updated: 0,
      removed: 1,
      failures: [{ action: "add", targetKey: "oauth_pool:pool-1", targetName: "OAuth 池", message: "资源不可用" }]
    });
    expect(state.rows.value.find((row) => row.targetId === "account-1")?.linked).toBe(false);
    expect(state.rows.value.find((row) => row.targetId === "pool-1")).toMatchObject({ selected: true, change: "add" });
  });

  it("surfaces a revoked/inactive binding's unavailability on the linked row", async () => {
    const api = createApi([
      { ...target("binding-1"), available: false, unavailable_reason: "access_revoked" }
    ]);
    const state = useGroupTargets({ groupId: () => "group-1", api });
    await state.load();

    const linked = state.rows.value.find((row) => row.key === "direct_upstream:account-1");
    expect(linked?.linked).toBe(true);
    expect(linked?.bindingUnavailableReason).toBe("access_revoked");
    // An available binding (or one from an older response without the field) stays clean.
    const other = state.rows.value.find((row) => row.key === "oauth_pool:pool-1");
    expect(other?.bindingUnavailableReason).toBeNull();
  });

  it("marks a binding whose resource vanished from the directory as missing", async () => {
    const api = createApi([
      { ...target("binding-gone"), account_id: "account-gone", account_name: "已删资源", available: false, unavailable_reason: "missing" }
    ]);
    const state = useGroupTargets({ groupId: () => "group-1", api });
    await state.load();

    const gone = state.rows.value.find((row) => row.targetId === "account-gone");
    expect(gone).toMatchObject({ linked: true, resourceState: "missing", bindingUnavailableReason: "missing" });
  });

  it("ignores stale directory results after switching groups", async () => {
    let resolveFirst!: (value: { items: TenantAiUpstreamResource[]; total: number }) => void;
    const groupId = shallowRef("group-1");
    const api = createApi();
    api.listTargets = vi.fn(async (id) => ({ items: id === "group-2" ? [] : [target()], total: id === "group-2" ? 0 : 1 }));
    api.listResources = vi.fn()
      .mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve; }))
      .mockResolvedValueOnce({ items: [resources[1]], total: 1 });
    const state = useGroupTargets({ groupId: () => groupId.value, api });
    groupId.value = "group-2";
    await nextTick();
    await nextTick();
    resolveFirst({ items: resources, total: resources.length });
    await nextTick();

    expect(state.rows.value.map((row) => row.key)).toEqual(["oauth_pool:pool-1"]);
  });
});
