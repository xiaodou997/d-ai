import { describe, expect, it } from "vitest";

import type { TenantAiDispatchRule } from "../../../types/aiTenant";
import {
  eligibleModels,
  isRulePriced,
  selectableDispatchModels,
  validateMatchPattern
} from "./dispatchRulePolicy";

const entries = [
  { model_code: "gpt-chat", capability_type: "chat" },
  { model_code: "gpt-chat", capability_type: "embedding" },
  { model_code: "embed-only", capability_type: "embedding" },
  { model_code: "image-only", capability_type: "image" }
];

function rule(patch: Partial<TenantAiDispatchRule> = {}): TenantAiDispatchRule {
  return {
    id: "rule-1",
    group_id: "group-1",
    client_surface: "openai_embeddings",
    match_type: "exact",
    match_value: "public-model",
    target_model_code: "embed-only",
    priority: 100,
    status: "active",
    can_enable: true,
    ...patch
  };
}

describe("dispatch rule price policy", () => {
  it("filters target models by API capability and removes duplicate model codes", () => {
    expect(eligibleModels(entries, "openai_chat").map((item) => item.model_code)).toEqual(["gpt-chat"]);
    expect(eligibleModels(entries, "gemini_embeddings").map((item) => item.model_code)).toEqual(["embed-only", "gpt-chat"]);
    expect(eligibleModels(entries, "openai_images").map((item) => item.model_code)).toEqual(["image-only"]);
  });

  it("marks historical rules invalid when the exact model capability price is absent", () => {
    expect(isRulePriced(rule(), entries)).toBe(true);
    expect(isRulePriced(rule({ target_model_code: "gpt-chat" }), [{ model_code: "gpt-chat", capability_type: "chat" }])).toBe(false);
  });

  it("hides zero-candidate models from the picker without removing priced model knowledge", () => {
    const models = [
      { model_code: "gpt-5.5", capability: "chat", available_targets: 0 },
      { model_code: "gpt-5.6-luna", capability: "chat", available_targets: 1 }
    ];

    expect(selectableDispatchModels(models).map((item) => item.model_code)).toEqual(["gpt-5.6-luna"]);
    expect(models.some((item) => item.model_code === "gpt-5.5")).toBe(true);
  });

  it("validates regular expressions before save", () => {
    expect(validateMatchPattern("regex", "^gpt-(4|5)$")).toBe("");
    expect(validateMatchPattern("regex", "[")).toContain("正则表达式");
    expect(validateMatchPattern("exact", "   ")).toContain("匹配值");
  });
});
