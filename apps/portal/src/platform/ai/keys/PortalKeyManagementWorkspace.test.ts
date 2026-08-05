import { flushPromises, mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import PortalKeyManagementWorkspace from "./PortalKeyManagementWorkspace.vue";

const PortalPagePanelStub = {
  name: "PortalPagePanel",
  template: "<section><slot name='actions' /><slot /></section>"
};

const DsTabsStub = {
  name: "DsTabs",
  props: ["tabs", "modelValue"],
  template: "<nav data-test='key-tabs'>{{ modelValue }}</nav>"
};

describe("PortalKeyManagementWorkspace", () => {
  it("shows only model API keys when application keys are disabled", async () => {
    const wrapper = mount(PortalKeyManagementWorkspace, {
      props: {
        activeTab: "application",
        showApplicationKeys: false
      },
      slots: {
        api: "<div data-test='api-keys'>API keys</div>",
        application: "<div data-test='application-keys'>Application keys</div>"
      },
      global: {
        stubs: {
          DsTabs: DsTabsStub,
          ElButton: true,
          PortalPagePanel: PortalPagePanelStub
        }
      }
    });
    await flushPromises();

    expect(wrapper.find("[data-test='key-tabs']").exists()).toBe(false);
    expect(wrapper.find("[data-test='api-keys']").exists()).toBe(true);
    expect(wrapper.find("[data-test='application-keys']").exists()).toBe(false);
    expect(wrapper.emitted("update:activeTab")).toContainEqual(["api"]);
  });

  it("keeps the existing application key workspace enabled by default", () => {
    const wrapper = mount(PortalKeyManagementWorkspace, {
      props: { activeTab: "application" },
      slots: {
        api: "<div data-test='api-keys'>API keys</div>",
        application: "<div data-test='application-keys'>Application keys</div>"
      },
      global: {
        stubs: {
          DsTabs: DsTabsStub,
          ElButton: true,
          PortalPagePanel: PortalPagePanelStub
        }
      }
    });

    expect(wrapper.find("[data-test='key-tabs']").exists()).toBe(true);
    expect(wrapper.find("[data-test='api-keys']").exists()).toBe(false);
    expect(wrapper.find("[data-test='application-keys']").exists()).toBe(true);
  });
});
