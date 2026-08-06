export type PortalThemeName = "admin" | "tenant" | "customer";

export interface PortalThemeMeta {
  name: PortalThemeName;
  label: string;
  accent: string;
  accentSoft: string;
  surface: string;
}

export const portalThemes: Record<PortalThemeName, PortalThemeMeta> = {
  admin: {
    name: "admin",
    label: "管理平台",
    accent: "#7C3AED",
    accentSoft: "#F3F0FF",
    surface: "#F6F7F9"
  },
  tenant: {
    name: "tenant",
    label: "租户平台",
    accent: "#2563EB",
    accentSoft: "#EFF6FF",
    surface: "#F6F7F9"
  },
  customer: {
    name: "customer",
    label: "用户平台",
    accent: "#B0603C",
    accentSoft: "#F4E5DB",
    surface: "#F6F7F9"
  }
};

export function resolvePortalTheme(name: PortalThemeName): PortalThemeMeta {
  return portalThemes[name];
}

const portalThemeClasses = Object.keys(portalThemes).map((name) => `ds-theme-${name}`);

export function applyPortalTheme(name: PortalThemeName): void {
  if (typeof document === "undefined") return;

  const root = document.documentElement;
  root.dataset.theme = name;
  root.classList.remove(...portalThemeClasses);
  root.classList.add(`ds-theme-${name}`);
}
