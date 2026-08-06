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
      return redirectToLogin(to.path);
    }

    try {
      await authStore.ensureSession();
    } catch {
      authStore.clear();
      return redirectToLogin(to.path);
    }

    if (!routeAllowedForUserType(to, authStore.userType)) {
      return { path: defaultPortalPath(options.env.portal), replace: true };
    }

    return true;
  });
}

function defaultPortalPath(_portal: PortalEnv["portal"]): string {
  return "/overview";
}
