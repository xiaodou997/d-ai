import { beforeEach, describe, expect, it, vi } from "vitest";

import { createUpstreamModelBindingBatchApi } from "./api";

const adapter = vi.fn();

beforeEach(() => adapter.mockReset());

describe("upstream model binding batch operation client", () => {
  it("forwards generated bodies and encoded account/pool paths", async () => {
    adapter
      .mockResolvedValueOnce({ deleted: 2, $schema: "ignored" })
      .mockResolvedValueOnce({ deleted: 1, $schema: "ignored" });
    const api = createUpstreamModelBindingBatchApi(adapter);

    await expect(api.deleteAccountBindings("account/1", ["binding-1", "binding-2"])).resolves.toEqual({
      deleted: 2,
      $schema: "ignored"
    });
    await expect(api.deletePoolBindings("pool/1", ["binding-3"])).resolves.toEqual({
      deleted: 1,
      $schema: "ignored"
    });

    expect(adapter.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/upstream-accounts/account%2F1/model-bindings/batch-delete",
      pathParams: { accountID: "account/1" },
      body: { binding_ids: ["binding-1", "binding-2"] }
    });
    expect(adapter.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/credential-pools/pool%2F1/model-bindings/batch-delete",
      pathParams: { poolID: "pool/1" },
      body: { binding_ids: ["binding-3"] }
    });
  });
});
