import { defineComponent } from "vue";
import { flushPromises, shallowMount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import TenantDetailWorkspace from "./TenantDetailWorkspace.vue";

const api = vi.hoisted(() => ({
  createRecharge: vi.fn(),
  createTenantUser: vi.fn(),
  getAccountBalance: vi.fn(),
  getTenant: vi.fn(),
  listEndUsers: vi.fn(),
  listTenantUsers: vi.fn(),
  resetTenantUserPassword: vi.fn(),
  updateTenantUserStatus: vi.fn()
}));
const route = vi.hoisted(() => ({ params: { id: "tenant-1" }, query: {} as Record<string, string> }));
const router = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }));
const messageBox = vi.hoisted(() => ({ confirm: vi.fn(), prompt: vi.fn() }));

vi.mock("@/api/platformAdmin", () => ({ platformAdminApi: api }));
vi.mock("vue-router", () => ({ useRoute: () => route, useRouter: () => router }));
vi.mock("element-plus", () => ({
  ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn() },
  ElMessageBox: messageBox
}));

const PanelStub = defineComponent({ template: "<div><slot name=\"actions\" /><slot /></div>" });
const TabsStub = defineComponent({
  props: { modelValue: { type: String, default: "" }, tabs: { type: Array, default: () => [] } },
  emits: ["update:modelValue"],
  template: "<div class=\"tabs\"><button v-for=\"tab in tabs\" :key=\"tab.key\" @click=\"$emit('update:modelValue', tab.key)\">{{ tab.label }}</button></div>"
});
const ButtonStub = defineComponent({ emits: ["click"], template: "<button @click=\"$emit('click')\"><slot /></button>" });
const PassThroughStub = defineComponent({ template: "<div><slot /><slot name=\"footer\" /></div>" });

const global = {
  stubs: {
    PortalPagePanel: PanelStub,
    DsTabs: TabsStub,
    DsTable: PassThroughStub,
    DsPagination: PassThroughStub,
    DsTag: PassThroughStub,
    AccountOverviewDrawer: true,
    RechargeDialog: true,
    ElButton: ButtonStub,
    ElDialog: PassThroughStub,
    ElForm: PassThroughStub,
    ElFormItem: PassThroughStub,
    ElInput: PassThroughStub,
    ElOption: PassThroughStub,
    ElSelect: PassThroughStub,
    ElTooltip: PassThroughStub,
    ElIcon: PassThroughStub
  }
};

describe("TenantDetailWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    route.params.id = "tenant-1";
    route.query = {};
    api.getTenant.mockResolvedValue({ tenantId: "tenant-1", tenantName: "Acme", status: 1, statusDisplay: "正常" });
    api.getAccountBalance.mockResolvedValue({ availableUsd: 12.5 });
    api.listTenantUsers.mockResolvedValue({ items: [], total: 0, page: 1, size: 20 });
    api.listEndUsers.mockResolvedValue({ items: [], total: 0, page: 1, size: 100 });
    api.createRecharge.mockResolvedValue({});
    api.createTenantUser.mockResolvedValue({ activationToken: "token", activationExpiresIn: 3600 });
    api.resetTenantUserPassword.mockResolvedValue({ activationToken: "token", activationExpiresIn: 3600 });
    api.updateTenantUserStatus.mockResolvedValue({});
    messageBox.confirm.mockResolvedValue(true);
  });

  it("loads tenant metadata, balance and organization users from the route tenant", async () => {
    const wrapper = shallowMount(TenantDetailWorkspace, { global });
    await flushPromises();

    expect(api.getTenant).toHaveBeenCalledWith("tenant-1");
    expect(api.getAccountBalance).toHaveBeenCalledWith({ accountType: 1, accountId: "tenant-1" });
    expect(api.listTenantUsers).toHaveBeenCalledWith({ tenantId: "tenant-1", page: 1, size: 20, keyword: undefined });
    wrapper.unmount();
  });

  it("loads associated users lazily and preserves the selected tab in the route", async () => {
    const wrapper = shallowMount(TenantDetailWorkspace, { global });
    await flushPromises();

    wrapper.findComponent(TabsStub).vm.$emit("update:modelValue", "users");
    await flushPromises();

    expect(api.listEndUsers).toHaveBeenCalledWith({ tenantId: "tenant-1", page: 1, size: 100 });
    expect(router.replace).toHaveBeenCalledWith({ query: { tab: "users" } });

    wrapper.unmount();
  });
});
