import type { PortalThemeName } from "@/shared/ui";

export type PortalKind = "unified";

export interface PortalBuildEnv {
  [key: string]: unknown;
  VITE_APP_VERSION?: string;
}

export interface PortalEnv {
  portal: PortalKind;
  title: string;
  appVersion: string;
  theme: PortalThemeName;
  storagePrefix: string;
  apiBaseUrl: string;
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

  return createPortalEnv({
    portal: options.portal || "unified",
    title: options.title || "D-AI 统一平台",
    appVersion: buildEnv.VITE_APP_VERSION?.trim() || "0.3.3",
    theme: options.theme || "admin",
    storagePrefix: options.storagePrefix || "dai:portal",
    apiBaseUrl: "/"
  });
}
