import { flushPromises, shallowMount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { PortalGroupPricingApi } from "@/platform/ai/groups";
import CustomerGroupPricingWorkspace from "./CustomerGroupPricingWorkspace.vue";

const api = vi.hoisted(() => ({
  listMyGroups: vi.fn(),
  getMyGroupEffectivePrices: vi.fn()
}));
const platform = vi.hoisted(() => ({ notifyError: vi.fn() }));

vi.mock("@/api/aiCustomer", () => ({ aiCustomerApi: api }));
vi.mock("@/platform", () => platform);

const groupWorkspaceStub = {
  name: "PortalGroupPricingWorkspace",
  props: ["api", "breadcrumbs", "description", "capabilityOptions"],
  template: "<div><slot name='actions' /></div>"
};

describe("CustomerGroupPricingWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listMyGroups.mockResolvedValue({ items: [], total: 0 });
    api.getMyGroupEffectivePrices.mockResolvedValue({ group_id: "group-1", effective_user_multiplier: 1, items: [] });
  });

  it("binds the customer group pricing facade and display contract", async () => {
    const wrapper = await mountWorkspace();
    const workspace = wrapper.findComponent({ name: "PortalGroupPricingWorkspace" });
    const pricingApi = workspace.props("api") as PortalGroupPricingApi;

    await pricingApi.listGroups();
    await pricingApi.getGroupEffectivePrices("group-1");

    expect(api.listMyGroups).toHaveBeenCalledOnce();
    expect(api.getMyGroupEffectivePrices).toHaveBeenCalledWith("group-1");
    expect(workspace.props("description")).toContain("USD");
    expect(workspace.props("capabilityOptions")).toEqual(expect.arrayContaining([{ label: "文本对话", value: "chat" }]));
    wrapper.unmount();
  });

  it("keeps the API key navigation action inside the feature", async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/customer/services/models", component: CustomerGroupPricingWorkspace },
        { path: "/customer/developer/keys", component: { template: "<div />" } }
      ]
    });
    await router.push("/customer/services/models");
    await router.isReady();
    const wrapper = shallowMount(CustomerGroupPricingWorkspace, {
      global: {
        plugins: [router],
        stubs: {
          PortalGroupPricingWorkspace: groupWorkspaceStub,
          "el-button": { template: "<button @click=\"$emit('click')\"><slot /></button>" }
        }
      }
    });

    await wrapper.find("button").trigger("click");
    await flushPromises();

    expect(router.currentRoute.value.fullPath).toBe("/customer/developer/keys");
    wrapper.unmount();
  });
});

async function mountWorkspace() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/customer/services/models", component: CustomerGroupPricingWorkspace }]
  });
  await router.push("/customer/services/models");
  await router.isReady();
  return shallowMount(CustomerGroupPricingWorkspace, {
    global: {
      plugins: [router],
      stubs: {
        PortalGroupPricingWorkspace: groupWorkspaceStub,
        "el-button": { template: "<button><slot /></button>" }
      }
    }
  });
}
