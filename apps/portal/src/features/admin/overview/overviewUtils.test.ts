import { describe, expect, it } from "vitest";
import { cacheRate, formatNumber, formatUSDStat, rateText } from "./overviewUtils";

describe("admin overview metric formatting", () => {
  it("uses compact K/M/B notation for large counters", () => {
    expect(formatNumber(999)).toBe("999");
    expect(formatNumber(1_000)).toBe("1.00K");
    expect(formatNumber(1_250_000)).toBe("1.25M");
    expect(formatNumber(1_250_000_000)).toBe("1.25B");
  });

  it("keeps aggregate amounts at two decimal places", () => {
    expect(formatUSDStat(12.345)).toBe("$12.35");
    expect(formatUSDStat(0)).toBe("$0.00");
  });

  it("returns an empty rate when there is no denominator", () => {
    expect(rateText(1, 0)).toBe("—");
    expect(cacheRate({ total_prompt_tokens: 0, cache_read_tokens: 0 })).toBe("—");
    expect(cacheRate({ total_prompt_tokens: 800, cache_read_tokens: 200 })).toBe("20.0%");
  });
});
