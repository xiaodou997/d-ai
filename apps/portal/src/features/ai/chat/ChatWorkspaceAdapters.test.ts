import { shallowMount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { PortalChatApi } from "@/platform/ai/chat";
import CustomerChatWorkspace from "./CustomerChatWorkspace.vue";
import TenantChatWorkspace from "./TenantChatWorkspace.vue";

const tenantRuntimeApi = vi.hoisted(() => ({
  listModels: vi.fn(),
  listSessions: vi.fn(),
  createSession: vi.fn(),
  getSession: vi.fn(),
  deleteSession: vi.fn()
}));
const customerRuntimeApi = vi.hoisted(() => ({
  listModels: vi.fn(),
  listSessions: vi.fn(),
  createSession: vi.fn(),
  getSession: vi.fn(),
  deleteSession: vi.fn()
}));
const tenantStream = vi.hoisted(() => vi.fn());
const customerStream = vi.hoisted(() => vi.fn());
const platform = vi.hoisted(() => ({
  createNamedConfirmDialog: vi.fn(() => vi.fn()),
  notifyError: vi.fn(),
  notifySuccess: vi.fn(),
  notifyWarning: vi.fn()
}));

vi.mock("@/api/aiTenant", () => ({ runtimeChatApi: tenantRuntimeApi, streamRuntimeChatMessage: tenantStream }));
vi.mock("@/api/aiCustomer", () => ({ runtimeChatApi: customerRuntimeApi, streamRuntimeChatMessage: customerStream }));
vi.mock("@/platform", () => platform);

const chatWorkspaceStub = {
  name: "PortalChatWorkspace",
  props: ["api", "usageMessage", "sourceLabel", "confirmDelete"],
  template: "<div />"
};

describe("AI chat workspace adapters", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    tenantRuntimeApi.listModels.mockResolvedValue([]);
    customerRuntimeApi.listSessions.mockResolvedValue([]);
  });

  it("binds the tenant runtime facade and tenant usage copy", async () => {
    const wrapper = shallowMount(TenantChatWorkspace, { global: { stubs: { PortalChatWorkspace: chatWorkspaceStub } } });
    const workspace = wrapper.findComponent({ name: "PortalChatWorkspace" });
    const api = workspace.props("api") as PortalChatApi;

    await api.listModels();

    expect(tenantRuntimeApi.listModels).toHaveBeenCalledOnce();
    expect(workspace.props("usageMessage")).toBe("消耗计入租户用量。");
    expect(workspace.props("sourceLabel")).toBe("租户网页对话");
    expect(workspace.props("confirmDelete")).toEqual(expect.any(Function));
    wrapper.unmount();
  });

  it("binds the customer runtime facade and personal usage copy", async () => {
    const wrapper = shallowMount(CustomerChatWorkspace, { global: { stubs: { PortalChatWorkspace: chatWorkspaceStub } } });
    const workspace = wrapper.findComponent({ name: "PortalChatWorkspace" });
    const api = workspace.props("api") as PortalChatApi;

    await api.listSessions();

    expect(customerRuntimeApi.listSessions).toHaveBeenCalledOnce();
    expect(workspace.props("usageMessage")).toBe("消耗计入个人用量。");
    expect(workspace.props("sourceLabel")).toBe("个人网页对话");
    wrapper.unmount();
  });
});
