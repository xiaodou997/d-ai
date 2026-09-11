import { describe, expect, it } from "vitest";
import { usageTagMeta } from "./meta";

describe("usage metering labels", () => {
  it("distinguishes missing reports from actual zero usage and historical estimates", () => {
    expect(usageTagMeta("tokenUsageSource", "missing")?.label).toBe("缺失用量未计费");
    expect(usageTagMeta("tokenUsageSource", "upstream")?.label).toBe("上游报告");
    expect(usageTagMeta("tokenUsageSource", "estimated")?.label).toBe("估算");
    expect(usageTagMeta("tokenUsageSource", "mixed")?.label).toBe("部分估算");
  });
});
