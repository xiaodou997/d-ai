import { flushPromises, shallowMount } from "@vue/test-utils";
import { ElMessageBox } from "element-plus";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import TenantUserDetailWorkspace from "./TenantUserDetailWorkspace.vue";

const platformApi = vi.hoisted(() => ({
  getUsers: vi.fn(),
  getUserRechargeRecords: vi.fn(),
  resetUserPassword: vi.fn(),
  deleteEndUser: vi.fn()
}));
const aiApi = vi.hoisted(() => ({
  listMyGroups: vi.fn(),
  listUserGroups: vi.fn(),
  listUserLimitPolicies: vi.fn()
}));
const usageApi = vi.hoisted(() => ({
  listTenantUsageRecords: vi.fn()
}));

vi.mock("@/api/platformTenant", () => ({ platformTenantApi: platformApi }));
vi.mock("@/api/aiTenant", () => ({ aiTenantApi: aiApi }));
vi.mock("@/features/ai/usage", () => ({ listTenantUsageRecords: usageApi.listTenantUsageRecords }));
vi.mock("@/platform/auth/activation", () => ({ showActivationCredential: vi.fn() }));

const user = {
  userId: "user-1",
  tenantId: "tenant-1",
  username: "alice",
  email: "alice@example.com",
  status: 1,
  credentialState: "active" as const,
  balanceUsd: 12.5,
  createdTime: 1_700_000_000_000
};

describe("TenantUserDetailWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    platformApi.getUsers.mockResolvedValue({ items: [user], total: 1 });
    platformApi.getUserRechargeRecords.mockResolvedValue({ items: [], total: 0 });
    platformApi.resetUserPassword.mockResolvedValue({ activationToken: "token", activationExpiresIn: 3600 });
    platformApi.deleteEndUser.mockResolvedValue({ success: true });
    aiApi.listMyGroups.mockResolvedValue({ items: [] });
    aiApi.listUserGroups.mockResolvedValue({ items: [] });
    aiApi.listUserLimitPolicies.mockResolvedValue({ items: [] });
    usageApi.listTenantUsageRecords.mockResolvedValue({ stats: {}, records: [] });
  });

  it("loads the route user and its cross-feature activity data", async () => {
    const { wrapper } = await mountWorkspace();

    expect(platformApi.getUsers).toHaveBeenCalledWith({ page: 1, size: 100 });
    expect(usageApi.listTenantUsageRecords).toHaveBeenCalledWith(expect.objectContaining({ user_id: "user-1", limit: 8 }));
    expect(aiApi.listUserGroups).toHaveBeenCalledWith("user-1");
    expect(aiApi.listUserLimitPolicies).toHaveBeenCalledWith("user-1");
    expect(platformApi.getUserRechargeRecords).toHaveBeenCalledWith({ page: 1, size: 8, username: "alice" });
    expect(wrapper.find("[data-testid='overview-user']").attributes("data-user-id")).toBe("user-1");
    wrapper.unmount();
  });

  it("keeps refresh and back navigation inside the workspace boundary", async () => {
    const { router, wrapper } = await mountWorkspace();

    await wrapper.get("[data-testid='overview-refresh']").trigger("click");
    await flushPromises();
    expect(platformApi.getUsers).toHaveBeenCalledTimes(2);

    await wrapper.get("[data-testid='overview-back']").trigger("click");
    await flushPromises();
    expect(router.currentRoute.value.fullPath).toBe("/tenant/users/directory");
    wrapper.unmount();
  });

  it("handles password reset and deletion from the detail actions", async () => {
    vi.spyOn(ElMessageBox, "confirm").mockResolvedValue("confirm" as never);
    const { router, wrapper } = await mountWorkspace();

    await wrapper.get("[data-testid='overview-reset-password']").trigger("click");
    await flushPromises();
    expect(platformApi.resetUserPassword).toHaveBeenCalledWith("user-1");

    await wrapper.get("[data-testid='overview-delete-user']").trigger("click");
    await flushPromises();
    expect(platformApi.deleteEndUser).toHaveBeenCalledWith("user-1");
    expect(router.currentRoute.value.fullPath).toBe("/tenant/users/directory");
    wrapper.unmount();
  });
});

async function mountWorkspace() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/tenant/users/directory/:userId", component: TenantUserDetailWorkspace },
      { path: "/tenant/users/directory", component: { template: "<div />" } }
    ]
  });
  await router.push("/tenant/users/directory/user-1");
  await router.isReady();

  const wrapper = shallowMount(TenantUserDetailWorkspace, {
    global: {
      plugins: [router],
      stubs: {
        PortalPagePanel: { template: "<main><slot /></main>" },
        UserOverviewHero: {
          props: ["userId", "user"],
          emits: ["back", "refresh", "reset-password", "delete-user"],
          template: "<div data-testid='overview-user' :data-user-id='user?.userId || userId'><button data-testid='overview-refresh' @click='$emit(\"refresh\")'>刷新</button><button data-testid='overview-back' @click='$emit(\"back\")'>返回</button><button data-testid='overview-reset-password' @click='$emit(\"reset-password\")'>重置密码</button><button data-testid='overview-delete-user' @click='$emit(\"delete-user\")'>删除用户</button></div>"
        },
        UserOverviewMetrics: { template: "<div />" },
        UserOverviewActivityGrid: { template: "<div />" },
        UserOverviewControlGrid: { template: "<div />" },
        UserEditDialog: { template: "<div />" },
        UserGroupPolicyDialog: { template: "<div />" }
      }
    }
  });
  await flushPromises();
  return { router, wrapper };
}
