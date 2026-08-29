import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { DsPagination } from "@/shared/ui";
import CustomerRechargeRecordsWorkspace from "./CustomerRechargeRecordsWorkspace.vue";

const getRechargeRecords = vi.hoisted(() => vi.fn());

vi.mock("@/api/platformCustomer", () => ({
  platformCustomerApi: { getRechargeRecords }
}));

const record = {
  orderId: "topup-1",
  orderType: "wechat",
  paidAmountMinor: 1_000,
  amountUsd: 10,
  status: "SUCCESS",
  note: "套餐充值",
  userId: "user-1",
  username: "alice",
  tenantName: "示例租户",
  createdTime: 1_700_000_000_000
};

describe("CustomerRechargeRecordsWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getRechargeRecords.mockResolvedValue({ items: [record], total: 21, page: 1, size: 20 });
  });

  it("loads and renders the customer's recharge history", async () => {
    const wrapper = await mountWorkspace();

    expect(getRechargeRecords).toHaveBeenCalledWith({ page: 1, size: 20 });
    expect(wrapper.text()).toContain("topup-1");
    expect(wrapper.text()).toContain("成功");
    expect(wrapper.text()).toContain("+$10.00");
    wrapper.unmount();
  });

  it("reloads the requested page from shared pagination", async () => {
    const wrapper = await mountWorkspace();
    getRechargeRecords.mockResolvedValue({ items: [], total: 21, page: 2, size: 20 });
    wrapper.findComponent(DsPagination).vm.$emit("update:page", 2);
    await flushPromises();

    expect(getRechargeRecords).toHaveBeenLastCalledWith({ page: 2, size: 20 });
    wrapper.unmount();
  });
});

async function mountWorkspace() {
  const wrapper = mount(CustomerRechargeRecordsWorkspace);
  await flushPromises();
  return wrapper;
}
