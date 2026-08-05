import { describe, expect, it } from "vitest";

import { normalizeRuntimeConfig } from "./contract";

describe("normalizeRuntimeConfig", () => {
  it("migrates legacy image sizes into tiers and aspect ratios", () => {
    const config = normalizeRuntimeConfig("image_generation", {
      image: { resolution: "1536x1024" }
    });

    expect(config.image).toMatchObject({ resolution: "2k", aspect_ratio: "3:2" });
  });

  it("preserves the configured semantic image tier", () => {
    const config = normalizeRuntimeConfig("image_edit", {
      image: { resolution: "1k", aspect_ratio: "32:18" }
    });

    expect(config.image).toMatchObject({ resolution: "1k", aspect_ratio: "16:9" });
  });

  it("falls back to auto for invalid legacy image sizes", () => {
    const config = normalizeRuntimeConfig("image_generation", {
      image: { resolution: "9x9" }
    });

    expect(config.image).toMatchObject({ resolution: "auto", aspect_ratio: "1:1" });
  });
});
