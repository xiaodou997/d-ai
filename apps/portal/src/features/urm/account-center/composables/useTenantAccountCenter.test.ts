import { defineComponent, h } from "vue";
import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { AccountCenterApi } from "../api";
import { useTenantAccountCenter } from "./useTenantAccountCenter";

vi.mock("element-plus", () => ({
  ElMessage: { success: vi.fn(), error: vi.fn() }
}));
vi.mock("../api", () => ({ accountCenterApi: {} }));

describe("useTenantAccountCenter", () => {
  beforeEach(() => vi.clearAllMocks());

  it("loads both balances and selects the cheapest purchase method", async () => {
    const api = fakeApi();
    let state: ReturnType<typeof useTenantAccountCenter> | undefined;
    const wrapper = mount(defineComponent({
      setup() {
        state = useTenantAccountCenter({ api });
        return () => h("div");
      }
    }));

    await vi.waitFor(() => expect(state?.points.value.availableCredits).toBe(1200));
    expect(state?.points.value.availableCredits).toBe(1200);
    expect(state?.cash.value.available).toBe(5000);
    state?.openPurchase();
    expect(state?.purchaseMethod.value).toBe("balance");
    wrapper.unmount();
  });

  it("rejects balance purchases above the available amount before calling the adapter", async () => {
    const api = fakeApi();
    let state: ReturnType<typeof useTenantAccountCenter> | undefined;
    const wrapper = mount(defineComponent({
      setup() {
        state = useTenantAccountCenter({ api });
        return () => h("div");
      }
    }));
    await vi.waitFor(() => expect(api.getCash).toHaveBeenCalled());

    await expect(state?.buyWithBalance(100)).rejects.toThrow("不超过可用余额");
    expect(api.buyCredits).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});

function fakeApi(): AccountCenterApi {
  return {
    getPoints: vi.fn().mockResolvedValue({
      totalCredits: 1500,
      usedCredits: 300,
      remainingCredits: 1200,
      frozenCredits: 0,
      availableCredits: 1200,
      permanentCredits: 1200,
      timedCredits: 0,
      packages: []
    }),
    getCash: vi.fn().mockResolvedValue({ balance: 5000, frozen: 0, available: 5000, creditsPerCny: 100, withdrawFeeBp: 160 }),
    listPointRecords: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, size: 20 }),
    listPendingOrders: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, size: 20 }),
    listCashLedger: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, size: 20 }),
    listWithdrawals: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, size: 20 }),
    getTopupConfig: vi.fn().mockResolvedValue({ enabled: true, exchangeRate: 100, feeRateBp: 160, min: 1000, max: 1000000, packages: [] }),
    createTopupOrder: vi.fn(),
    getTopupOrder: vi.fn(),
    buyCredits: vi.fn().mockResolvedValue({ creditOrderId: "order-1", credits: 100 }),
    applyWithdrawal: vi.fn(),
    cancelWithdrawal: vi.fn()
  };
}
