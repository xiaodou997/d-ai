import { describe, expect, it } from "vitest";

import { findSourceStyleViolations } from "./validate-source-styles.mjs";

describe("Portal source style contract", () => {
  it("keeps business source colors on the design-token contract", async () => {
    await expect(findSourceStyleViolations()).resolves.toEqual([]);
  });
});
