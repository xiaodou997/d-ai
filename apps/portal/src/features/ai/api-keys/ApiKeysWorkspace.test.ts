import { shallowMount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { PortalApiKeyWriteInput } from "@/platform/ai/apikeys";
import CustomerApiKeysWorkspace from "./CustomerApiKeysWorkspace.vue";
import TenantApiKeysWorkspace from "./TenantApiKeysWorkspace.vue";

const tenantApi = vi.hoisted(() => ({
  listApiKeys: vi.fn(),
  createApiKey: vi.fn(),
  updateApiKey: vi.fn(),
  updateApiKeyStatus: vi.fn(),
  revealApiKey: vi.fn(),
  rotateApiKey: vi.fn(),
  deleteApiKey: vi.fn(),
  listMyGroups: vi.fn()
}));
const customerApi = vi.hoisted(() => ({
  listApiKeys: vi.fn(),
  createApiKey: vi.fn(),
  updateApiKey: vi.fn(),
  updateApiKeyStatus: vi.fn(),
  revealApiKey: vi.fn(),
  rotateApiKey: vi.fn(),
  deleteApiKey: vi.fn(),
  listMyGroups: vi.fn()
}));
const platform = vi.hoisted(() => ({
  confirmDialog: vi.fn(),
  createNamedConfirmDialog: vi.fn(() => vi.fn()),
  notifyError: vi.fn(),
  notifySuccess: vi.fn(),
  notifyWarning: vi.fn(),
  resolvePortalPublicBaseUrl: vi.fn((value: string | undefined) => value ?? "")
}));

vi.mock("@/api/aiTenant", () => ({ aiTenantApi: tenantApi, statusOptions: [] }));
vi.mock("@/api/aiCustomer", () => ({ aiCustomerApi: customerApi, statusOptions: [] }));
vi.mock("@/platform", () => platform);

describe("API key feature workspaces", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    tenantApi.createApiKey.mockResolvedValue({});
    customerApi.listApiKeys.mockResolvedValue({ items: [], total: 0 });
  });

  it("normalizes tenant API key writes before delegating to the tenant facade", async () => {
    const wrapper = shallowMount(TenantApiKeysWorkspace, { global: { stubs: { PortalApiKeyWorkspace: apiKeyStub } } });
    const keyApi = wrapper.findComponent({ name: "PortalApiKeyWorkspace" }).props("api") as {
      createApiKey: (payload: PortalApiKeyWriteInput) => Promise<unknown>;
    };
    const payload: PortalApiKeyWriteInput = {
      name: "demo",
      group_id: "group-1",
      quota_limit_micro_usd: undefined,
      status: "active",
      expires_at: undefined,
      limit_policy: { concurrency_limit: undefined, status: "disabled" }
    };

    await keyApi.createApiKey(payload);

    expect(tenantApi.createApiKey).toHaveBeenCalledWith({
      name: "demo",
      group_id: "group-1",
      quota_limit_micro_usd: null,
      status: "active",
      expires_at: null,
      limit_policy: { concurrency_limit: null, status: "disabled" }
    });
    wrapper.unmount();
  });

  it("shares the generic workspace contract for customer API keys", async () => {
    const wrapper = shallowMount(CustomerApiKeysWorkspace, {
      props: { embedded: true },
      global: { stubs: { PortalApiKeyWorkspace: apiKeyStub } }
    });
    const child = wrapper.findComponent({ name: "PortalApiKeyWorkspace" });
    const keyApi = child.props("api") as { listApiKeys: () => Promise<unknown> };

    await keyApi.listApiKeys();

    expect(child.props("title")).toBe("我的模型 API 密钥");
    expect(child.props("embedded")).toBe(true);
    expect(customerApi.listApiKeys).toHaveBeenCalledOnce();
    wrapper.unmount();
  });
});

const apiKeyStub = {
  name: "PortalApiKeyWorkspace",
  props: ["api", "title", "embedded"],
  template: "<div />"
};
