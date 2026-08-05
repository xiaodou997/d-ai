import { nextTick, shallowRef } from "vue";
import { describe, expect, it, vi } from "vitest";

vi.mock("@/api/aiTenant", () => ({ aiTenantApi: {} }));

import type { TenantAiDispatchRule, TenantAiDispatchRuleWriteRequest } from "@/api/types/aiTenant";
import { useGroupDispatchRules, type GroupDispatchRulesApi } from "./useGroupDispatchRules";

function rule(patch: Partial<TenantAiDispatchRule> = {}): TenantAiDispatchRule {
  return {
    id: "rule-1",
    group_id: "group-1",
    client_surface: "openai_chat",
    match_type: "exact",
    match_value: "gpt-latest",
    target_model_code: "gpt-5.5",
    priority: 10,
    status: "active",
    can_enable: true,
    required_capability: "chat",
    price_state: "priced",
    ...patch
  };
}

function createApi(initial: TenantAiDispatchRule[] = [rule()]): GroupDispatchRulesApi {
  let stored = [...initial];
  return {
    list: vi.fn(async () => ({ items: stored, total: stored.length })),
    listModels: vi.fn(async () => ({ items: [{ model_code: "gpt-5.5", capability: "chat", available_targets: 1 }], total: 1 })),
    create: vi.fn(async (groupId, body) => {
      const saved = rule({ ...body, id: `rule-${stored.length + 1}`, group_id: groupId });
      stored = [...stored, saved];
      return saved;
    }),
    update: vi.fn(async (_groupId, ruleId, body) => {
      const saved = rule({ ...stored.find((item) => item.id === ruleId), ...body, id: ruleId });
      stored = stored.map((item) => item.id === ruleId ? saved : item);
      return saved;
    }),
    updateStatus: vi.fn(async (_groupId, ruleId, status) => {
      const saved = rule({ ...stored.find((item) => item.id === ruleId), id: ruleId, status });
      stored = stored.map((item) => item.id === ruleId ? saved : item);
      return saved;
    }),
    remove: vi.fn(async (_groupId, ruleId) => {
      stored = stored.filter((item) => item.id !== ruleId);
    })
  };
}

const write: TenantAiDispatchRuleWriteRequest = {
  client_surface: "openai_responses",
  match_type: "prefix",
  match_value: "gpt-",
  target_model_code: "gpt-5.5",
  priority: 20,
  notes: "responses clients"
};

describe("useGroupDispatchRules", () => {
  it("uses the dedicated status command and server price eligibility", async () => {
    const api = createApi([rule({ can_enable: false, price_state: "unpriced", status: "disabled" })]);
    const state = useGroupDispatchRules({ groupId: () => "group-1", api });
    await state.load();

    expect(state.unpricedRuleIds.value.has("rule-1")).toBe(true);
    await state.loadModels("openai_chat");
    expect(api.listModels).toHaveBeenCalledWith("group-1", "openai_chat");
    expect(state.models.value[0]?.model_code).toBe("gpt-5.5");
    await state.updateStatus("rule-1", "active");
    expect(api.updateStatus).toHaveBeenCalledWith("group-1", "rule-1", "active");
    expect(api.update).not.toHaveBeenCalled();
  });

  it("creates and edits without tenant, group, status, or provider-family body fields", async () => {
    const api = createApi([]);
    const state = useGroupDispatchRules({ groupId: () => "group-1", api });
    await state.load();

    const created = await state.create(write);
    expect(api.create).toHaveBeenCalledWith("group-1", write);
    await state.update(created.id, { ...write, match_value: "gpt-5" });
    expect(api.update).toHaveBeenCalledWith("group-1", created.id, { ...write, match_value: "gpt-5" });
  });

  it("does not let a previous group response replace the current group", async () => {
    let resolveFirst!: (value: { items: TenantAiDispatchRule[] | null; total: number }) => void;
    const groupId = shallowRef("group-1");
    const api = createApi();
    api.list = vi.fn()
      .mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve; }))
      .mockResolvedValueOnce({ items: [rule({ id: "rule-2", group_id: "group-2" })], total: 1 });
    const state = useGroupDispatchRules({ groupId: () => groupId.value, api });

    groupId.value = "group-2";
    await nextTick();
    await nextTick();
    resolveFirst({ items: [rule()], total: 1 });
    await nextTick();

    expect(state.rules.value.map((item) => item.id)).toEqual(["rule-2"]);
  });
});
