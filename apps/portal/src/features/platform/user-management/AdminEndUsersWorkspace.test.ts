import { flushPromises, mount } from "@vue/test-utils";
import ElementPlus, { ElMessageBox } from "element-plus";
import { createMemoryHistory, createRouter } from "vue-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import AdminEndUsersWorkspace from "./AdminEndUsersWorkspace.vue";

const api = vi.hoisted(() => ({
  listEndUsers: vi.fn(),
  updateEndUserStatus: vi.fn()
}));

vi.mock("@/api/platformAdmin", () => ({ platformAdminApi: api }));

const user = {
  userId: "user-1",
  tenantId: "tenant-1",
  tenantName: "示例租户",
  username: "alice",
  email: "alice@example.com",
  status: 1,
  credentialState: "active",
  balanceUsd: 12.5,
  createdTime: 1_700_000_000_000,
  lastLoginTime: 1_700_000_100_000
};

describe("AdminEndUsersWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listEndUsers.mockResolvedValue({ items: [], total: 0, page: 1, size: 20 });
    api.updateEndUserStatus.mockResolvedValue({ message: "ok" });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    document.body.innerHTML = "";
  });

  it("loads users and navigates to the owning tenant", async () => {
    api.listEndUsers.mockResolvedValue({ items: [user], total: 1, page: 1, size: 20 });
    const { router, wrapper } = await mountWorkspace();

    expect(api.listEndUsers).toHaveBeenCalledWith({
      tenantName: undefined,
      username: undefined,
      status: undefined,
      page: 1,
      size: 20
    });
    expect(wrapper.text()).toContain("alice");
    expect(wrapper.text()).toContain("示例租户");

    await wrapper.get(".endusers-tenant-link").trigger("click");
    await flushPromises();
    expect(router.currentRoute.value.path).toBe("/admin/organization/tenants/tenant-1");
    wrapper.unmount();
  });

  it("updates a controllable user's status after confirmation", async () => {
    api.listEndUsers.mockResolvedValue({ items: [{ ...user }], total: 1, page: 1, size: 20 });
    vi.spyOn(ElMessageBox, "confirm").mockResolvedValue("confirm" as never);
    const { wrapper } = await mountWorkspace();

    await wrapper.get('[aria-label="alice状态"]').trigger("click");
    await flushPromises();
    expect(api.updateEndUserStatus).toHaveBeenCalledWith("user-1", "disabled");
    wrapper.unmount();
  });
});

async function mountWorkspace() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/admin/organization/users", component: AdminEndUsersWorkspace },
      { path: "/admin/organization/tenants/:id", component: { template: "<div>Tenant</div>" } }
    ]
  });
  await router.push("/admin/organization/users");
  await router.isReady();
  const wrapper = mount(AdminEndUsersWorkspace, {
    attachTo: document.body,
    global: {
      plugins: [router, ElementPlus],
      stubs: { AccountOverviewDrawer: { template: "<div />" } }
    }
  });
  await flushPromises();
  return { router, wrapper };
}
