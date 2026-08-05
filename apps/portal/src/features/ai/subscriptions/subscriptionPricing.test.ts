import { describe, expect, it } from "vitest";

import {
  estimateSubscriptionPaygValue
} from "./subscriptionPricing";

const groups = [{
  groupId: "g1",
  paygMultiplier: 0.15,
  quotaDebitMultiplier: 0.13
}];

describe("subscription pricing", () => {
  it("estimates the minimum pay-as-you-go retail value", () => {
    const value = estimateSubscriptionPaygValue(1_000_000, [
      { groupId: "standard", paygMultiplier: 0.15, quotaDebitMultiplier: 0.15 },
      { groupId: "discounted", paygMultiplier: 0.2, quotaDebitMultiplier: 0.25 }
    ]);
    expect(value).toBe(800_000);
  });

  it("returns null for unavailable retail inputs", () => {
    expect(estimateSubscriptionPaygValue(0, groups)).toBeNull();
    expect(estimateSubscriptionPaygValue(1_000_000, [])).toBeNull();
    expect(estimateSubscriptionPaygValue(1_000_000, [{ ...groups[0]!, quotaDebitMultiplier: 0 }])).toBeNull();
  });
});
