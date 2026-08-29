import { shallowMount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import AiDocsWorkspace from "./AiDocsWorkspace.vue";

const authStore = vi.hoisted(() => ({ userType: 4 }));

vi.mock("@/stores/auth", () => ({ useAuthStore: () => authStore }));

const docsPageStub = {
  name: "PortalAiDocsPage",
  props: ["baseUrl", "scope", "section"],
  template: "<div />"
};

describe("AiDocsWorkspace", () => {
  beforeEach(() => {
    authStore.userType = 4;
  });

  it("uses the customer documentation scope for terminal users", () => {
    const wrapper = shallowMount(AiDocsWorkspace, {
      props: { section: "tooling" },
      global: { stubs: { PortalAiDocsPage: docsPageStub } }
    });
    expect(wrapper.findComponent({ name: "PortalAiDocsPage" }).props("scope")).toBe("user");
    wrapper.unmount();
  });

  it("uses the tenant documentation scope for tenant-side users", () => {
    authStore.userType = 3;
    const wrapper = shallowMount(AiDocsWorkspace, {
      props: { section: "tooling" },
      global: { stubs: { PortalAiDocsPage: docsPageStub } }
    });
    expect(wrapper.findComponent({ name: "PortalAiDocsPage" }).props("scope")).toBe("tenant");
    wrapper.unmount();
  });
});
