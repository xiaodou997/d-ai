import { describe, expect, it } from "vitest";

import {
  formatDisplayMicroUSD,
  formatDisplayUSD,
  truncateUSDForDisplay
} from "./currency";

describe("display currency formatting", () => {
  it("truncates USD to two decimals without rounding", () => {
    expect(formatDisplayUSD(29.428236)).toBe("$29.42");
    expect(formatDisplayUSD(29.999999)).toBe("$29.99");
    expect(formatDisplayUSD(-29.428236)).toBe("$-29.42");
    expect(truncateUSDForDisplay(12_345.678901)).toBe(12_345.67);
  });

  it("truncates exact micro-USD values to cents", () => {
    expect(formatDisplayMicroUSD(29_428_236)).toBe("$29.42");
    expect(formatDisplayMicroUSD(29_999_999)).toBe("$29.99");
    expect(formatDisplayMicroUSD(-1)).toBe("$0.00");
  });

  it("formats empty and invalid values as zero", () => {
    expect(formatDisplayUSD(null)).toBe("$0.00");
    expect(formatDisplayUSD("invalid")).toBe("$0.00");
    expect(formatDisplayMicroUSD(undefined)).toBe("$0.00");
  });
});
