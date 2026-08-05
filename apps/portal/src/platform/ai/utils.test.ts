import { describe, expect, it } from "vitest";

import { formatCredits, formatMultiplier } from "./utils";

describe("formatCredits", () => {
  it("preserves four-decimal microcredit precision", () => {
    expect(formatCredits(0.0802)).toBe("0.0802");
    expect(formatCredits(0.0962)).toBe("0.0962");
    expect(formatCredits(0.0121)).toBe("0.0121");
    expect(formatCredits(0.0152)).toBe("0.0152");
  });
});

describe("formatMultiplier", () => {
  it("removes insignificant trailing decimal zeroes", () => {
    expect(formatMultiplier(1)).toBe("1");
    expect(formatMultiplier(1.457)).toBe("1.457");
    expect(formatMultiplier(1.45)).toBe("1.45");
  });

  it("keeps at most four meaningful decimal places", () => {
    expect(formatMultiplier(1.23456)).toBe("1.2346");
    expect(formatMultiplier(null)).toBe("-");
  });
});
