import { flushPromises, shallowMount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { describe, expect, it } from "vitest";

import TenantSubscriptionWorkspace from "./TenantSubscriptionWorkspace.vue";

describe("TenantSubscriptionWorkspace", () => {
  it("lazily mounts subscription panels while syncing the tab query", async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/tenant/ai/subscriptions", component: TenantSubscriptionWorkspace }]
    });
    await router.push("/tenant/ai/subscriptions?tab=plans");
    await router.isReady();

    const wrapper = shallowMount(TenantSubscriptionWorkspace, {
      global: {
        plugins: [router],
        stubs: {
          PortalPagePanel: { template: "<main><slot /></main>" },
          DsTabs: {
            emits: ["update:modelValue"],
            template: "<button data-testid='orders-tab' @click='$emit(\"update:modelValue\", \"orders\")'>订单记录</button>"
          },
          SubscriptionPlansPanel: { template: "<div data-testid='plans-panel' />" },
          SubscriptionInstancesPanel: { template: "<div data-testid='instances-panel' />" },
          SubscriptionOrdersPanel: { template: "<div data-testid='orders-panel' />" }
        }
      }
    });
    await flushPromises();

    expect(wrapper.find("[data-testid='plans-panel']").exists()).toBe(true);
    expect(wrapper.find("[data-testid='instances-panel']").exists()).toBe(false);
    expect(wrapper.find("[data-testid='orders-panel']").exists()).toBe(false);

    await wrapper.get("[data-testid='orders-tab']").trigger("click");
    await flushPromises();

    expect(router.currentRoute.value.query.tab).toBe("orders");
    expect(wrapper.find("[data-testid='orders-panel']").exists()).toBe(true);
    expect(wrapper.find("[data-testid='instances-panel']").exists()).toBe(false);
    wrapper.unmount();
  });
});
