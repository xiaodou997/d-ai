import { describe, expect, it, vi } from "vitest";

import type { PortalEnv } from "./env";
import { redirectPortalToLogin } from "./http";
import {
  attachPortalSSOGuard,
  routeAllowedForUserType,
  type PortalAuthStoreLike,
  type PortalRouteLike
} from "./portal-router";

vi.mock("./http", () => ({
  redirectPortalToLogin: vi.fn().mockResolvedValue(true)
}));

const baseRoute: PortalRouteLike = {
  path: "/overview",
  query: {},
  hash: "",
  matched: [{ meta: {} }]
};

describe("portal route authorization", () => {
  it("rejects platform administrators from super-admin-only routes", () => {
    const route = { ...baseRoute, matched: [{ meta: { allowedUserTypes: [1] } }] };
    expect(routeAllowedForUserType(route, 1)).toBe(true);
    expect(routeAllowedForUserType(route, 2)).toBe(false);
  });

  it("keeps public registration routes public", async () => {
    const { guard, useAuthStore } = guardHarness("customer");
    const result = await guard({
      ...baseRoute,
      path: "/register/invite",
      matched: [{ meta: { public: true } }]
    });

    expect(result).toBe(true);
    expect(useAuthStore).not.toHaveBeenCalled();
  });

  it("keeps the legal center public", async () => {
    const { guard, useAuthStore } = guardHarness("admin");
    const result = await guard({
      ...baseRoute,
      path: "/legal/privacy",
      matched: [{ meta: { public: true } }]
    });

    expect(result).toBe(true);
    expect(useAuthStore).not.toHaveBeenCalled();
  });

  it.each([
    ["tenant", "/workspace", "ai", "/overview"],
    ["customer", "/gateway/routes", "proxy", "/account"]
  ] as const)("redirects unauthorized %s service URLs", async (portal, path, service, fallback) => {
    const { guard } = guardHarness(portal, false);
    await expect(guard({ ...baseRoute, path, matched: [{ meta: { service } }, { meta: {} }] })).resolves.toEqual({
      path: fallback,
      replace: true
    });
  });

  it("clears local auth state before redirecting an invalid session to SSO login", async () => {
    vi.mocked(redirectPortalToLogin).mockClear();
    const { guard, store } = guardHarness("tenant");
    store.ensureSession = vi.fn().mockRejectedValue(new Error("account disabled"));

    await expect(
      guard({ ...baseRoute, path: "/overview", matched: [{ meta: { service: "urm" } }] })
    ).resolves.toBe(false);

    expect(store.clear).toHaveBeenCalledOnce();
    expect(redirectPortalToLogin).toHaveBeenCalledWith(
      expect.objectContaining({ portal: "tenant" }),
      "/overview"
    );
  });
});

function guardHarness(portal: PortalEnv["portal"], hasClientAccess = true) {
  let guard!: (route: PortalRouteLike) => unknown;
  const router = {
    beforeEach(callback: (route: PortalRouteLike) => unknown) {
      guard = callback;
    }
  };
  const store: PortalAuthStoreLike = {
    accessToken: "token",
    userInfo: {},
    userType: portal === "admin" ? 2 : portal === "tenant" ? 3 : 4,
    init: vi.fn(),
    ensureSession: vi.fn().mockResolvedValue(undefined),
    exchangeCode: vi.fn(),
    exchangeServiceCode: vi.fn(),
    fetchUserInfo: vi.fn(),
    hasClientAccess: vi.fn(() => hasClientAccess),
    clear: vi.fn()
  };
  const useAuthStore = vi.fn(() => store);
  const env = {
    portal,
    serviceClientIds: { urm: "urm-client", ai: "ai-client", proxy: "proxy-client" }
  } as PortalEnv;

  attachPortalSSOGuard(router, { env, useAuthStore });
  return { guard: guard as (route: PortalRouteLike) => Promise<unknown>, useAuthStore, store };
}
