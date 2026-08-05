import { defineComponent } from "vue";
import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import type { ServiceRegistryDetail } from "../../../types/admin";
import ServiceDetailsDrawer from "./ServiceDetailsDrawer.vue";

const DrawerStub = defineComponent({
  props: { modelValue: Boolean },
  emits: ["update:modelValue"],
  template: `<section><slot name="header" /><slot /></section>`
});

const ButtonStub = defineComponent({
  props: { disabled: Boolean, loading: Boolean },
  emits: ["click"],
  template: `<button :disabled="disabled" @click="$emit('click')"><slot /></button>`
});

const service: ServiceRegistryDetail = {
  id: 1,
  serviceId: "uni-ai-api",
  displayName: "AI Gateway",
  status: "active",
  portalEnabled: false,
  sourceCount: 0,
  instanceCount: 0,
  onlineInstances: 0,
  createdAt: "2026-07-11T00:00:00Z",
  updatedAt: "2026-07-11T00:00:00Z",
  sources: [],
  instances: []
};

describe("ServiceDetailsDrawer", () => {
  it("exposes independent portal and service-status actions", async () => {
    const wrapper = mount(ServiceDetailsDrawer, {
      props: { modelValue: true, service, loading: false, portalModuleLabel: "智能服务" },
      global: {
        directives: { loading: {} },
        stubs: {
          "el-drawer": DrawerStub,
          "el-button": ButtonStub,
          ServiceInstancesPanel: true
        }
      }
    });

    const portalButton = wrapper.findAll("button").find((button) => button.text() === "开启门户入口");
    const statusButton = wrapper.findAll("button").find((button) => button.text() === "停用注册续签");
    expect(portalButton).toBeDefined();
    expect(statusButton).toBeDefined();

    await portalButton!.trigger("click");
    await statusButton!.trigger("click");

    expect(wrapper.emitted("toggle-portal")).toHaveLength(1);
    expect(wrapper.emitted("toggle-status")).toHaveLength(1);
    expect(wrapper.text()).toContain("未开放");
    expect(wrapper.text()).toContain("在线实例");
    expect(wrapper.text()).toContain("0 个");
    expect(wrapper.text()).toContain("前端已接入 · 智能服务");
  });

  it("identifies a service without a frontend module", () => {
    const wrapper = mount(ServiceDetailsDrawer, {
      props: {
        modelValue: true,
        service: { ...service, serviceId: "report-service", displayName: "Report Service" },
        loading: false
      },
      global: {
        directives: { loading: {} },
        stubs: {
          "el-drawer": DrawerStub,
          "el-button": ButtonStub,
          ServiceInstancesPanel: true
        }
      }
    });

    expect(wrapper.text()).toContain("前端未接入");
  });
});
