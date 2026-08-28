import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/api",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));
vi.mock("@/platform", () => ({ redirectPortalToLogin: vi.fn() }));
vi.mock("@/platform/ai/runtime", () => ({
  createPortalRuntimeTransport: vi.fn(() => ({ request: vi.fn(), formRequest: vi.fn(), streamChatMessage: vi.fn() })),
  portalStatusOptions: []
}));
vi.mock("@/platform/ai/usage", () => ({ formatUSD: vi.fn() }));
vi.mock("@/platform/ai/images", () => ({}));
vi.mock("@/platform/ai/tasks", () => ({}));
vi.mock("@/env", () => ({ portalEnv: { apiBaseUrl: "/api" } }));
vi.mock("@/stores/auth", () => ({ useAuthStore: vi.fn(() => ({ accessToken: "token" })) }));

import { aiTenantApi, runtimeChatApi } from "./aiTenant";

beforeEach(() => mocks.request.mockReset());

const session = {
  id: "session-1",
  title: "Demo",
  model_code: "gpt-4o",
  group_id: "group-1",
  group_name: "Default",
  effective_user_multiplier: 1,
  billing_group_label: "default",
  provider_api_format: "openai_chat",
  selected_route_id: "route-1",
  status: "active",
  created_at: 100,
  updated_at: 200,
  $schema: "ignored"
};

describe("AI tenant workspace generated operation facade", () => {
  it("normalizes nullable model/session/image job collections", async () => {
    mocks.request
      .mockResolvedValueOnce({ items: [{
        group_id: "group-1",
        group_name: "Default",
        effective_user_multiplier: 1,
        billing_group_label: "default",
        model_code: "gpt-4o",
        capability_type: "chat",
        default_api_format: "openai_chat",
        available_api_formats: null,
        supports_stream: true,
        status: "active"
      }], total: 1 })
      .mockResolvedValueOnce({ items: null, total: 0 })
      .mockResolvedValueOnce({ items: [{
        id: "job-1",
        operation: "generation",
        model_code: "gpt-image-1",
        prompt: "a cat",
        status: "completed",
        storage_policy: "temporary",
        raw_image_retained: false,
        requested_output_count: 1,
        caller_charge_usd: 0.1,
        image_count: 1,
        inline_count: 0,
        url_count: 1,
        assets: null,
        revised_prompts: null,
        created_at: 100
      }], total: 1 });

    await expect(aiTenantApi.listWorkspaceChatModels()).resolves.toMatchObject({
      items: [{ model_code: "gpt-4o", available_api_formats: [] }]
    });
    await expect(aiTenantApi.listWorkspaceChatSessions()).resolves.toEqual({ items: [], total: 0 });
    await expect(aiTenantApi.listWorkspaceImageJobs()).resolves.toMatchObject({
      items: [{ id: "job-1", operation: "generation", assets: undefined, revised_prompts: undefined }],
      total: 1
    });
    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({
      query: { limit: 50 },
      path: "/api/v1/tenants/me/workspace/image/jobs"
    });
  });

  it("maps chat session detail and forwards encoded session paths", async () => {
    mocks.request
      .mockResolvedValueOnce(session)
      .mockResolvedValueOnce({ session, messages: null, $schema: "ignored" })
      .mockResolvedValueOnce({ deleted: true });

    await expect(runtimeChatApi.createSession({ model_code: "gpt-4o", group_id: "group-1" })).resolves.toMatchObject({
      id: "session-1"
    });
    await expect(runtimeChatApi.getSession("session/1")).resolves.toEqual({
      session: expect.objectContaining({ id: "session-1" }),
      messages: []
    });
    await expect(runtimeChatApi.deleteSession("session/1")).resolves.toEqual({ deleted: true });

    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/tenants/me/workspace/chat/sessions/session%2F1",
      pathParams: { sessionID: "session/1" }
    });
  });

  it("rejects an unknown image operation at the domain boundary", async () => {
    mocks.request.mockResolvedValueOnce({
      items: [{
        id: "job-1",
        operation: "unknown",
        model_code: "gpt-image-1",
        prompt: "a cat",
        status: "completed",
        storage_policy: "temporary",
        raw_image_retained: false,
        requested_output_count: 1,
        caller_charge_usd: 0,
        image_count: 0,
        inline_count: 0,
        url_count: 0,
        created_at: 100
      }],
      total: 1
    });
    await expect(aiTenantApi.listWorkspaceImageJobs({ limit: 10 })).rejects.toThrow("Unexpected workspace image operation");
  });
});
