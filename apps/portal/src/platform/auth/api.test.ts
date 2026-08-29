import { afterEach, describe, expect, it, vi } from "vitest";

import { createPortalAuthApi } from "./api";
import { createFetchAdapter } from "../http";

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
      request: createFetchAdapter(),
      baseUrl: "/"
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
      request: createFetchAdapter(),
      baseUrl: "/"
    });

    await api.refreshToken();

    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(url).toBe("/api/auth/refresh");
    expect(init?.body).toBeUndefined();
    expect(init?.credentials).toBe("include");
  });

  it("does not recursively recover a rejected refresh request", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
      new Response(JSON.stringify({ status: 401, title: "Unauthorized" }), {
        status: 401,
        headers: { "Content-Type": "application/json" }
      })
    );
    const onUnauthorized = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const api = createPortalAuthApi({
      request: createFetchAdapter({ onUnauthorized }),
      refreshRequest: createFetchAdapter(),
      baseUrl: "/"
    });

    await expect(api.refreshToken()).rejects.toThrow("HTTP 401");
    expect(fetchMock).toHaveBeenCalledOnce();
    expect(onUnauthorized).not.toHaveBeenCalled();
  });
});
