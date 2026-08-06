import { describe, expect, it } from "vitest";

import {
  cashTransactionText,
  creditSourceText,
  formatCents,
  formatCredits,
  maskAccount,
  normalizeAccountTab,
  withdrawalStatusTone
} from "./model";

describe("account center presentation", () => {
  it("normalizes tabs and formats both account units", () => {
    expect(normalizeAccountTab("balance")).toBe("balance");
    expect(normalizeAccountTab("unknown")).toBe("points");
    expect(formatCredits(12345.5)).toBe("12,345.5");
    expect(formatCents(12345)).toBe("123.45");
  });

  it("maps internal sources and account transaction names to user language", () => {
    expect(creditSourceText("cash_purchase")).toBe("余额购买");
    expect(cashTransactionText("topup_income")).toBe("用户充值到账");
    expect(withdrawalStatusTone("pending")).toBe("warning");
  });

  it("masks withdrawal accounts while preserving the final four digits", () => {
    expect(maskAccount("6222021234567890")).toBe("************7890");
    expect(maskAccount("1234")).toBe("1234");
  });
});
