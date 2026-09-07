import { describe, expect, it } from "vitest";

import { formatCompactNumber, formatUSD as formatUsageUSD } from "./format";
import {
  formatCompactToken,
  formatMaskedApiKey,
  formatUSD2,
  formatUSD as formatPortalUsageUSD
} from "@/platform/ai/usage";

describe("usage amount formatting", () => {
  it("keeps micro-USD precision for usage records", () => {
    expect(formatUsageUSD(29.428236)).toBe("$29.428236");
    expect(formatPortalUsageUSD(29.428236)).toBe("$29.428236");
  });
});

describe("compact analytics numbers", () => {
  it("uses K/M/B units and carries rounding into the next unit", () => {
    expect(formatCompactNumber(12_954)).toBe("12.95K");
    expect(formatCompactNumber(2_500_000)).toBe("2.5M");
    expect(formatCompactNumber(1000)).toBe("1K");
    expect(formatCompactNumber(999.999)).toBe("1K");
  });
});

describe("compact token formatting", () => {
  it("uses K/M/B units with two decimal places", () => {
    expect(formatCompactToken(999)).toBe("999");
    expect(formatCompactToken(1_000)).toBe("1.00K");
    expect(formatCompactToken(12_500)).toBe("12.50K");
    expect(formatCompactToken(1_250_000)).toBe("1.25M");
    expect(formatCompactToken(1_250_000_000)).toBe("1.25B");
  });
});

describe("two-decimal USD formatting", () => {
  it("rounds values beyond cents", () => {
    expect(formatUSD2(1.236)).toBe("$1.24");
  });
});

describe("masked API key formatting", () => {
  it("uses the safe prefix and preserves the four-character suffix", () => {
    expect(formatMaskedApiKey("-DFk")).toBe("sk-xxx-DFk");
  });
});
