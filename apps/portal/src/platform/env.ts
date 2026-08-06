import type { PortalThemeName } from "@/shared/ui";

export type PortalKind = "unified";

export interface PortalBuildEnv {
  [key: string]: unknown;
  VITE_APP_VERSION?: string;
  VITE_API_BASE_URL?: string;
  VITE_PUBLIC_BASE_URL?: string;
  VITE_LEGAL_BASE_URL?: string;
}

export interface PortalEnv {
  portal: PortalKind;
  title: string;
  appVersion: string;
  theme: PortalThemeName;
  storagePrefix: string;
  apiBaseUrl: string;
  publicBaseUrl: string;
  legalBaseUrl: string;
}

export function createPortalEnv(input: PortalEnv): PortalEnv {
  return { ...input };
}

export interface StandardPortalEnvOptions {
  portal?: PortalKind;
  env: PortalBuildEnv;
  title?: string;
  theme?: PortalThemeName;
  storagePrefix?: string;
}

export function createStandardPortalEnv(options: StandardPortalEnvOptions): PortalEnv {
  const buildEnv = options.env;
  const apiBaseUrl = buildEnv.VITE_API_BASE_URL?.trim() || "/";

  return createPortalEnv({
    portal: options.portal || "unified",
    title: options.title || "D-AI 统一平台",
    appVersion: buildEnv.VITE_APP_VERSION?.trim() || "0.0.1",
    theme: options.theme || "admin",
    storagePrefix: options.storagePrefix || "dai:portal",
    apiBaseUrl,
    publicBaseUrl: buildEnv.VITE_PUBLIC_BASE_URL?.trim() || apiBaseUrl,
    legalBaseUrl: buildEnv.VITE_LEGAL_BASE_URL || "/legal"
  });
}
