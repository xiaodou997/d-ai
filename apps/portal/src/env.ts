import type { PortalEnv, PortalThemeName } from "@dai/app-core";

/**
 * 统一 Portal 环境 —— 不再有固定的 portal 值。
 * 主题由 userType 运行时决定。
 */
export interface UnifiedPortalEnv extends Omit<PortalEnv, "portal" | "theme"> {
  portal: "unified";
}

export const portalEnv: UnifiedPortalEnv = {
  portal: "unified",
  apiBaseUrl: (import.meta.env.VITE_API_BASE_URL as string) || "",
  appVersion: (import.meta.env.VITE_APP_VERSION as string) || "dev",
  legalBaseUrl: (import.meta.env.VITE_LEGAL_BASE_URL as string) || "/legal",
  serviceClientIds: {
    urm: "dai-portal",
    ai: "dai-portal",
    proxy: "dai-portal"
  }
} as UnifiedPortalEnv;

/**
 * 根据 userType 计算主题
 * 1 = 超级管理员 → admin（紫）
 * 2 = 平台管理员 → admin（紫）
 * 3 = 租户 → tenant（蓝）
 * 4 = 终端用户 → customer（赤陶）
 */
export function themeForUserType(userType: number | undefined): PortalThemeName {
  if (userType === undefined) return "admin";
  if (userType <= 2) return "admin";
  if (userType === 3) return "tenant";
  return "customer";
}
