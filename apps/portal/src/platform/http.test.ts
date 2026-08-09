import { afterEach, describe, expect, it, vi } from "vitest";

import { createFetchAdapter, createPortalRequestContext, HttpProblem } from "./http";
import type { PortalAuthLike } from "./http";
import type { PortalEnv } from "./env";

afterEach(() => vi.unstubAllGlobals());

const env: PortalEnv = {
  portal: "unified",
  apiBaseUrl: "http://dai.test",
  appVersion: "test",
  title: "test",
  theme: "customer",
  storagePrefix: "test",
};

function authStore(): PortalAuthLike {
  return {
    accessToken: "portal-token",
    refreshAccessToken: vi.fn(),
    ensureSession: vi.fn(),
    clear: vi.fn()
  };
}

describe("unified Portal HTTP", () => {
  it("uses the same base URL and token for every functional area", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);
    const context = createPortalRequestContext({ env, useAuthStore: authStore });

    expect(context.apiBaseUrl).toBe("http://dai.test");
    await context.authenticatedRequest()({ method: "GET", path: "/api/v1/models", baseUrl: env.apiBaseUrl });

    const headers = fetchMock.mock.calls[0]?.[1]?.headers as Headers;
    expect(headers.get("Authorization")).toBe("Bearer portal-token");
    expect(context.apiHeaders).toEqual({});
  });

  it("preserves structured problem metadata", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ status: 409, code: "conflict", title: "Conflict", meta: { retry_at: "later" } }), { status: 409, headers: { "Content-Type": "application/problem+json" } })));
    const error = await createFetchAdapter()<HttpProblem>({ method: "POST", path: "/orders", baseUrl: env.apiBaseUrl }).catch((caught) => caught as HttpProblem);
    expect(error).toBeInstanceOf(HttpProblem);
    expect(error.meta).toEqual({ retry_at: "later" });
  });
});
