import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/api",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { aiAdminApi } from "./aiAdmin";

beforeEach(() => mocks.request.mockReset());

const binding = {
  id: "binding-1",
  model_code: "gpt-4o",
  capability_type: "chat",
  api_format: "openai_chat",
  upstream_model_name: "gpt-4o-2024-08-06",
  status: "active",
  image_stream_mode: "force_sync",
  image_edit_transport: "multipart/form-data",
  image_upstream_response_format: "url",
  image_max_output_count: 1,
  image_edit_max_output_count: 1,
  created_at: 100,
  updated_at: 200,
  $schema: "ignored"
};

const testResult = {
  ok: true,
  http_status: 200,
  latency_ms: 42,
  capability: "chat",
  api_format: "openai_chat",
  upstream_model: "gpt-4o-2024-08-06",
  reply_text: "hello",
  prompt_tokens: 3,
  output_tokens: 4,
  total_tokens: 7,
  image_edit_transport: undefined,
  image_upstream_response_format: undefined
};

describe("AI admin generated model binding facade", () => {
  it("normalizes discovered models and binding lists while passing account paths", async () => {
    mocks.request
      .mockResolvedValueOnce({ items: null, total: 0 })
      .mockResolvedValueOnce({ items: [binding], total: 1 });

    await expect(aiAdminApi.fetchAccountUpstreamModels("account/1")).resolves.toEqual({ items: [] });
    await expect(aiAdminApi.listAccountModelBindings("account/1")).resolves.toMatchObject({
      items: [{ id: "binding-1", model_code: "gpt-4o" }],
      total: 1
    });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/upstream-accounts/account%2F1/upstream-models",
      pathParams: { accountID: "account/1" }
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      pathParams: { accountID: "account/1" }
    });
  });

  it("binds account CRUD, test and import operations with typed bodies", async () => {
    mocks.request
      .mockResolvedValueOnce(binding)
      .mockResolvedValueOnce(binding)
      .mockResolvedValueOnce({ deleted: true })
      .mockResolvedValueOnce(testResult)
      .mockResolvedValueOnce({ created: null, skipped: ["gpt-4o"] });

    const writeBody = {
      model_code: "gpt-4o",
      capability_type: "chat",
      api_format: "openai_chat",
      upstream_model_name: "gpt-4o-2024-08-06",
      status: "active",
      image_stream_mode: "force_sync",
      image_edit_transport: "multipart/form-data" as const,
      image_upstream_response_format: "" as const,
      image_max_output_count: 1,
      image_edit_max_output_count: 1
    };

    await expect(aiAdminApi.createAccountModelBinding("account/1", writeBody)).resolves.toMatchObject({ id: "binding-1" });
    await expect(aiAdminApi.updateAccountModelBinding("account/1", "binding/1", writeBody)).resolves.toMatchObject({ id: "binding-1" });
    await expect(aiAdminApi.deleteAccountModelBinding("account/1", "binding/1")).resolves.toEqual({ deleted: true });
    await expect(aiAdminApi.testUpstreamAccount("account/1", {
      model_code: "gpt-4o",
      prompt: "hello",
      image_edit: false
    })).resolves.toMatchObject({ ok: true, upstream_model: "gpt-4o-2024-08-06" });
    await expect(aiAdminApi.importAccountUpstreamModels("account/1", {
      models: ["gpt-4o"],
      api_format: "openai_chat"
    })).resolves.toEqual({ created: [], skipped: ["gpt-4o"] });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      pathParams: { accountID: "account/1" },
      body: { capability_type: "chat", api_format: "openai_chat", image_upstream_response_format: "" }
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/upstream-accounts/account%2F1/model-bindings/binding%2F1",
      pathParams: { accountID: "account/1", bindingID: "binding/1" }
    });
    expect(mocks.request.mock.calls[3]?.[0]).toMatchObject({
      pathParams: { accountID: "account/1" },
      body: { model_code: "gpt-4o", prompt: "hello" }
    });
    expect(mocks.request.mock.calls[4]?.[0]).toMatchObject({
      body: { models: ["gpt-4o"], api_format: "openai_chat" }
    });
  });

  it("maps capability inference and pool model operations with nullable collections", async () => {
    mocks.request
      .mockResolvedValueOnce({ capability_type: "image", api_format: "openai_images", source: "heuristic", $schema: "ignored" })
      .mockResolvedValueOnce({
        pool_id: "pool-1",
        fixed_provider_type: "codex",
        models: null,
        observed_at: 100,
        profile_revision: "v1",
        source: "cache",
        $schema: "ignored"
      })
      .mockResolvedValueOnce({ items: null, total: 0 })
      .mockResolvedValueOnce(binding)
      .mockResolvedValueOnce(binding)
      .mockResolvedValueOnce({ deleted: true })
      .mockResolvedValueOnce({ created: null, skipped: null });

    await expect(aiAdminApi.inferModelCapability("image-1", "openai_compatible")).resolves.toEqual({
      capability_type: "image",
      api_format: "openai_images",
      source: "heuristic"
    });
    await expect(aiAdminApi.getPoolAvailableModels("pool/1")).resolves.toMatchObject({ pool_id: "pool-1", models: [], source: "cache" });
    await expect(aiAdminApi.listPoolModelBindings("pool/1")).resolves.toEqual({ items: [], total: 0 });
    await expect(aiAdminApi.createPoolModelBinding("pool/1", { model_code: "gpt-4o" })).resolves.toMatchObject({ id: "binding-1" });
    await expect(aiAdminApi.updatePoolModelBinding("pool/1", "binding/1", { model_code: "gpt-4o" })).resolves.toMatchObject({ id: "binding-1" });
    await expect(aiAdminApi.deletePoolModelBinding("pool/1", "binding/1")).resolves.toEqual({ deleted: true });
    await expect(aiAdminApi.importPoolAvailableModels("pool/1", { models: ["gpt-4o"] })).resolves.toEqual({ created: [], skipped: [] });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      query: { model_code: "image-1", endpoint_protocol: "openai_compatible" }
    });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/credential-pools/pool%2F1/available-models",
      pathParams: { poolID: "pool/1" }
    });
    expect(mocks.request.mock.calls[4]?.[0]).toMatchObject({
      pathParams: { poolID: "pool/1", bindingID: "binding/1" }
    });
  });

  it("rejects values outside generated binding and inference enums", async () => {
    expect(() => aiAdminApi.createAccountModelBinding("account-1", { capability_type: "unknown" })).toThrow(
      "Unexpected upstream capability type"
    );
    expect(() => aiAdminApi.inferModelCapability("gpt-4o", "azure")).toThrow("Unexpected endpoint protocol");
    expect(mocks.request).not.toHaveBeenCalled();

    mocks.request.mockResolvedValueOnce({ ...testResult, image_upstream_response_format: "xml" });
    await expect(aiAdminApi.testUpstreamAccount("account-1", { model_code: "gpt-4o" })).rejects.toThrow(
      "Unexpected image upstream response format"
    );
  });
});
