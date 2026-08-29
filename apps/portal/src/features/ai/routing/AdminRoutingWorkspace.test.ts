import { defineComponent } from "vue";
import { flushPromises, shallowMount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import AdminRoutingWorkspace from "./AdminRoutingWorkspace.vue";

const api = vi.hoisted(() => ({
  getRouteWeights: vi.fn(),
  putRouteWeights: vi.fn()
}));

vi.mock("@/api/aiAdmin", () => ({ aiAdminApi: api }));
vi.mock("element-plus", () => ({
  ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn() }
}));

const PanelStub = defineComponent({ template: "<div><slot /></div>" });
const CardStub = defineComponent({
  template: "<section><slot name=\"header\" /><slot name=\"actions\" /><slot /></section>"
});
const ButtonStub = defineComponent({
  emits: ["click"],
  template: "<button @click=\"$emit('click')\"><slot /></button>"
});
const PassThroughStub = defineComponent({ template: "<div><slot /></div>" });

const global = {
  directives: { loading: {} },
  stubs: {
    PortalPagePanel: PanelStub,
    PortalContentCard: CardStub,
    ElButton: ButtonStub,
    ElForm: PassThroughStub,
    ElFormItem: PassThroughStub,
    ElSlider: PassThroughStub,
    ElInputNumber: PassThroughStub
  }
};

describe("AdminRoutingWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getRouteWeights.mockResolvedValue({
      scope: "global",
      weights: { cost: 0.4, latency: 0.3, load: 0.2, health: 0.1 }
    });
    api.putRouteWeights.mockResolvedValue({
      scope: "global",
      weights: { cost: 0.4, latency: 0.3, load: 0.2, health: 0.1 }
    });
  });

  it("loads global route weights and keeps the save action in the workspace", async () => {
    const wrapper = shallowMount(AdminRoutingWorkspace, { global });
    await flushPromises();

    expect(api.getRouteWeights).toHaveBeenCalledWith("global");
    expect(wrapper.text()).toContain("多维评分路由权重");
    expect(wrapper.findAll("button").some((button) => button.text() === "保存权重")).toBe(true);

    wrapper.unmount();
  });

  it("saves the normalized four-dimensional weights through the admin API", async () => {
    const wrapper = shallowMount(AdminRoutingWorkspace, { global });
    await flushPromises();

    const saveButton = wrapper.findAll("button").find((button) => button.text() === "保存权重");
    expect(saveButton).toBeDefined();
    await saveButton!.trigger("click");
    await flushPromises();

    expect(api.putRouteWeights).toHaveBeenCalledWith("global", {
      cost: 0.4,
      latency: 0.3,
      load: 0.2,
      health: 0.1
    });

    wrapper.unmount();
  });
});
