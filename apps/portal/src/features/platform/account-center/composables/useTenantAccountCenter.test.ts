import { defineComponent, h } from "vue";
import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { AccountCenterApi } from "../api";
import { useTenantAccountCenter } from "./useTenantAccountCenter";

vi.mock("element-plus", () => ({ ElMessage: { success: vi.fn(), error: vi.fn() } }));
vi.mock("../api", () => ({ accountCenterApi: {} }));

describe("useTenantAccountCenter", () => {
  beforeEach(() => vi.clearAllMocks());

  it("loads the unified USD balance", async () => {
    const api = fakeApi();
    let state: ReturnType<typeof useTenantAccountCenter> | undefined;
    const wrapper = mount(defineComponent({ setup() { state = useTenantAccountCenter({ api }); return () => h("div"); } }));

    await vi.waitFor(() => expect(state?.balance.value.availableUsd).toBe(1200));
    expect(state?.activeTab.value).toBe("ledger");
    wrapper.unmount();
  });
});

function fakeApi(): AccountCenterApi {
  return {
    getBalance: vi.fn().mockResolvedValue({ currency: "USD", totalUsd: 1500, usedUsd: 300, remainingUsd: 1200, availableUsd: 1200, permanentUsd: 1200, timedUsd: 0, outstandingDebtMicroUsd: 0, serviceState: "active", balanceLots: [] }),
    listRechargeRecords: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, size: 20 }),
    listPendingOrders: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, size: 20 }),
    listBalanceLedger: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, size: 20 }),
    getTopupConfig: vi.fn().mockResolvedValue({ enabled: true, currency: "USD", feeRateBp: 160, minMicroUsd: 10_000_000, maxMicroUsd: 10_000_000_000, packages: [] }),
    createTopupOrder: vi.fn(), getTopupOrder: vi.fn()
  };
}
