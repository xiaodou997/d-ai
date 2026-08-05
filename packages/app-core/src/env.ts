import type { PortalThemeName } from "@dai/ui";

export type PortalKind = "admin" | "tenant" | "customer";
export type BackendService = "urm" | "ai" | "proxy";

export interface PortalServiceModuleDefinition {
  service: BackendService;
  label: string;
  clientIDEnvKey: "VITE_URM_CLIENT_ID" | "VITE_AI_CLIENT_ID" | "VITE_PROXY_CLIENT_ID";
  defaultClientID?: string;
  alwaysEnabled?: boolean;
}

export const PORTAL_SERVICE_MODULES = [
  {
    service: "urm",
    label: "用户中心",
    clientIDEnvKey: "VITE_URM_CLIENT_ID",
    alwaysEnabled: true
  },
  {
    service: "ai",
    label: "智能服务",
    clientIDEnvKey: "VITE_AI_CLIENT_ID",
    defaultClientID: "uni-ai-api"
  },
  {
    service: "proxy",
    label: "接口代理",
    clientIDEnvKey: "VITE_PROXY_CLIENT_ID",
    defaultClientID: "uni-api-proxy"
  }
] as const satisfies readonly PortalServiceModuleDefinition[];

export interface PortalBuildEnv {
  [key: string]: unknown;
  VITE_APP_VERSION?: string;
  VITE_X_CLIENT_ID?: string;
  VITE_URM_CLIENT_ID?: string;
  VITE_AI_CLIENT_ID?: string;
  VITE_PROXY_CLIENT_ID?: string;
  VITE_URM_BASE_URL?: string;
  VITE_AI_BASE_URL?: string;
  VITE_AI_PUBLIC_BASE_URL?: string;
  VITE_PROXY_BASE_URL?: string;
  VITE_SSO_AUTHORIZE_URL?: string;
  VITE_LEGAL_BASE_URL?: string;
}

export interface PortalEnv {
  portal: PortalKind;
  title: string;
  appVersion: string;
  theme: PortalThemeName;
  storagePrefix: string;
  clientTypeHeader: "admin" | "tenant" | "customer";
  xClientId: string;
  serviceClientIds?: Partial<Record<BackendService, string>>;
  urmBaseUrl: string;
  aiBaseUrl: string;
  aiPublicBaseUrl: string;
  proxyBaseUrl: string;
  ssoAuthorizeUrl?: string;
  legalBaseUrl: string;
}

export function createPortalEnv(input: PortalEnv): PortalEnv {
  const defaultServiceClientIds = Object.fromEntries(
    PORTAL_SERVICE_MODULES.map((module) => [module.service, input.xClientId])
  ) as Record<BackendService, string>;

  return {
    ...input,
    serviceClientIds: {
      ...defaultServiceClientIds,
      ...input.serviceClientIds
    }
  };
}

export interface StandardPortalEnvOptions {
  portal: PortalKind;
  env: PortalBuildEnv;
  title?: string;
  theme?: PortalThemeName;
  storagePrefix?: string;
}

export function createStandardPortalEnv(options: StandardPortalEnvOptions): PortalEnv {
  const buildEnv = options.env;
  const xClientId = buildEnv.VITE_X_CLIENT_ID || "urm";
  const serviceClientIds = Object.fromEntries(
    PORTAL_SERVICE_MODULES.map((module) => [
      module.service,
      buildEnv[module.clientIDEnvKey]?.trim()
        || ("defaultClientID" in module ? module.defaultClientID : xClientId)
    ])
  ) as Record<BackendService, string>;

  return createPortalEnv({
    portal: options.portal,
    title: options.title || defaultPortalTitle(options.portal),
    appVersion: buildEnv.VITE_APP_VERSION?.trim() || "0.0.1",
    theme: options.theme || options.portal,
    storagePrefix: options.storagePrefix || `doustack:${options.portal}`,
    clientTypeHeader: options.portal,
    xClientId,
    serviceClientIds,
    urmBaseUrl: buildEnv.VITE_URM_BASE_URL || "http://127.0.0.1:13000",
    aiBaseUrl: buildEnv.VITE_AI_BASE_URL || "http://127.0.0.1:13002",
    aiPublicBaseUrl: buildEnv.VITE_AI_PUBLIC_BASE_URL || "http://127.0.0.1:13002",
    proxyBaseUrl: buildEnv.VITE_PROXY_BASE_URL || "http://127.0.0.1:13001",
    ssoAuthorizeUrl: buildEnv.VITE_SSO_AUTHORIZE_URL,
    legalBaseUrl: buildEnv.VITE_LEGAL_BASE_URL || "http://localhost:6901/legal"
  });
}

export function enabledPortalServices(
  env: PortalEnv,
  enabledClientIDs: readonly string[]
): BackendService[] {
  return PORTAL_SERVICE_MODULES.filter((module) => {
    if ("alwaysEnabled" in module && module.alwaysEnabled) return true;
    const clientID = env.serviceClientIds?.[module.service];
    return Boolean(clientID && enabledClientIDs.includes(clientID));
  }).map((module) => module.service);
}

export function portalModuleForClientID(
  env: PortalEnv,
  clientID: string
): PortalServiceModuleDefinition | undefined {
  const normalizedClientID = clientID.trim();
  if (!normalizedClientID) return undefined;

  return PORTAL_SERVICE_MODULES.find(
    (module) => env.serviceClientIds?.[module.service] === normalizedClientID
  );
}

function defaultPortalTitle(portal: PortalKind): string {
  switch (portal) {
    case "admin":
      return "豆栈 DouStack 管理平台";
    case "tenant":
      return "豆栈 DouStack 租户平台";
    case "customer":
      return "豆栈 DouStack 用户平台";
  }
}
