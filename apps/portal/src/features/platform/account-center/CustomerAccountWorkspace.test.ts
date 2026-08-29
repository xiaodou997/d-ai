import { flushPromises, mount } from "@vue/test-utils";
import ElementPlus from "element-plus";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import CustomerAccountWorkspace from "./CustomerAccountWorkspace.vue";

const getBalance = vi.hoisted(() => vi.fn());

vi.mock("@/api/platformCustomer", () => ({
  platformCustomerApi: { getBalance }
}));

const balance = {
  currency: "USD",
  totalUsd: 20,
  usedUsd: 7.5,
  remainingUsd: 12.5,
  availableUsd: 12.5,
  permanentUsd: 10,
  timedUsd: 2.5,
  outstandingDebtMicroUsd: 0,
  serviceState: "active",
  balanceLots: [{
    balanceLotId: "lot-1",
    totalUsd: 10,
    remainingUsd: 8,
    createdAt: "2026-08-01T00:00:00Z",
    expiresAt: "2099-08-01T00:00:00Z",
    source: "套餐充值"
  }]
};

describe("CustomerAccountWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getBalance.mockResolvedValue(balance);
  });

  afterEach(() => {
    document.body.innerHTML = "";
  });

  it("loads the balance and renders effective credit lots", async () => {
    const wrapper = await mountWorkspace();

    expect(getBalance).toHaveBeenCalledWith(true);
    expect(wrapper.text()).toContain("$12.50");
    expect(wrapper.text()).toContain("套餐充值");
    expect(wrapper.text()).toContain("可用");
    wrapper.unmount();
  });

  it("refreshes the customer balance from the workspace action", async () => {
    const wrapper = await mountWorkspace();
    const refreshButton = wrapper.findAll("button").find((button) => button.text().includes("立即刷新"));
    expect(refreshButton).toBeDefined();

    await refreshButton!.trigger("click");
    await flushPromises();
    expect(getBalance).toHaveBeenCalledTimes(2);
    wrapper.unmount();
  });
});

async function mountWorkspace() {
  const wrapper = mount(CustomerAccountWorkspace, {
    attachTo: document.body,
    global: { plugins: [ElementPlus] }
  });
  await flushPromises();
  return wrapper;
}
