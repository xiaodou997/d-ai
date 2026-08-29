import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { applyPortalTheme, portalThemes, type PortalThemeName } from "./theme";

const baseCss = readFileSync(resolve(process.cwd(), "apps/portal/src/shared/ui/styles/base.css"), "utf8");
let baseStyle: HTMLStyleElement;

beforeEach(() => {
  baseStyle = document.createElement("style");
  baseStyle.dataset.test = "ds-theme-contract";
  baseStyle.textContent = baseCss;
  document.head.append(baseStyle);
});

afterEach(() => {
  const root = document.documentElement;
  delete root.dataset.theme;
  root.classList.remove("ds-theme-admin", "ds-theme-tenant", "ds-theme-customer");
  baseStyle.remove();
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

  it.each(Object.keys(portalThemes) as PortalThemeName[])(
    "keeps the %s theme's accent and Element Plus bridge tokens aligned",
    (name) => {
      applyPortalTheme(name);
      const styles = getComputedStyle(document.documentElement);
      const meta = portalThemes[name];
      const preview = document.createElement("button");
      preview.style.cssText =
        "background: var(--ds-accent); color: var(--ds-accent-contrast); border-radius: var(--ds-radius-control); box-shadow: var(--ds-shadow-focus);";
      document.body.append(preview);
      const previewStyles = getComputedStyle(preview);

      expect(styles.getPropertyValue("--ds-accent").trim().toLowerCase()).toBe(meta.accent.toLowerCase());
      expect(styles.getPropertyValue("--ds-accent-soft").trim().toLowerCase()).toBe(meta.accentSoft.toLowerCase());
      expect(styles.getPropertyValue("--ds-accent-hover").trim().toLowerCase()).not.toBe("");
      expect(styles.getPropertyValue("--ds-paper").trim().toLowerCase()).toBe(meta.surface.toLowerCase());
      expect(styles.getPropertyValue("--el-color-primary").trim().toLowerCase()).toBe(meta.accent.toLowerCase());
      expect(styles.getPropertyValue("--el-component-size").trim()).toBe("36px");
      expect(styles.getPropertyValue("--el-border-radius-base").trim()).toBe("8px");
      expect(previewStyles.backgroundColor.toLowerCase()).toBe(meta.accent.toLowerCase());
      expect(previewStyles.borderRadius).toBe("8px");
      expect(previewStyles.boxShadow).toContain(meta.accentSoft.toLowerCase());

      preview.remove();
    }
  );
});
