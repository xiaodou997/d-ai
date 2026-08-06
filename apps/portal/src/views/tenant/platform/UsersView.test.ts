import { flushPromises, mount } from "@vue/test-utils";
import ElementPlus from "element-plus";
import { ElMessageBox } from "element-plus";
import { nextTick } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import UsersView from "./UsersView.vue";

const api = vi.hoisted(() => ({
  getUsers: vi.fn(),
  createEndUser: vi.fn(),
  updateUserStatus: vi.fn(),
  deleteEndUser: vi.fn(),
  resetUserPassword: vi.fn(),
  rechargeUser: vi.fn()
}));
const aiApi = vi.hoisted(() => ({
  listMyGroups: vi.fn(),
  listUserGroups: vi.fn(),
  listUserLimitPolicies: vi.fn()
}));

vi.mock("@/api/platformTenant", () => ({
  platformTenantApi: api
}));

vi.mock("@/api/aiTenant", () => ({
  aiTenantApi: aiApi
}));

const user = {
  userId: "user-1",
  tenantId: "tenant-1",
  username: "alice",
  email: "alice@example.com",
  status: 1,
  credits: 100,
  createdTime: 1_700_000_000_000
};

describe("tenant user management", () => {
  beforeEach(() => {
    api.getUsers.mockReset();
    api.getUsers.mockResolvedValue({ items: [], total: 0 });
    api.updateUserStatus.mockReset();
    api.updateUserStatus.mockResolvedValue({ message: "ok" });
    aiApi.listMyGroups.mockReset();
    aiApi.listMyGroups.mockResolvedValue({ items: [] });
    aiApi.listUserGroups.mockReset();
    aiApi.listUserGroups.mockResolvedValue({ items: [] });
    aiApi.listUserLimitPolicies.mockReset();
    aiApi.listUserLimitPolicies.mockResolvedValue({ items: [] });
  });

  afterEach(() => {
    document.body.innerHTML = "";
  });

  it("opens the create-user dialog while the AI policy drawer is closed", async () => {
    const wrapper = await mountUsers();

    await wrapper.get('[data-testid="create-user-button"]').trigger("click");
    await nextTick();

    expect(document.body.textContent).toContain("创建终端用户");
    wrapper.unmount();
  });

  it("opens AI policy as an action scoped to the selected user", async () => {
    api.getUsers.mockResolvedValue({ items: [user], total: 1 });
    const wrapper = await mountUsers();
    const policyButtons = wrapper.findAll("button").filter((button) => button.text() === "AI 策略");

    expect(policyButtons).toHaveLength(1);
    await policyButtons[0].trigger("click");
    await flushPromises();

    expect(document.body.textContent).toContain("alice · user-1");
    expect(document.body.textContent).toContain("策略仅作用于当前用户");
    wrapper.unmount();
  });

  it("changes user status through the confirmed switch", async () => {
    api.getUsers.mockResolvedValue({ items: [{ ...user }], total: 1 });
    vi.spyOn(ElMessageBox, "confirm").mockResolvedValue("confirm");
    const wrapper = await mountUsers();

    await wrapper.get('[aria-label="alice状态"]').trigger("click");
    await flushPromises();

    expect(api.updateUserStatus).toHaveBeenCalledWith("user-1", "disabled");
    wrapper.unmount();
  });
});

async function mountUsers() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/tenant/users/directory", component: UsersView }]
  });
  await router.push("/tenant/users/directory");
  await router.isReady();

  const wrapper = mount(UsersView, {
    attachTo: document.body,
    global: { plugins: [router, ElementPlus] }
  });
  await flushPromises();
  return wrapper;
}
