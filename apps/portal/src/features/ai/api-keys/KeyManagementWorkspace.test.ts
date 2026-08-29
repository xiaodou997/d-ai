import { shallowMount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import CustomerKeyManagementWorkspace from "./CustomerKeyManagementWorkspace.vue";
import TenantKeyManagementWorkspace from "./TenantKeyManagementWorkspace.vue";

const managementWorkspaceStub = {
  name: "PortalKeyManagementWorkspace",
  props: ["title", "eyebrow"],
  template: "<div><slot name='api' /></div>"
};
const apiKeysStub = {
  props: { embedded: Boolean },
  template: "<div />"
};

describe("API key management workspaces", () => {
  it("composes the tenant API key feature and management shell", () => {
    const wrapper = shallowMount(TenantKeyManagementWorkspace, {
      global: {
        stubs: {
          PortalKeyManagementWorkspace: managementWorkspaceStub,
          TenantApiKeysWorkspace: { ...apiKeysStub, name: "TenantApiKeysWorkspace" }
        }
      }
    });

    const shell = wrapper.findComponent({ name: "PortalKeyManagementWorkspace" });
    const api = wrapper.findComponent({ name: "TenantApiKeysWorkspace" });
    expect(shell.props("title")).toBe("API 密钥");
    expect(shell.props("eyebrow")).toBe("智能服务 / 开发接入");
    expect(api.props("embedded")).toBe(true);
    wrapper.unmount();
  });

  it("composes the customer API key feature without tenant-only actions", () => {
    const wrapper = shallowMount(CustomerKeyManagementWorkspace, {
      global: {
        stubs: {
          PortalKeyManagementWorkspace: managementWorkspaceStub,
          CustomerApiKeysWorkspace: { ...apiKeysStub, name: "CustomerApiKeysWorkspace" }
        }
      }
    });

    const api = wrapper.findComponent({ name: "CustomerApiKeysWorkspace" });
    expect(api.props("embedded")).toBe(true);
    wrapper.unmount();
  });
});
