import { describe, expect, it } from "vitest";

import { audiencePresetToSelections, audienceRulesToEditorState } from "./audience";

describe("announcement audience mapping", () => {
  it("maps all recipients to the three global audience rules", () => {
    expect(audiencePresetToSelections("all")).toEqual([
      { kind: "admin", scope: "all" },
      { kind: "tenant_user", scope: "all" },
      { kind: "end_user", scope: "all" }
    ]);
  });

  it("maps selected tenants and recipient kinds without duplicate tenant ids", () => {
    expect(audiencePresetToSelections("selected", ["tenant-a", "tenant-a", "tenant-b"], ["tenant_user", "end_user"]))
      .toEqual([
        { kind: "tenant_user", scope: "tenant", tenantIds: ["tenant-a", "tenant-b"] },
        { kind: "end_user", scope: "tenant", tenantIds: ["tenant-a", "tenant-b"] }
      ]);
  });

  it("restores a selected audience for draft editing", () => {
    expect(audienceRulesToEditorState([
      { kind: "tenant_user", scope: "tenant", tenantId: "tenant-a" },
      { kind: "end_user", scope: "tenant", tenantId: "tenant-a" }
    ])).toEqual({
      preset: "selected",
      tenantIds: ["tenant-a"],
      selectedKinds: ["tenant_user", "end_user"]
    });
  });
});
