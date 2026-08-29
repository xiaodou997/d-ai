import { shallowMount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { PortalTaskApi, PortalTaskRecord } from "@/platform/ai/tasks";
import CustomerTasksWorkspace from "./CustomerTasksWorkspace.vue";
import TenantTasksWorkspace from "./TenantTasksWorkspace.vue";

const tenantRuntimeApi = vi.hoisted(() => ({
  listTasks: vi.fn(),
  getTask: vi.fn(),
  cancelTask: vi.fn(),
  deleteTask: vi.fn()
}));
const customerRuntimeApi = vi.hoisted(() => ({
  listTasks: vi.fn(),
  getTask: vi.fn(),
  cancelTask: vi.fn(),
  deleteTask: vi.fn()
}));
const platform = vi.hoisted(() => ({
  confirmDialog: vi.fn(),
  notifyError: vi.fn(),
  notifySuccess: vi.fn()
}));

vi.mock("@/api/aiTenant", () => ({ runtimeTaskApi: tenantRuntimeApi }));
vi.mock("@/api/aiCustomer", () => ({ runtimeTaskApi: customerRuntimeApi }));
vi.mock("@/platform", () => platform);

const taskWorkspaceStub = {
  name: "PortalTaskWorkspace",
  props: ["api", "mode", "eyebrow", "confirmDelete"],
  template: "<div />"
};

const task: PortalTaskRecord = {
  id: "task-1",
  type: "images.generation",
  source: "portal",
  status: "completed",
  owner: { scope: "tenant" },
  permissions: { read_only: false, can_cancel: false, can_delete: true },
  attempt: 1,
  result_available: true,
  created_at: 1_700_000_000_000
};

describe("AI task workspace adapters", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    tenantRuntimeApi.listTasks.mockResolvedValue({ items: [], has_more: false });
    customerRuntimeApi.listTasks.mockResolvedValue({ items: [], has_more: false });
    platform.confirmDialog.mockResolvedValue(true);
  });

  it("keeps tenant task mode and confirmation wording in the feature", async () => {
    const wrapper = shallowMount(TenantTasksWorkspace, { global: { stubs: { PortalTaskWorkspace: taskWorkspaceStub } } });
    const workspace = wrapper.findComponent({ name: "PortalTaskWorkspace" });
    const api = workspace.props("api") as PortalTaskApi;

    await api.listTasks({ status: "running" });
    const confirmDelete = workspace.props("confirmDelete") as (value: PortalTaskRecord) => Promise<boolean>;
    await confirmDelete(task);

    expect(tenantRuntimeApi.listTasks).toHaveBeenCalledWith({ status: "running" });
    expect(workspace.props("mode")).toBe("tenant");
    expect(workspace.props("eyebrow")).toBe("智能服务 / 任务管理");
    expect(platform.confirmDialog).toHaveBeenCalledWith(expect.objectContaining({ message: expect.stringContaining("文生图") }));
    wrapper.unmount();
  });

  it("keeps customer task mode while delegating task deletion", async () => {
    const wrapper = shallowMount(CustomerTasksWorkspace, { global: { stubs: { PortalTaskWorkspace: taskWorkspaceStub } } });
    const workspace = wrapper.findComponent({ name: "PortalTaskWorkspace" });
    const api = workspace.props("api") as PortalTaskApi;

    await api.deleteTask("task-1");

    expect(customerRuntimeApi.deleteTask).toHaveBeenCalledWith("task-1");
    expect(workspace.props("mode")).toBe("user");
    expect(workspace.props("eyebrow")).toBeUndefined();
    wrapper.unmount();
  });
});
