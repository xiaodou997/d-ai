import { afterEach, describe, expect, it, vi } from "vitest";

import type { RequestAdapter } from "@/api";
import { createPortalAuthApi } from "./api";

afterEach(() => vi.unstubAllGlobals());

describe("unified Portal password login", () => {
  it("posts only the entered credentials", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
      new Response(
        JSON.stringify({
          accessToken: "access-token",
          expiresIn: 3600,
          refreshExpiresIn: 604800
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetchMock);
    const api = createPortalAuthApi({
      request: vi.fn() as unknown as RequestAdapter,
      baseUrl: ""
    });

    await api.login(" alice ", "secret");

    const [url, init] = fetchMock.mock.calls[0] ?? [];
    const headers = new Headers(init?.headers);
    const body = JSON.parse(String(init?.body));
    expect(url).toBe("/api/auth/login");
    expect(headers.get("Content-Type")).toBe("application/json");
    expect(headers.get("X-Client-Type")).toBeNull();
    expect(headers.get("X-Client-Id")).toBeNull();
    expect(body).toEqual({ username: "alice", password: "secret" });
  });

  it("uses the direct refresh endpoint without an OAuth grant type", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
      new Response(JSON.stringify({ accessToken: "next", expiresIn: 3600, refreshExpiresIn: 604700 }), {
        status: 200,
        headers: { "Content-Type": "application/json" }
      })
    );
    vi.stubGlobal("fetch", fetchMock);
    const api = createPortalAuthApi({
      request: vi.fn() as unknown as RequestAdapter,
      baseUrl: ""
    });

    await api.refreshToken();

    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(url).toBe("/api/auth/refresh");
    expect(init?.body).toBeUndefined();
    expect(init?.credentials).toBe("include");
  });
});
