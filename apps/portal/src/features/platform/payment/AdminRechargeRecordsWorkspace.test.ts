import { defineComponent } from "vue";
import { flushPromises, shallowMount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import AdminRechargeRecordsWorkspace from "./AdminRechargeRecordsWorkspace.vue";

const api = vi.hoisted(() => ({
  getAdminRechargeOrder: vi.fn(),
  listAdminRechargeOrders: vi.fn(),
  syncAdminRechargeOrder: vi.fn()
}));

vi.mock("@/api/platformAdmin", () => ({ platformAdminApi: api }));
vi.mock("element-plus", () => ({
  ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn() }
}));

const PanelStub = defineComponent({
  template: "<div><slot name=\"filters\" /><slot /><slot name=\"pagination\" /></div>"
});
const PassThroughStub = defineComponent({ template: "<div><slot /><slot name=\"actions\" /><slot name=\"footer\" /></div>" });

const global = {
  stubs: {
    PortalPagePanel: PanelStub,
    DsFilterBar: PassThroughStub,
    DsFilterField: PassThroughStub,
    DsTable: PassThroughStub,
    DsPagination: PassThroughStub,
    DsDrawer: PassThroughStub,
    DsSkeleton: PassThroughStub,
    DsTag: PassThroughStub,
    ElButton: PassThroughStub,
    ElDialog: PassThroughStub,
    ElForm: PassThroughStub,
    ElFormItem: PassThroughStub,
    ElInput: PassThroughStub,
    ElSelect: PassThroughStub,
    ElOption: PassThroughStub,
    ElSegmented: PassThroughStub,
    ElDatePicker: PassThroughStub,
    ElTooltip: PassThroughStub
  }
};

describe("AdminRechargeRecordsWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listAdminRechargeOrders.mockResolvedValue({ items: [], total: 0 });
    api.getAdminRechargeOrder.mockResolvedValue({});
    api.syncAdminRechargeOrder.mockResolvedValue({});
  });

  it("loads the first page with normalized empty filters", async () => {
    const wrapper = shallowMount(AdminRechargeRecordsWorkspace, { global });
    await flushPromises();

    expect(api.listAdminRechargeOrders).toHaveBeenCalledWith({
      keyword: undefined,
      method: undefined,
      targetType: undefined,
      paymentStatus: undefined,
      fulfillmentStatus: undefined,
      refundStatus: undefined,
      page: 1,
      size: 20
    });

    wrapper.unmount();
  });

  it("keeps filters inside the workspace when searching", async () => {
    const wrapper = shallowMount(AdminRechargeRecordsWorkspace, { global });
    await flushPromises();

    const vm = wrapper.vm as unknown as {
      query: { keyword: string; method: string };
      search: () => Promise<void>;
    };
    vm.query.keyword = "order-42";
    vm.query.method = "online";
    await vm.search();

    expect(api.listAdminRechargeOrders).toHaveBeenLastCalledWith({
      keyword: "order-42",
      method: "online",
      targetType: undefined,
      paymentStatus: undefined,
      fulfillmentStatus: undefined,
      refundStatus: undefined,
      page: 1,
      size: 20
    });

    wrapper.unmount();
  });
});
