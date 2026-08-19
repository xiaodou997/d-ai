import { describe, expect, it } from "vitest";

import {
  balanceSourceText,
  balanceTransactionText,
  formatMicroUSD,
  formatUSD,
  normalizeAccountTab,
} from "./model";

describe("account center presentation", () => {
  it("normalizes tabs and formats USD amounts", () => {
    expect(normalizeAccountTab("recharges")).toBe("recharges");
    expect(normalizeAccountTab("unknown")).toBe("ledger");
    expect(formatUSD(12345.5)).toBe("$12,345.50");
    expect(formatMicroUSD(123_456_789)).toBe("$123.45");
  });

  it("maps balance sources and transaction names", () => {
    expect(balanceSourceText("online_tenant_topup")).toBe("在线充值");
    expect(balanceTransactionText("topup_income")).toBe("用户充值收入");
    expect(balanceTransactionText("refund_reversal")).toBe("退款收入冲正");
  });
});
