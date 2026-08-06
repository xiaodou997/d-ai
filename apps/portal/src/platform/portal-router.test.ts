import { describe, expect, it, vi } from "vitest";

import type { PortalEnv } from "./env";
import { redirectPortalToLogin } from "./http";
import {
  attachPortalAuthGuard,
  resolvePortalPublicBaseUrl,
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

describe("unified Portal route authorization", () => {
  it("resolves same-origin and relative public gateway URLs", () => {
    expect(resolvePortalPublicBaseUrl("/")).toBe(window.location.origin);
    expect(resolvePortalPublicBaseUrl("/gateway")).toBe(`${window.location.origin}/gateway`);
    expect(resolvePortalPublicBaseUrl("https://dai.example.com/gateway/")).toBe("https://dai.example.com/gateway");
  });

  it("rejects platform administrators from super-admin-only routes", () => {
    const route = { ...baseRoute, matched: [{ meta: { allowedUserTypes: [1] } }] };
    expect(routeAllowedForUserType(route, 1)).toBe(true);
    expect(routeAllowedForUserType(route, 2)).toBe(false);
  });

  it("enforces capability metadata when the portal supplies a capability resolver", () => {
    const route = { ...baseRoute, matched: [{ meta: { capability: "admin.identity" } }] };
    const hasCapability = (userType: number, capability: string) =>
      userType === 1 && capability === "admin.identity";

    expect(routeAllowedForUserType(route, 1, hasCapability)).toBe(true);
    expect(routeAllowedForUserType(route, 2, hasCapability)).toBe(false);
  });

  it("keeps public routes public", async () => {
    const { guard, useAuthStore } = guardHarness();
    const result = await guard({
      ...baseRoute,
      path: "/legal/privacy",
      matched: [{ meta: { public: true } }]
    });

    expect(result).toBe(true);
    expect(useAuthStore).not.toHaveBeenCalled();
  });

  it("clears local auth state before redirecting an invalid session to login", async () => {
    vi.mocked(redirectPortalToLogin).mockClear();
    const { guard, store } = guardHarness();
    store.ensureSession = vi.fn().mockRejectedValue(new Error("account disabled"));

    await expect(
      guard({ ...baseRoute, path: "/overview", matched: [{ meta: {} }] })
    ).resolves.toBe(false);

    expect(store.clear).toHaveBeenCalledOnce();
    expect(redirectPortalToLogin).toHaveBeenCalledWith(
      expect.objectContaining({ portal: "unified" }),
      "/overview"
    );
  });
});

function guardHarness() {
  let guard!: (route: PortalRouteLike) => unknown;
  const router = {
    beforeEach(callback: (route: PortalRouteLike) => unknown) {
      guard = callback;
    }
  };
  const store: PortalAuthStoreLike = {
    accessToken: "token",
    userInfo: {},
    userType: 3,
    init: vi.fn(),
    ensureSession: vi.fn().mockResolvedValue(undefined),
    fetchUserInfo: vi.fn(),
    clear: vi.fn()
  };
  const useAuthStore = vi.fn(() => store);
  const env = {
    portal: "unified"
  } as PortalEnv;

  attachPortalAuthGuard(router, { env, useAuthStore });
  return { guard: guard as (route: PortalRouteLike) => Promise<unknown>, useAuthStore, store };
}
