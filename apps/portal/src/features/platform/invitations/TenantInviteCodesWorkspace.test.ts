import { defineComponent } from "vue";
import { flushPromises, shallowMount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import TenantInviteCodesWorkspace from "./TenantInviteCodesWorkspace.vue";

const api = vi.hoisted(() => ({
  createInviteCode: vi.fn(),
  deleteInviteCode: vi.fn(),
  getInviteCodes: vi.fn(),
  updateInviteCode: vi.fn()
}));
const messageBox = vi.hoisted(() => ({ confirm: vi.fn() }));

vi.mock("@/api/platformTenant", () => ({ platformTenantApi: api }));
vi.mock("element-plus", () => ({
  ElMessage: { error: vi.fn(), success: vi.fn() },
  ElMessageBox: messageBox
}));

const PanelStub = defineComponent({ template: "<div><slot name=\"actions\" /><slot /><slot name=\"pagination\" /></div>" });
const PassThroughStub = defineComponent({ template: "<div><slot /><slot name=\"reference\" /><slot name=\"footer\" /></div>" });

const global = {
  directives: { loading: {} },
  stubs: {
    PortalPagePanel: PanelStub,
    DsTable: PassThroughStub,
    DsPagination: PassThroughStub,
    DsEmpty: PassThroughStub,
    ElButton: PassThroughStub,
    ElIcon: PassThroughStub,
    ElTooltip: PassThroughStub,
    ElDialog: PassThroughStub,
    ElForm: PassThroughStub,
    ElFormItem: PassThroughStub,
    ElInput: PassThroughStub,
    ElInputNumber: PassThroughStub,
    ElDatePicker: PassThroughStub
  }
};

describe("TenantInviteCodesWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getInviteCodes.mockResolvedValue({ items: [], total: 0, page: 1, size: 20 });
    api.createInviteCode.mockResolvedValue({});
    api.deleteInviteCode.mockResolvedValue({});
    api.updateInviteCode.mockResolvedValue({});
    messageBox.confirm.mockResolvedValue(true);
  });

  it("loads the first page through the tenant invite-code API", async () => {
    const wrapper = shallowMount(TenantInviteCodesWorkspace, { global });
    await flushPromises();

    expect(api.getInviteCodes).toHaveBeenCalledWith({ page: 1, size: 20 });
    expect(wrapper.text()).toContain("创建邀请码");

    wrapper.unmount();
  });

  it("confirms and applies a status change inside the workspace", async () => {
    const wrapper = shallowMount(TenantInviteCodesWorkspace, { global });
    await flushPromises();

    const vm = wrapper.vm as unknown as {
      toggleStatus: (row: { id: string; code: string; status: number }) => Promise<void>;
    };
    await vm.toggleStatus({ id: "invite-1", code: "ABC123", status: 1 });
    await flushPromises();

    expect(messageBox.confirm).toHaveBeenCalledOnce();
    expect(api.updateInviteCode).toHaveBeenCalledWith("invite-1", { status: 2 });
    expect(api.getInviteCodes).toHaveBeenCalledTimes(2);

    wrapper.unmount();
  });
});
