import { describe, expect, it } from "vitest";

import { findFrontendQualityViolations } from "./validate-frontend-quality.mjs";

describe("Portal frontend quality contract", () => {
  it("does not allow explicit any outside the reviewed baseline", async () => {
    await expect(findFrontendQualityViolations()).resolves.toEqual([]);
  });
});
