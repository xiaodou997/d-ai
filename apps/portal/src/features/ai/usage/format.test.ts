import { describe, expect, it } from "vitest";

import { formatUSD as formatUsageUSD } from "./format";
import { formatUSD as formatPortalUsageUSD } from "@/platform/ai/utils";

describe("usage amount formatting", () => {
  it("keeps micro-USD precision for usage records", () => {
    expect(formatUsageUSD(29.428236)).toBe("$29.428236");
    expect(formatPortalUsageUSD(29.428236)).toBe("$29.428236");
  });
});
