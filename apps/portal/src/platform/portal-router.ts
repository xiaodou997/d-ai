import type { PortalEnv } from "./env";
import { redirectPortalToLogin } from "./http";

export interface PortalAuthStoreLike {
  accessToken: string;
  userInfo: unknown;
  userType: number;
  init: () => void;
  ensureSession: () => Promise<unknown>;
  fetchUserInfo: () => Promise<unknown>;
  clear: () => void;
}

export interface PortalAuthGuardOptions {
  env: PortalEnv;
  useAuthStore: () => PortalAuthStoreLike;
  defaultPathForUserType?: (userType: number) => string;
  hasCapability?: (userType: number, capability: string) => boolean;
}

export interface PortalRouteLike {
  path: string;
  fullPath: string;
  query: Record<string, unknown>;
  hash: string;
  matched: Array<{ meta: Record<string, unknown> }>;
}

export function routeAllowedForUserType(
  route: PortalRouteLike,
  userType: number,
  hasCapability?: (userType: number, capability: string) => boolean
): boolean {
  return route.matched.every((record) => {
    const capability = record.meta.capability;
    if (typeof capability === "string" && hasCapability && !hasCapability(userType, capability)) {
      return false;
    }
    const allowed = record.meta.allowedUserTypes;
    return !Array.isArray(allowed) || allowed.includes(userType);
  });
}

export interface PortalRouterLike {
  beforeEach: (guard: (to: PortalRouteLike) => any) => unknown;
}

export function resolvePortalPublicBaseUrl(baseUrl?: string): string {
  const value = baseUrl?.trim();
  if (!value || value === "/") return window.location.origin;
  if (value.startsWith("/")) {
    return new URL(value, window.location.origin).toString().replace(/\/$/, "");
  }
  return value.replace(/\/$/, "");
}

export function attachPortalAuthGuard(router: PortalRouterLike, options: PortalAuthGuardOptions) {
  async function redirectToLogin(path: string): Promise<false> {
    await redirectPortalToLogin(options.env, path);
    return false;
  }

  router.beforeEach(async (to) => {
    if (to.matched.some((record) => record.meta.public)) {
      return true;
    }

    const authStore = options.useAuthStore();
    authStore.init();
    if (!authStore.accessToken) {
      return redirectToLogin(to.fullPath);
    }

    try {
      await authStore.ensureSession();
    } catch {
      authStore.clear();
      return redirectToLogin(to.fullPath);
    }

    if (!routeAllowedForUserType(to, authStore.userType, options.hasCapability)) {
      return {
        path: options.defaultPathForUserType?.(authStore.userType) ?? defaultPortalPath(options.env.portal),
        replace: true
      };
    }

    return true;
  });
}

function defaultPortalPath(_portal: PortalEnv["portal"]): string {
  return "/overview";
}
