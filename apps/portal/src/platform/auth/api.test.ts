import { afterEach, describe, expect, it, vi } from "vitest";

import type { RequestAdapter } from "@/api";
import { createPortalAuthApi } from "./api";

afterEach(() => vi.unstubAllGlobals());

describe("unified Portal password login", () => {
  it("posts only the entered credentials", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
      new Response(
        JSON.stringify({
          access_token: "access-token",
          refresh_token: "refresh-token",
          token_type: "Bearer",
          expires_in: 3600
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
    const body = init?.body as URLSearchParams;
    expect(url).toBe("/api/oauth2/token");
    expect(headers.get("X-Client-Type")).toBeNull();
    expect(headers.get("X-Client-Id")).toBeNull();
    expect(body.get("grant_type")).toBe("password");
    expect(body.get("username")).toBe("alice");
    expect(body.get("password")).toBe("secret");
  });
});
