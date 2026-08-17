import { describe, expect, it } from "vitest";

import { isOnlineRecharge, rechargeMethodText } from "./recharge";

describe("recharge method presentation", () => {
  it.each([
    "online_user_topup",
    "online_tenant_topup",
    "user_topup_income"
  ])("labels %s as online", (orderType) => {
    expect(isOnlineRecharge(orderType)).toBe(true);
    expect(rechargeMethodText(orderType)).toBe("在线充值");
  });

  it.each(["platform_to_tenant", "tenant_to_user"])("labels %s as manual", (orderType) => {
    expect(isOnlineRecharge(orderType)).toBe(false);
    expect(rechargeMethodText(orderType)).toBe("手动充值");
  });
});
