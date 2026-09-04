import { describe, expect, it } from "vitest";

import { formatCompactNumber, formatUSD as formatUsageUSD } from "./format";
import { formatUSD as formatPortalUsageUSD } from "@/platform/ai/utils";

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
