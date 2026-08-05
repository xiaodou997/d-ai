import { describe, expect, it } from "vitest";

import { editablePolicyForActor } from "./policy";

describe("editablePolicyForActor", () => {
  it("preserves an all policy for a superadmin", () => {
    expect(editablePolicyForActor({ mode: "all", serviceIds: [] }, 1, ["ai"])).toEqual({
      mode: "all",
      serviceIds: []
    });
  });

  it("projects all access onto a platform administrator's capabilities", () => {
    expect(editablePolicyForActor({ mode: "all", serviceIds: [] }, 2, ["ai"])).toEqual({
      mode: "selected",
      serviceIds: ["ai"]
    });
  });

  it("removes selected services outside a platform administrator's capabilities", () => {
    expect(editablePolicyForActor({ mode: "selected", serviceIds: ["ai", "billing"] }, 2, ["ai"])).toEqual({
      mode: "selected",
      serviceIds: ["ai"]
    });
  });
});
