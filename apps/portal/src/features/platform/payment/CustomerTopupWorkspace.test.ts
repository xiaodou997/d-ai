import { flushPromises, shallowMount } from "@vue/test-utils";
import ElementPlus from "element-plus";
import { beforeEach, describe, expect, it, vi } from "vitest";

import CustomerTopupWorkspace from "./CustomerTopupWorkspace.vue";

const api = vi.hoisted(() => ({
  getTopupConfig: vi.fn(),
  listTopupOrders: vi.fn(),
  createTopupOrder: vi.fn(),
  getTopupOrder: vi.fn()
}));

vi.mock("@/api/platformCustomer", () => ({ platformCustomerApi: api }));

const config = {
  enabled: true,
  currency: "USD",
  feeRateBp: 160,
  minMicroUsd: 10_000_000,
  maxMicroUsd: 10_000_000_000,
  packages: [{
    id: "pkg-10",
    name: "基础额度包",
    paymentAmountMicroUsd: 10_000_000,
    giftAmountMicroUsd: 1_000_000,
    enabled: true,
    sortOrder: 1
  }]
};

describe("CustomerTopupWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getTopupConfig.mockResolvedValue(config);
    api.listTopupOrders.mockResolvedValue({ items: [], total: 0, page: 1, size: 20 });
    api.createTopupOrder.mockResolvedValue({
      orderId: "order-1",
      codeUrl: "https://pay.example/qr",
      paymentCurrency: "CNY",
      paymentAmountMinor: 7200,
      grossAmountMicroUsd: 10_000_000,
      feeAmountMicroUsd: 160_000,
      giftAmountMicroUsd: 1_000_000,
      creditedAmountMicroUsd: 10_840_000,
      topupMode: "package",
      status: "created",
      expiresAt: Date.now() + 300_000
    });
  });

  it("loads top-up configuration and paged orders", async () => {
    const wrapper = await mountWorkspace();

    expect(api.getTopupConfig).toHaveBeenCalledOnce();
    expect(api.listTopupOrders).toHaveBeenCalledWith({ page: 1, size: 20 });
    expect(wrapper.text()).toContain("基础额度包");
    expect(wrapper.text()).toContain("选择充值金额");
    wrapper.unmount();
  });

  it("creates a package order inside the payment workspace", async () => {
    const wrapper = await mountWorkspace();

    await wrapper.get("button.package-card").trigger("click");
    await flushPromises();

    expect(api.createTopupOrder).toHaveBeenCalledWith({ packageId: "pkg-10" });
    expect(wrapper.findComponent({ name: "PortalQrPayDialog" }).exists()).toBe(true);
    wrapper.unmount();
  });
});

async function mountWorkspace() {
  const wrapper = shallowMount(CustomerTopupWorkspace, {
    global: {
      plugins: [ElementPlus],
      stubs: {
        PortalPagePanel: { template: "<main><slot /><slot name='pagination' /></main>" },
        PortalQrPayDialog: { name: "PortalQrPayDialog", template: "<div data-testid='qr-dialog' />" },
        DsTable: { template: "<div><slot /></div>" },
        DsPagination: { template: "<div />" }
      }
    }
  });
  await flushPromises();
  return wrapper;
}
