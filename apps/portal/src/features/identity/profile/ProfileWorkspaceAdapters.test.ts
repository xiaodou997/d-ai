import { shallowMount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import AdminProfileWorkspace from "./AdminProfileWorkspace.vue";
import TenantProfileWorkspace from "./TenantProfileWorkspace.vue";

const authStore = vi.hoisted(() => ({
  accessToken: "access-token",
  username: "alice",
  tenantName: "Acme",
  userInfo: { sub: "user-1", mfaEnabled: true },
  logout: vi.fn().mockResolvedValue(true)
}));
const adminApi = vi.hoisted(() => ({ changePassword: vi.fn() }));
const tenantApi = vi.hoisted(() => ({ changePassword: vi.fn(), updateProfile: vi.fn() }));

vi.mock("@/stores/auth", () => ({ useAuthStore: () => authStore }));
vi.mock("@/api/platformAdmin", () => ({ platformAdminApi: adminApi }));
vi.mock("@/api/platformTenant", () => ({ platformTenantApi: tenantApi }));

const profileWorkspaceStub = {
  name: "PortalProfileWorkspace",
  props: [
    "fields",
    "changePassword",
    "afterPasswordChanged",
    "updateProfile",
    "initialUsername",
    "afterProfileChanged",
    "mfa"
  ],
  template: "<div />"
};

describe("profile workspace adapters", () => {
  beforeEach(() => vi.clearAllMocks());

  it("binds admin identity fields, password API and MFA state", () => {
    const wrapper = shallowMount(AdminProfileWorkspace, { global: { stubs: { PortalProfileWorkspace: profileWorkspaceStub } } });
    const workspace = wrapper.findComponent({ name: "PortalProfileWorkspace" });

    expect(workspace.props("fields")).toEqual([
      { label: "用户名", value: "alice", tone: "strong" },
      { label: "用户 ID", value: "user-1", tone: "mono" }
    ]);
    expect(workspace.props("changePassword")).toBe(adminApi.changePassword);
    expect((workspace.props("mfa") as { enabled: boolean }).enabled).toBe(true);
    wrapper.unmount();
  });

  it("binds tenant identity fields and profile update API", () => {
    const wrapper = shallowMount(TenantProfileWorkspace, { global: { stubs: { PortalProfileWorkspace: profileWorkspaceStub } } });
    const workspace = wrapper.findComponent({ name: "PortalProfileWorkspace" });

    expect(workspace.props("fields")).toEqual([
      { label: "用户名", value: "alice", tone: "strong" },
      { label: "用户 ID", value: "user-1", tone: "mono" },
      { label: "所属租户", value: "Acme" }
    ]);
    expect(workspace.props("changePassword")).toBe(tenantApi.changePassword);
    expect(workspace.props("updateProfile")).toBe(tenantApi.updateProfile);
    expect(workspace.props("initialUsername")).toBe("alice");
    wrapper.unmount();
  });
});
