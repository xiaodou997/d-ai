import type { BackendService, PortalEnv } from "./env";
import { redirectPortalToLogin } from "./http";
import {
  clearSSOAttempts,
  consumePKCEVerifier,
  currentRedirectUri,
} from "./sso";

export interface PortalAuthStoreLike {
  accessToken: string;
  userInfo: unknown;
  userType: number;
  init: () => void;
  ensureSession: () => Promise<unknown>;
  exchangeCode: (code: string, redirectUri: string, codeVerifier: string) => Promise<unknown>;
  exchangeServiceCode: (
    service: string,
    code: string,
    redirectUri: string,
    codeVerifier: string
  ) => Promise<unknown>;
  fetchUserInfo: () => Promise<unknown>;
  hasClientAccess: (clientID: string) => boolean;
  clear: () => void;
}

export interface PortalSSOGuardOptions {
  env: PortalEnv;
  useAuthStore: () => PortalAuthStoreLike;
}

export interface PortalRouteLike {
  path: string;
  query: Record<string, unknown>;
  hash: string;
  matched: Array<{ meta: Record<string, unknown> }>;
}

export function routeAllowedForUserType(route: PortalRouteLike, userType: number): boolean {
  return route.matched.every((record) => {
    const allowed = record.meta.allowedUserTypes;
    return !Array.isArray(allowed) || allowed.includes(userType);
  });
}

export interface PortalRouterLike {
  beforeEach: (guard: (to: PortalRouteLike) => any) => unknown;
}

export function resolvePortalPublicBaseUrl(baseUrl?: string): string {
  return (baseUrl || window.location.origin).replace(/\/$/, "");
}

export function attachPortalSSOGuard(router: PortalRouterLike, options: PortalSSOGuardOptions) {
  async function redirectToSSO(path: string): Promise<false> {
    await redirectPortalToLogin(options.env, path);
    return false;
  }

  router.beforeEach(async (to) => {
    if (to.matched.some((record) => record.meta.public)) {
      return true;
    }

    const authStore = options.useAuthStore();
    authStore.init();
    const code = typeof to.query.code === "string" ? to.query.code : "";

    if (code) {
      const service = typeof to.query.service === "string" ? to.query.service : "urm";
      try {
        if (service === "ai") {
          await authStore.exchangeServiceCode(
            service,
            code,
            currentRedirectUri(`${to.path}?service=${service}`),
            consumePKCEVerifier(service)
          );
        } else {
          await authStore.exchangeCode(code, currentRedirectUri(to.path), consumePKCEVerifier("urm"));
        }
        return { path: to.path, query: {}, hash: to.hash, replace: true };
      } catch {
        authStore.clear();
        return redirectToSSO(to.path);
      }
    }

    if (!authStore.accessToken) {
      return redirectToSSO(to.path);
    }

    try {
      await authStore.ensureSession();
    } catch {
      authStore.clear();
      return redirectToSSO(to.path);
    }

    if (!routeAllowedForUserType(to, authStore.userType)) {
      return { path: defaultPortalPath(options.env.portal), replace: true };
    }

    clearSSOAttempts();
    const requiredService = serviceFromRoute(to);
    if (requiredService && requiredService !== "urm") {
      const clientID = options.env.serviceClientIds?.[requiredService];
      if (!clientID || !authStore.hasClientAccess(clientID)) {
        return { path: defaultPortalPath(options.env.portal), replace: true };
      }
    }
    return true;
  });
}

function serviceFromRoute(route: PortalRouteLike): BackendService | null {
  for (const record of [...route.matched].reverse()) {
    const service = record.meta.service;
    if (service === "urm" || service === "ai") {
      return service;
    }
  }
  return null;
}

function defaultPortalPath(_portal: PortalEnv["portal"]): string {
  return "/overview";
}
