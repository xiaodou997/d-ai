import { mount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { describe, expect, it, vi } from "vitest";

import PortalWorkspaceLayout from "./PortalWorkspaceLayout.vue";

vi.mock("@/stores/auth", () => ({
  useAuthStore: () => ({ userType: 1 })
}));

describe("PortalWorkspaceLayout navigation", () => {
  it("hides duplicate tabs when admin organization views are separate sidebar menus", async () => {
    const wrapper = await mountWorkspace("admin-organization-workspace");

    expect(wrapper.find(".portal-workspace-layout__tabs").exists()).toBe(false);
  });
});

async function mountWorkspace(moduleId: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/", component: { template: "<div />" } }]
  });
  await router.push("/");
  await router.isReady();

  return mount(PortalWorkspaceLayout, {
    props: { moduleId },
    global: { plugins: [router] }
  });
}
