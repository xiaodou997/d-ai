import { describe, expect, it } from "vitest";

import { formatMultiplier } from "./presentation";

describe("user pricing presentation", () => {
  it("keeps multiplier precision legible without insignificant zeroes", () => {
    expect(formatMultiplier(1)).toBe("×1");
    expect(formatMultiplier(1.457)).toBe("×1.457");
    expect(formatMultiplier(1.23456)).toBe("×1.2346");
  });
});
