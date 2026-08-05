import { describe, expect, it } from "vitest";

import { generationHash, generationIsCurrent } from "./generation-state.mjs";

describe("OpenAPI generated type freshness", () => {
  it("requires generation on a fresh checkout", () => {
    const hash = generationHash([{ name: "dai", content: "openapi: 3.1.0" }]);
    expect(generationIsCurrent({ expectedHash: hash, marker: "", outputsExist: false })).toBe(false);
  });

  it("accepts matching outputs and invalidates a stale specification", () => {
    const original = generationHash([{ name: "dai", content: "openapi: 3.1.0" }]);
    const changed = generationHash([{ name: "dai", content: "openapi: 3.1.0\ninfo: changed" }]);

    expect(generationIsCurrent({ expectedHash: original, marker: original, outputsExist: true })).toBe(true);
    expect(generationIsCurrent({ expectedHash: changed, marker: original, outputsExist: true })).toBe(false);
  });
});
