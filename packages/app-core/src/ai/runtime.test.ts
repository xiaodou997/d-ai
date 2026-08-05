import { afterEach, describe, expect, it, vi } from "vitest";

import { createPortalRuntimeTransport } from "./runtime";

afterEach(() => vi.unstubAllGlobals());

describe("AI runtime service access recovery", () => {
  it("recognizes service access denial in an OpenAI error envelope", async () => {
    const recover = vi.fn(() => false);
    vi.stubGlobal("fetch", vi.fn(async () => openAIError("service_access_denied")));
    const transport = createTransport(recover);

    await expect(transport.request("GET", "/runtime/v1/models")).rejects.toThrow("Service access denied");
    expect(recover).toHaveBeenCalledWith(403, "service_access_denied");
  });

  it("does not recover from an ordinary AI business denial", async () => {
    const recover = vi.fn(() => false);
    vi.stubGlobal("fetch", vi.fn(async () => openAIError("model_not_authorized")));
    const transport = createTransport(recover);

    await expect(transport.request("GET", "/runtime/v1/models")).rejects.toThrow("Service access denied");
    expect(recover).not.toHaveBeenCalled();
  });
});

function createTransport(onAccessDenied: (status: number, code?: string) => boolean) {
  return createPortalRuntimeTransport({
    baseUrl: () => "http://ai.example.test",
    portalHeadersForAI: () => ({ "X-Client-Id": "ai" }),
    getAccessToken: () => "token",
    onAccessDenied
  });
}

function openAIError(code: string) {
  return new Response(JSON.stringify({ error: { message: "Service access denied", type: code, code } }), {
    status: 403,
    headers: { "Content-Type": "application/json" }
  });
}
