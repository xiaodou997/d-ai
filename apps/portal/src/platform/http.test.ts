import { afterEach, describe, expect, it, vi } from "vitest";

import {
  createFetchAdapter,
  createPortalRequestContext,
  HttpProblem,
  ServiceAccessUnavailableError
} from "./http";
import type { PortalAuthLike } from "./http";
import type { PortalEnv } from "./env";

afterEach(() => vi.unstubAllGlobals());

function createEnv(overrides: Partial<PortalEnv> = {}): PortalEnv {
  return {
    portal: "unified",
    clientTypeHeader: "customer",
    xClientId: "dai-portal",
    serviceClientIds: { urm: "dai-portal", ai: "dai-portal" },
    urmBaseUrl: "http://urm",
    aiBaseUrl: "http://ai",
    aiPublicBaseUrl: "http://ai",
    appVersion: "test",
    title: "test",
    theme: "customer",
    storagePrefix: "test",
    legalBaseUrl: "http://legal",
    ...overrides
  };
}

function createAuthStore(overrides: Partial<PortalAuthLike> = {}): PortalAuthLike {
  return {
    accessToken: "urm-token",
    serviceTokens: { ai: { accessToken: "ai-token" } },
    refreshServiceAccessToken: vi.fn(),
    ensureSession: vi.fn(),
    refreshCapabilities: vi.fn(),
    hasClientAccess: vi.fn(() => true),
    clearServiceToken: vi.fn(),
    clear: vi.fn(),
    ...overrides
  };
}

describe("service access recovery", () => {
  it("does not resolve the auth store while creating a request adapter", async () => {
    const fetchMock = vi.fn(async () => new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);
    const useAuthStore = vi.fn(() => createAuthStore());
    const context = createPortalRequestContext({
      env: createEnv(),
      useAuthStore
    });

    const request = context.authenticatedRequest("ai");
    expect(useAuthStore).not.toHaveBeenCalled();

    await request({ method: "GET", path: "/api/v1/models", baseUrl: "http://ai" });
    expect(useAuthStore).toHaveBeenCalled();
    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it("does not start SSO or issue a request for an unavailable service", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const context = createPortalRequestContext({
      env: createEnv(),
      useAuthStore: () => createAuthStore({ hasClientAccess: vi.fn(() => false) })
    });

    await expect(
      context.authenticatedRequest("ai")({
        method: "GET",
        path: "/api/v1/models",
        baseUrl: "http://ai"
      })
    ).rejects.toEqual(new ServiceAccessUnavailableError("ai"));

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("invokes recovery only for service_access_denied", async () => {
    const recover = vi.fn(() => false);
    vi.stubGlobal("fetch", vi.fn(async () => problem(403, "service_access_denied")));

    const request = createFetchAdapter({ onAccessDenied: recover });
    await expect(request({ method: "GET", path: "/models", baseUrl: "http://service" })).rejects.toBeInstanceOf(HttpProblem);
    expect(recover).toHaveBeenCalledWith(403, "service_access_denied");
  });

  it("does not treat an ordinary business 403 as a portal revocation", async () => {
    const recover = vi.fn(() => false);
    vi.stubGlobal("fetch", vi.fn(async () => problem(403, "model_not_authorized")));

    const request = createFetchAdapter({ onAccessDenied: recover });
    await expect(request({ method: "GET", path: "/models", baseUrl: "http://service" })).rejects.toBeInstanceOf(HttpProblem);
    expect(recover).not.toHaveBeenCalled();
  });

  it("preserves structured problem metadata", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        new Response(
          JSON.stringify({
            status: 409,
            code: "purchase_rolling_limit_reached",
            title: "Conflict",
            meta: { retry_at: "2026-07-22T10:00:00Z" }
          }),
          {
            status: 409,
            headers: { "Content-Type": "application/problem+json" }
          }
        )
      )
    );

    const request = createFetchAdapter();
    const error = await request<HttpProblem>({
      method: "POST",
      path: "/subscription-orders",
      baseUrl: "http://service"
    }).catch((caught) => caught as HttpProblem);

    expect(error).toBeInstanceOf(HttpProblem);
    expect(error.meta).toEqual({ retry_at: "2026-07-22T10:00:00Z" });
  });
});

function problem(status: number, code: string) {
  return new Response(JSON.stringify({ status, code, title: "Forbidden" }), {
    status,
    headers: { "Content-Type": "application/problem+json" }
  });
}
