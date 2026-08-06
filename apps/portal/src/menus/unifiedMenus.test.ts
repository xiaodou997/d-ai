import { describe, expect, it } from "vitest";

import { buildUnifiedNav } from "./unifiedMenus";

function pathsFor(userType: number): string[] {
  return buildUnifiedNav(userType).flatMap((module) =>
    (module.children ?? []).flatMap((group) =>
      (group.children ?? []).map((item) => item.to).filter((path): path is string => Boolean(path))
    )
  );
}

describe("buildUnifiedNav", () => {
  it("keeps platform-only finance pages out of tenant and customer menus", () => {
    expect(pathsFor(3)).not.toContain("/billing/tenant-credit");
    expect(pathsFor(3)).not.toContain("/billing/cash-accounts");
    expect(pathsFor(4)).not.toContain("/billing/tenant-credit");
  });

  it("separates admin usage from tenant and customer usage", () => {
    expect(pathsFor(1)).toContain("/ai-gateway/usage");
    expect(pathsFor(3)).not.toContain("/ai-gateway/usage");
    expect(pathsFor(3)).toContain("/workspace/usage-records");
    expect(pathsFor(4)).toContain("/workspace/usage-records");
  });

  it("shows tenant-owned subscription management only to tenants", () => {
    expect(pathsFor(1)).not.toContain("/ai-gateway/subscriptions");
    expect(pathsFor(2)).not.toContain("/ai-gateway/subscriptions");
    expect(pathsFor(3)).toContain("/ai-gateway/subscriptions");
  });
});
