import { defineComponent } from "vue";
import { flushPromises, shallowMount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import TenantPaymentSettingsWorkspace from "./TenantPaymentSettingsWorkspace.vue";

const api = vi.hoisted(() => ({
  getPaymentSettings: vi.fn(),
  updatePaymentSettings: vi.fn()
}));

vi.mock("@/api/tenant", () => ({ tenantApi: api }));
vi.mock("element-plus", () => ({
  ElMessage: { error: vi.fn(), success: vi.fn() }
}));

const PanelStub = defineComponent({ template: "<div><slot /></div>" });
const ButtonStub = defineComponent({
  emits: ["click"],
  template: "<button @click=\"$emit('click')\"><slot /></button>"
});
const PassThroughStub = defineComponent({ template: "<div><slot /><slot name=\"suffix\" /></div>" });

const global = {
  directives: { loading: {} },
  stubs: {
    PortalPagePanel: PanelStub,
    ElButton: ButtonStub,
    ElForm: PassThroughStub,
    ElFormItem: PassThroughStub,
    ElInput: PassThroughStub,
    ElInputNumber: PassThroughStub,
    ElSwitch: PassThroughStub
  }
};

describe("TenantPaymentSettingsWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getPaymentSettings.mockResolvedValue({
      userCustomTopupFeeBp: 160,
      userCustomValidityDays: 30,
      userTopupPackages: [{
        id: "pkg-1",
        name: "用户体验包",
        paymentAmountMicroUsd: 20_000_000,
        giftAmountMicroUsd: 3_000_000,
        validityDays: 30,
        badge: "推荐",
        enabled: true,
        sortOrder: 10
      }]
    });
    api.updatePaymentSettings.mockResolvedValue({});
  });

  it("loads tenant user top-up settings and package previews", async () => {
    const wrapper = shallowMount(TenantPaymentSettingsWorkspace, { global });
    await flushPromises();

    expect(api.getPaymentSettings).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain("用户体验包");
    expect(wrapper.text()).toContain("推荐");

    wrapper.unmount();
  });

  it("converts the displayed fee percentage back to basis points on save", async () => {
    const wrapper = shallowMount(TenantPaymentSettingsWorkspace, { global });
    await flushPromises();

    const saveButton = wrapper.findAll("button").find((button) => button.text() === "保存用户充值设置");
    expect(saveButton).toBeDefined();
    await saveButton!.trigger("click");
    await flushPromises();

    expect(api.updatePaymentSettings).toHaveBeenCalledWith({
      userCustomTopupFeeBp: 160,
      userCustomValidityDays: 30,
      userTopupPackages: [{
        id: "pkg-1",
        name: "用户体验包",
        paymentAmountMicroUsd: 20_000_000,
        giftAmountMicroUsd: 3_000_000,
        validityDays: 30,
        badge: "推荐",
        enabled: true,
        sortOrder: 10
      }]
    });

    wrapper.unmount();
  });
});
