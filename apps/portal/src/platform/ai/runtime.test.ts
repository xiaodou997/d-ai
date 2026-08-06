import { afterEach, describe, expect, it, vi } from "vitest";

import { createPortalRuntimeTransport } from "./runtime";

afterEach(() => vi.unstubAllGlobals());

describe("AI runtime transport", () => {
  it("keeps JSON runtime requests on the current origin when the API base is root", async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ code: 0, data: { items: [] } }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const transport = createPortalRuntimeTransport({
      baseUrl: () => "/",
      getAccessToken: () => "portal-token"
    });

    await transport.request("GET", "/runtime/v1/tasks", undefined, { limit: 20 });

    expect(fetchMock).toHaveBeenCalledWith(
      "/runtime/v1/tasks?limit=20",
      expect.objectContaining({ method: "GET" })
    );
  });

  it("keeps form runtime requests on the current origin when the API base is root", async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ code: 0, data: { id: "task-1" } }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const transport = createPortalRuntimeTransport({
      baseUrl: () => "/",
      getAccessToken: () => "portal-token"
    });

    await transport.formRequest("POST", "/runtime/v1/images/tasks", new FormData());

    expect(fetchMock).toHaveBeenCalledWith(
      "/runtime/v1/images/tasks",
      expect.objectContaining({ method: "POST" })
    );
  });

  it("keeps streaming runtime requests on the current origin when the API base is root", async () => {
    const fetchMock = vi.fn(async () => new Response("data: [DONE]\n\n", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const transport = createPortalRuntimeTransport({
      baseUrl: () => "/",
      getAccessToken: () => "portal-token"
    });

    await transport.streamChatMessage({
      sessionId: "session-1",
      messages: [],
      onDelta: vi.fn()
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/runtime/v1/chat/sessions/session-1/messages:stream",
      expect.objectContaining({ method: "POST" })
    );
  });

  it("surfaces business errors without a service-access recovery layer", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ error: { message: "Model not authorized" } }), { status: 403 })));
    const transport = createPortalRuntimeTransport({
      baseUrl: () => "http://dai.test",
      getAccessToken: () => "portal-token"
    });
    await expect(transport.request("GET", "/runtime/v1/models")).rejects.toThrow("Model not authorized");
  });
});
