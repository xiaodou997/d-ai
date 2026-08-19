import { flushPromises, mount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { DsButton, DsSwitch } from "@/shared/ui";
import ProxyEgressView from "./ProxyEgressView.vue";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  setEnabled: vi.fn(),
  listProxyNodes: vi.fn(),
  createProxyNode: vi.fn(),
  updateProxyNode: vi.fn(),
  deleteProxyNode: vi.fn()
}));

vi.mock("@/api/systemModules", () => ({ systemModulesApi: api }));

const proxyModule = {
  name: "proxy_egress",
  displayName: "代理出口节点",
  description: "代理出口",
  category: "integration",
  adminRoute: "/admin/proxy-nodes",
  order: 20,
  available: true,
  enabled: false,
  active: false,
  configValidated: true,
  health: "disabled"
};

const node = {
  id: "node-1",
  name: "新加坡出口",
  proxyType: "http",
  endpoint: "http://proxy.example.com:8080",
  username: "proxy-user",
  weight: 10,
  status: "active",
  healthStatus: "healthy",
  lastCheckedAt: "2026-08-19T08:00:00Z",
  createdAt: "2026-08-19T08:00:00Z",
  updatedAt: "2026-08-19T08:00:00Z"
};

describe("ProxyEgressView", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue([proxyModule]);
    api.listProxyNodes.mockResolvedValue([node]);
    api.setEnabled.mockResolvedValue({ ...proxyModule, enabled: true, active: true });
    api.updateProxyNode.mockResolvedValue(node);
  });

  afterEach(() => {
    document.body.innerHTML = "";
  });

  it("loads proxy status and toggles the module", async () => {
    const wrapper = await mountView();

    expect(wrapper.text()).toContain("新加坡出口");
    expect(wrapper.text()).toContain("可调度");
    wrapper.findComponent(DsSwitch).vm.$emit("update:modelValue", true);
    await flushPromises();

    expect(api.setEnabled).toHaveBeenCalledWith("proxy_egress", true);
    wrapper.unmount();
  });

  it("keeps the current password when an existing node is saved without a new password", async () => {
    const wrapper = await mountView();
    await wrapper.find('[aria-label="编辑代理节点"]').trigger("click");
    await flushPromises();

    const saveButton = wrapper.findAllComponents(DsButton).find((button) => button.text().includes("保存节点"));
    expect(saveButton).toBeDefined();
    await saveButton!.trigger("click");
    await flushPromises();

    expect(api.updateProxyNode).toHaveBeenCalledWith("node-1", expect.objectContaining({
      name: "新加坡出口",
      password: undefined,
      weight: 10
    }));
    wrapper.unmount();
  });
});

async function mountView() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/admin/proxy-nodes", component: ProxyEgressView }]
  });
  await router.push("/admin/proxy-nodes");
  await router.isReady();
  const wrapper = mount(ProxyEgressView, { attachTo: document.body, global: { plugins: [router] } });
  await flushPromises();
  return wrapper;
}
