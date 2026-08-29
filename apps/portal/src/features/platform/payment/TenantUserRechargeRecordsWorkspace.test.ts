import { flushPromises, mount } from "@vue/test-utils";
import ElementPlus from "element-plus";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import TenantUserRechargeRecordsWorkspace from "./TenantUserRechargeRecordsWorkspace.vue";

const api = vi.hoisted(() => ({
  getUserRechargeRecords: vi.fn(),
  reverseRecharge: vi.fn()
}));

vi.mock("@/api/platformTenant", () => ({ platformTenantApi: api }));

const record = {
  orderId: "recharge-1",
  userId: "user-1",
  username: "alice",
  amountUsd: 10,
  paidAmountMinor: 1_000,
  orderType: "manual",
  status: "active",
  note: "运营补充",
  createdTime: 1_700_000_000_000
};

describe("TenantUserRechargeRecordsWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getUserRechargeRecords.mockResolvedValue({ items: [record], total: 1, page: 1, size: 20 });
    api.reverseRecharge.mockResolvedValue({ status: "REVERSED", orderId: record.orderId });
  });

  it("loads tenant user recharge records with normalized pagination", async () => {
    const { wrapper } = await mountWorkspace();

    expect(api.getUserRechargeRecords).toHaveBeenCalledWith({
      page: 1,
      size: 20,
      username: undefined,
      timeFrom: undefined,
      timeTo: undefined
    });
    expect(wrapper.text()).toContain("recharge-1");
    expect(wrapper.text()).toContain("alice");
    wrapper.unmount();
  });

  it("requires a reason before reversing a recharge record", async () => {
    const { wrapper } = await mountWorkspace();
    await wrapper.get("button.font-bold").trigger("click");
    await flushPromises();
    expect(document.body.textContent).toContain("确认撤销充值");

    const confirmButton = Array.from(document.body.querySelectorAll("button")).find((button) => button.textContent?.includes("确认撤销"));
    expect(confirmButton).toBeDefined();
    await confirmButton!.click();
    await flushPromises();
    expect(api.reverseRecharge).not.toHaveBeenCalled();

    const textarea = document.body.querySelector("textarea");
    expect(textarea).toBeTruthy();
    textarea!.value = "重复充值";
    textarea!.dispatchEvent(new Event("input", { bubbles: true }));
    await flushPromises();
    await confirmButton!.click();
    await flushPromises();
    expect(api.reverseRecharge).toHaveBeenCalledWith("recharge-1", { reason: "重复充值" });
    wrapper.unmount();
  });
});

async function mountWorkspace() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/tenant/finance/user-recharges", component: TenantUserRechargeRecordsWorkspace }]
  });
  await router.push("/tenant/finance/user-recharges");
  await router.isReady();
  const wrapper = mount(TenantUserRechargeRecordsWorkspace, {
    attachTo: document.body,
    global: { plugins: [router, ElementPlus] }
  });
  await flushPromises();
  return { router, wrapper };
}
