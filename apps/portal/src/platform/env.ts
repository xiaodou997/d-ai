import type { PortalThemeName } from "@/shared/ui";

/** The single Portal shell; the visible surface is selected by userType. */
export type PortalKind = "unified";
export type BackendService = "urm" | "ai";
export type PortalClientType = "admin" | "tenant" | "customer";

export const PORTAL_CLIENT_TYPES = ["admin", "tenant", "customer"] as const satisfies readonly PortalClientType[];
const PORTAL_CLIENT_TYPE_STORAGE_KEY = "dai:portal:client-type";

export interface PortalServiceModuleDefinition {
  service: BackendService;
  label: string;
  clientIDEnvKey: "VITE_URM_CLIENT_ID" | "VITE_AI_CLIENT_ID";
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
    defaultClientID: "dai-portal"
  }
] as const satisfies readonly PortalServiceModuleDefinition[];

export interface PortalBuildEnv {
  [key: string]: unknown;
  VITE_APP_VERSION?: string;
  VITE_X_CLIENT_ID?: string;
  VITE_CLIENT_TYPE?: PortalClientType;
  VITE_URM_CLIENT_ID?: string;
  VITE_AI_CLIENT_ID?: string;
  VITE_URM_BASE_URL?: string;
  VITE_AI_BASE_URL?: string;
  VITE_AI_PUBLIC_BASE_URL?: string;
  VITE_SSO_AUTHORIZE_URL?: string;
  VITE_LEGAL_BASE_URL?: string;
}

export interface PortalEnv {
  portal: PortalKind;
  title: string;
  appVersion: string;
  theme: PortalThemeName;
  storagePrefix: string;
  /** The initial SSO audience; the authenticated userType controls the UI surface. */
  clientTypeHeader: PortalClientType;
  xClientId: string;
  serviceClientIds: Partial<Record<BackendService, string>>;
  urmBaseUrl: string;
  aiBaseUrl: string;
  aiPublicBaseUrl: string;
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
  portal?: PortalKind;
  env: PortalBuildEnv;
  title?: string;
  theme?: PortalThemeName;
  storagePrefix?: string;
}

export function createStandardPortalEnv(options: StandardPortalEnvOptions): PortalEnv {
  const buildEnv = options.env;
  const xClientId = buildEnv.VITE_X_CLIENT_ID?.trim() || "dai-portal";
  const serviceClientIds = Object.fromEntries(
    PORTAL_SERVICE_MODULES.map((module) => [
      module.service,
      buildEnv[module.clientIDEnvKey]?.trim()
        || ("defaultClientID" in module ? module.defaultClientID : xClientId)
    ])
  ) as Record<BackendService, string>;

  return createPortalEnv({
    portal: options.portal || "unified",
    title: options.title || "D-AI 统一平台",
    appVersion: buildEnv.VITE_APP_VERSION?.trim() || "0.0.1",
    theme: options.theme || "admin",
    storagePrefix: options.storagePrefix || "dai:portal",
    clientTypeHeader: buildEnv.VITE_CLIENT_TYPE || "customer",
    xClientId,
    serviceClientIds,
    urmBaseUrl: buildEnv.VITE_URM_BASE_URL || "",
    aiBaseUrl: buildEnv.VITE_AI_BASE_URL || "",
    aiPublicBaseUrl: buildEnv.VITE_AI_PUBLIC_BASE_URL || buildEnv.VITE_AI_BASE_URL || "",
    ssoAuthorizeUrl: buildEnv.VITE_SSO_AUTHORIZE_URL,
    legalBaseUrl: buildEnv.VITE_LEGAL_BASE_URL || "/legal"
  });
}

export function isPortalClientType(value: unknown): value is PortalClientType {
  return typeof value === "string" && PORTAL_CLIENT_TYPES.includes(value as PortalClientType);
}

/**
 * Resolve the login context for the unified app. The context is only an SSO
 * audience selector; the authenticated JWT remains the source of userType.
 */
export function resolvePortalClientType(fallback: PortalClientType): PortalClientType {
  if (typeof window === "undefined") return fallback;

  const queryValue = new URLSearchParams(window.location.search).get("client_type");
  if (isPortalClientType(queryValue)) {
    rememberPortalClientType(queryValue);
    return queryValue;
  }

  try {
    const stored = localStorage.getItem(PORTAL_CLIENT_TYPE_STORAGE_KEY);
    if (isPortalClientType(stored)) return stored;
  } catch {
    // Storage can be disabled by the browser; the build-time fallback remains valid.
  }
  return fallback;
}

export function rememberPortalClientType(clientType: PortalClientType): void {
  if (typeof window === "undefined") return;
  try {
    localStorage.setItem(PORTAL_CLIENT_TYPE_STORAGE_KEY, clientType);
  } catch {
    // The current page can still continue with the in-memory env value.
  }
}

export function enabledPortalServices(
  env: PortalEnv,
  enabledClientIDs: readonly string[]
): BackendService[] {
  return PORTAL_SERVICE_MODULES.filter((module) => {
    if ("alwaysEnabled" in module && module.alwaysEnabled) return true;
    const clientID = env.serviceClientIds[module.service];
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
    (module) => env.serviceClientIds[module.service] === normalizedClientID
  );
}
