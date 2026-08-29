import { defineComponent } from "vue";
import { flushPromises, shallowMount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import AdminPaymentSettingsWorkspace from "./AdminPaymentSettingsWorkspace.vue";

const api = vi.hoisted(() => ({
  getPaymentSettings: vi.fn(),
  getWechatConfig: vi.fn(),
  updatePaymentSettings: vi.fn(),
  updateWechatConfig: vi.fn()
}));

vi.mock("@/api/platformAdmin", () => ({ platformAdminApi: api }));
vi.mock("element-plus", () => ({
  ElMessage: { error: vi.fn(), success: vi.fn() }
}));

const PanelStub = defineComponent({
  template: "<div><slot /></div>"
});
const TabsStub = defineComponent({
  props: { modelValue: { type: String, default: "" }, tabs: { type: Array, default: () => [] } },
  emits: ["update:modelValue"],
  template: "<div class=\"tabs\"><button v-for=\"tab in tabs\" :key=\"tab.key\" @click=\"$emit('update:modelValue', tab.key)\">{{ tab.label }}</button></div>"
});
const ButtonStub = defineComponent({
  emits: ["click"],
  template: "<button @click=\"$emit('click')\"><slot /></button>"
});
const PassThroughStub = defineComponent({ template: "<div><slot /><slot name=\"suffix\" /></div>" });

const global = {
  directives: { loading: {} },
  stubs: {
    PortalPagePanel: PanelStub,
    DsTabs: TabsStub,
    ElButton: ButtonStub,
    ElForm: PassThroughStub,
    ElFormItem: PassThroughStub,
    ElInput: PassThroughStub,
    ElInputNumber: PassThroughStub,
    ElSelect: PassThroughStub,
    ElOption: PassThroughStub,
    ElSwitch: PassThroughStub
  }
};

describe("AdminPaymentSettingsWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getWechatConfig.mockResolvedValue({
      enabled: true,
      mock: false,
      verifyMode: "public_key",
      appId: "app-id",
      mchId: "mch-id",
      mchCertSerialNo: "serial",
      notifyBaseUrl: "https://example.com/platform",
      orderTtlSeconds: 7200,
      hasPrivateKey: true,
      hasApiv3Key: true,
      wechatPayPublicKeyId: "public-key-id",
      hasWechatPayPublicKey: true
    });
    api.getPaymentSettings.mockResolvedValue({
      tenantCustomTopupFeeBp: 160,
      tenantWithdrawFeeBp: 275,
      tenantCustomValidityDays: 30,
      tenantTopupPackages: [{
        id: "pkg-1",
        name: "体验额度包",
        paymentAmountMicroUsd: 10_000_000,
        giftAmountMicroUsd: 2_000_000,
        validityDays: 30,
        badge: "推荐",
        enabled: true,
        sortOrder: 10
      }]
    });
    api.updatePaymentSettings.mockResolvedValue({});
    api.updateWechatConfig.mockResolvedValue({});
  });

  it("loads the payment rules and WeChat configuration into one workspace", async () => {
    const wrapper = shallowMount(AdminPaymentSettingsWorkspace, { global });
    await flushPromises();

    expect(api.getWechatConfig).toHaveBeenCalledOnce();
    expect(api.getPaymentSettings).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain("体验额度包");
    expect(wrapper.text()).toContain("推荐");

    wrapper.unmount();
  });

  it("round-trips loaded payment settings when saving the rules tab", async () => {
    const wrapper = shallowMount(AdminPaymentSettingsWorkspace, { global });
    await flushPromises();

    const saveButton = wrapper.findAll("button").find((button) => button.text() === "保存充值与提现规则");
    expect(saveButton).toBeDefined();
    await saveButton!.trigger("click");
    await flushPromises();

    expect(api.updatePaymentSettings).toHaveBeenCalledWith({
      tenantCustomTopupFeeBp: 160,
      tenantWithdrawFeeBp: 275,
      tenantCustomValidityDays: 30,
      tenantTopupPackages: [{
        id: "pkg-1",
        name: "体验额度包",
        paymentAmountMicroUsd: 10_000_000,
        giftAmountMicroUsd: 2_000_000,
        validityDays: 30,
        badge: "推荐",
        enabled: true,
        sortOrder: 10
      }]
    });

    wrapper.unmount();
  });
});
