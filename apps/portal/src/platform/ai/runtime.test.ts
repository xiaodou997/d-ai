import { afterEach, describe, expect, it, vi } from "vitest";

import { createPortalRuntimeTransport } from "./runtime";

afterEach(() => vi.unstubAllGlobals());

describe("AI runtime transport", () => {
  it("surfaces business errors without a service-access recovery layer", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ error: { message: "Model not authorized" } }), { status: 403 })));
    const transport = createPortalRuntimeTransport({
      baseUrl: () => "http://dai.test",
      getAccessToken: () => "portal-token"
    });
    await expect(transport.request("GET", "/runtime/v1/models")).rejects.toThrow("Model not authorized");
  });
});
