import { shallowMount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { PortalImageApi } from "@/platform/ai/images";
import CustomerImageStudioWorkspace from "./CustomerImageStudioWorkspace.vue";
import TenantImageStudioWorkspace from "./TenantImageStudioWorkspace.vue";

const tenantRuntimeApi = vi.hoisted(() => ({
  listModels: vi.fn(),
  listJobs: vi.fn(),
  createTask: vi.fn(),
  getTask: vi.fn(),
  cancelTask: vi.fn(),
  deleteTask: vi.fn()
}));
const customerRuntimeApi = vi.hoisted(() => ({
  listModels: vi.fn(),
  listJobs: vi.fn(),
  createTask: vi.fn(),
  getTask: vi.fn(),
  cancelTask: vi.fn(),
  deleteTask: vi.fn()
}));
const tenantFormatUSD = vi.hoisted(() => vi.fn());
const customerFormatUSD = vi.hoisted(() => vi.fn());
const platform = vi.hoisted(() => ({
  notifyError: vi.fn(),
  notifySuccess: vi.fn()
}));

vi.mock("@/api/aiTenant", () => ({ runtimeImageApi: tenantRuntimeApi, formatUSD: tenantFormatUSD }));
vi.mock("@/api/aiCustomer", () => ({ runtimeImageApi: customerRuntimeApi, formatUSD: customerFormatUSD }));
vi.mock("@/api/request", () => ({ apiBaseUrl: "https://api.example.test" }));
vi.mock("@/platform", () => platform);

const imageWorkspaceStub = {
  name: "PortalImageStudioWorkspace",
  props: ["api", "formatUSD", "usageMessage", "assetBaseUrl"],
  template: "<div />"
};

describe("AI image workspace adapters", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    tenantRuntimeApi.listJobs.mockResolvedValue([]);
    customerRuntimeApi.listModels.mockResolvedValue([]);
  });

  it("binds tenant image operations and asset settings", async () => {
    const wrapper = shallowMount(TenantImageStudioWorkspace, { global: { stubs: { PortalImageStudioWorkspace: imageWorkspaceStub } } });
    const workspace = wrapper.findComponent({ name: "PortalImageStudioWorkspace" });
    const api = workspace.props("api") as PortalImageApi;

    await api.listJobs();

    expect(tenantRuntimeApi.listJobs).toHaveBeenCalledOnce();
    expect(workspace.props("formatUSD")).toBe(tenantFormatUSD);
    expect(workspace.props("usageMessage")).toBe("消耗会计入租户用量");
    expect(workspace.props("assetBaseUrl")).toBe("https://api.example.test");
    wrapper.unmount();
  });

  it("binds customer image operations and personal usage copy", async () => {
    const wrapper = shallowMount(CustomerImageStudioWorkspace, { global: { stubs: { PortalImageStudioWorkspace: imageWorkspaceStub } } });
    const workspace = wrapper.findComponent({ name: "PortalImageStudioWorkspace" });
    const api = workspace.props("api") as PortalImageApi;

    await api.listModels();

    expect(customerRuntimeApi.listModels).toHaveBeenCalledOnce();
    expect(workspace.props("formatUSD")).toBe(customerFormatUSD);
    expect(workspace.props("usageMessage")).toBe("消耗会计入个人用量");
    wrapper.unmount();
  });
});
