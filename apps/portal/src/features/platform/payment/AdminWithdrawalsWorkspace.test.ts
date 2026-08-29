import { flushPromises, mount } from "@vue/test-utils";
import ElementPlus from "element-plus";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import AdminWithdrawalsWorkspace from "./AdminWithdrawalsWorkspace.vue";

const api = vi.hoisted(() => ({
  listWithdrawals: vi.fn(),
  listTenants: vi.fn(),
  createWithdrawal: vi.fn()
}));

vi.mock("@/api/platformAdmin", () => ({ platformAdminApi: api }));

const withdrawal = {
  withdrawalId: "wd-1",
  currency: "USD",
  amountMicroUsd: 12_500_000,
  feeAmountMicroUsd: 500_000,
  payoutAmountMicroUsd: 12_000_000,
  accountName: "收款人",
  bankName: "示例银行",
  accountNo: "62220000",
  status: "paid",
  createdAt: 1_700_000_000_000
};

describe("AdminWithdrawalsWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listWithdrawals.mockResolvedValue({ items: [withdrawal], total: 1, page: 1, size: 20 });
    api.listTenants.mockResolvedValue({ items: [{ tenantId: "tenant-1", tenantName: "示例租户" }], total: 1, page: 1, size: 100 });
  });

  it("loads withdrawal records through the payment feature boundary", async () => {
    const { wrapper } = await mountWorkspace();

    expect(api.listWithdrawals).toHaveBeenCalledWith({
      status: undefined,
      page: 1,
      size: 20
    });
    expect(wrapper.text()).toContain("wd-1");
    expect(wrapper.text()).toContain("已记录（已扣减）");
    wrapper.unmount();
  });

  it("loads tenants when opening the withdrawal form", async () => {
    const { wrapper } = await mountWorkspace();
    const createButton = wrapper.findAll("button").find((button) => button.text().includes("创建提现记录"));
    expect(createButton).toBeDefined();

    await createButton!.trigger("click");
    await flushPromises();
    expect(api.listTenants).toHaveBeenCalledWith({ page: 1, size: 100, status: 1 });
    expect(document.body.textContent).toContain("确认创建并扣减");
    wrapper.unmount();
  });
});

async function mountWorkspace() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/admin/settlement/withdrawals", component: AdminWithdrawalsWorkspace }]
  });
  await router.push("/admin/settlement/withdrawals");
  await router.isReady();
  const wrapper = mount(AdminWithdrawalsWorkspace, {
    attachTo: document.body,
    global: { plugins: [router, ElementPlus] }
  });
  await flushPromises();
  return { router, wrapper };
}
