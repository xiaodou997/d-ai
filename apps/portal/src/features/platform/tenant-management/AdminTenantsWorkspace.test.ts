import { defineComponent } from "vue";
import { flushPromises, shallowMount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import AdminTenantsWorkspace from "./AdminTenantsWorkspace.vue";

const api = vi.hoisted(() => ({
  createRecharge: vi.fn(),
  createTenant: vi.fn(),
  deleteTenant: vi.fn(),
  enterTenantOperations: vi.fn(),
  listTenants: vi.fn(),
  updateTenant: vi.fn(),
  updateTenantStatus: vi.fn()
}));
const router = vi.hoisted(() => ({ push: vi.fn() }));
const authStore = vi.hoisted(() => ({ enterTenantOperations: vi.fn() }));

vi.mock("@/api/platformAdmin", () => ({ platformAdminApi: api }));
vi.mock("vue-router", () => ({ useRouter: () => router }));
vi.mock("@/stores/auth", () => ({ useAuthStore: () => authStore }));
vi.mock("element-plus", () => ({
  ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn() },
  ElMessageBox: { confirm: vi.fn(), prompt: vi.fn() }
}));

const PanelStub = defineComponent({ template: "<div><slot name=\"actions\" /><slot name=\"filters\" /><slot /><slot name=\"pagination\" /></div>" });
const PassThroughStub = defineComponent({ template: "<div><slot /><slot name=\"reference\" /><slot name=\"footer\" /></div>" });

const global = {
  stubs: {
    PortalPagePanel: PanelStub,
    DsFilterBar: PassThroughStub,
    DsFilterField: PassThroughStub,
    DsTable: PassThroughStub,
    DsPagination: PassThroughStub,
    AccountOverviewDrawer: true,
    RechargeDialog: true,
    ElButton: PassThroughStub,
    ElIcon: PassThroughStub,
    ElDialog: PassThroughStub,
    ElForm: PassThroughStub,
    ElFormItem: PassThroughStub,
    ElInput: PassThroughStub,
    ElSelect: PassThroughStub,
    ElOption: PassThroughStub,
    ElSwitch: PassThroughStub,
    ElTooltip: PassThroughStub,
    ElPopconfirm: PassThroughStub,
    ElDivider: PassThroughStub
  }
};

describe("AdminTenantsWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listTenants.mockResolvedValue({ items: [], total: 0 });
    api.createRecharge.mockResolvedValue({});
    api.createTenant.mockResolvedValue({ tenantId: "tenant-1" });
    api.deleteTenant.mockResolvedValue({});
    api.enterTenantOperations.mockResolvedValue({
      accessToken: "tenant-operations-token",
      expiresIn: 3600,
      tenantId: "tenant-1",
      tenantName: "Tenant One"
    });
    api.updateTenant.mockResolvedValue({});
    api.updateTenantStatus.mockResolvedValue({});
  });

  it("loads the first tenant page with normalized filters", async () => {
    const wrapper = shallowMount(AdminTenantsWorkspace, { global });
    await flushPromises();

    expect(api.listTenants).toHaveBeenCalledWith({
      keyword: undefined,
      status: undefined,
      page: 1,
      size: 20
    });

    wrapper.unmount();
  });

  it("keeps tenant search and status filters inside the feature workspace", async () => {
    const wrapper = shallowMount(AdminTenantsWorkspace, { global });
    await flushPromises();

    const vm = wrapper.vm as unknown as {
      query: { keyword: string; status: number | "" };
      search: () => Promise<void>;
    };
    vm.query.keyword = "Acme";
    vm.query.status = 2;
    await vm.search();

    expect(api.listTenants).toHaveBeenLastCalledWith({
      keyword: "Acme",
      status: 2,
      page: 1,
      size: 20
    });

    wrapper.unmount();
  });

  it("enters tenant operations and opens the tenant overview", async () => {
    const wrapper = shallowMount(AdminTenantsWorkspace, { global });
    await flushPromises();

    const vm = wrapper.vm as unknown as {
      handleTenantOperations: (row: { tenantId: string; tenantName: string; status: number }) => Promise<void>;
    };
    await vm.handleTenantOperations({ tenantId: "tenant-1", tenantName: "Tenant One", status: 1 });

    expect(api.enterTenantOperations).toHaveBeenCalledWith("tenant-1");
    expect(authStore.enterTenantOperations).toHaveBeenCalledWith(expect.objectContaining({
      accessToken: "tenant-operations-token",
      tenantId: "tenant-1"
    }));
    expect(router.push).toHaveBeenCalledWith("/tenant/overview/business");
    wrapper.unmount();
  });
});
