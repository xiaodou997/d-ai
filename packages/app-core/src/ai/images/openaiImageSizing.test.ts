import { describe, expect, it } from "vitest";

import { resolveOpenAIImageSize } from "./openaiImageSizing";

describe("resolveOpenAIImageSize", () => {
  it("preserves auto and resolves fixed tiers on the OpenAI Image 2 grid", () => {
    expect(resolveOpenAIImageSize("auto", "16:9")).toBe("auto");
    expect(resolveOpenAIImageSize("1k", "1:1")).toBe("1024x1024");
    expect(resolveOpenAIImageSize("1k", "9:16")).toBe("768x1360");
    expect(resolveOpenAIImageSize("2k", "16:9")).toBe("2720x1536");
  });
});
