import { afterEach, describe, expect, it } from "vitest";

import { applyPortalTheme } from "./theme";

afterEach(() => {
  const root = document.documentElement;
  delete root.dataset.theme;
  root.classList.remove("ds-theme-admin", "ds-theme-tenant", "ds-theme-customer");
});

describe("applyPortalTheme", () => {
  it("synchronizes the active theme on the document root", () => {
    const root = document.documentElement;
    root.classList.add("unrelated-class");

    applyPortalTheme("tenant");

    expect(root.dataset.theme).toBe("tenant");
    expect(root.classList.contains("ds-theme-tenant")).toBe(true);

    applyPortalTheme("customer");

    expect(root.dataset.theme).toBe("customer");
    expect(root.classList.contains("ds-theme-tenant")).toBe(false);
    expect(root.classList.contains("ds-theme-customer")).toBe(true);
    expect(root.classList.contains("unrelated-class")).toBe(true);

    root.classList.remove("unrelated-class");
  });
});
