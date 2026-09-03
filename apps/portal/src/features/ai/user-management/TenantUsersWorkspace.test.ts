import { flushPromises, mount } from "@vue/test-utils";
import ElementPlus from "element-plus";
import { ElMessageBox } from "element-plus";
import { nextTick } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import TenantUsersWorkspace from "./TenantUsersWorkspace.vue";

const api = vi.hoisted(() => ({
  getUsers: vi.fn(),
  createEndUser: vi.fn(),
  updateEndUser: vi.fn(),
  updateUserStatus: vi.fn(),
  rechargeUser: vi.fn()
}));
const aiApi = vi.hoisted(() => ({
  listMyGroups: vi.fn(),
  listUserGroups: vi.fn(),
  listUserLimitPolicies: vi.fn(),
  upsertUserLimitPolicy: vi.fn()
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
  phone: "13800000000",
  internalNote: "重点客户",
  status: 1,
  credentialState: "active" as const,
  balanceUsd: 12.5,
  createdTime: 1_700_000_000_000
};

describe("tenant user management", () => {
  beforeEach(() => {
    api.getUsers.mockReset();
    api.getUsers.mockResolvedValue({ items: [], total: 0 });
    api.updateUserStatus.mockReset();
    api.updateUserStatus.mockResolvedValue({ message: "ok" });
    api.updateEndUser.mockReset();
    api.updateEndUser.mockResolvedValue({ message: "ok" });
    aiApi.listMyGroups.mockReset();
    aiApi.listMyGroups.mockResolvedValue({ items: [] });
    aiApi.listUserGroups.mockReset();
    aiApi.listUserGroups.mockResolvedValue({ items: [] });
    aiApi.listUserLimitPolicies.mockReset();
    aiApi.listUserLimitPolicies.mockResolvedValue({ items: [] });
    aiApi.upsertUserLimitPolicy.mockReset();
    aiApi.upsertUserLimitPolicy.mockResolvedValue({});
  });

  afterEach(() => {
    document.body.innerHTML = "";
  });

  it("opens the create-user dialog", async () => {
    const wrapper = await mountUsers();

    await wrapper.get('[data-testid="create-user-button"]').trigger("click");
    await nextTick();

    expect(document.body.textContent).toContain("创建终端用户");
    wrapper.unmount();
  });

  it("opens group policy as an action scoped to the selected user", async () => {
    api.getUsers.mockResolvedValue({ items: [user], total: 1 });
    aiApi.listMyGroups.mockResolvedValue({
      items: [{
        id: "group-1",
        name: "标准分组",
        default_user_multiplier: 1,
        user_default_visible: true
      }]
    });
    const wrapper = await mountUsers();
    const policyButtons = wrapper.findAll("button").filter((button) => button.text() === "分组策略");

    expect(policyButtons).toHaveLength(1);
    await policyButtons[0].trigger("click");
    await flushPromises();

    expect(document.body.textContent).toContain("分组策略 · alice");
    expect(document.body.textContent).toContain("租户默认");
    expect(document.body.textContent).toContain("单独配置");
    expect(document.body.textContent).toContain("沿用租户默认");
    wrapper.unmount();
  });

  it("shows the user balance and keeps uncommon account actions out of the list", async () => {
    api.getUsers.mockResolvedValue({ items: [{ ...user }], total: 1 });
    const wrapper = await mountUsers();

    expect(wrapper.text()).toContain("$12.50");
    expect(wrapper.findAll("button").some((button) => button.text() === "编辑")).toBe(false);
    expect(wrapper.findAll("button").some((button) => button.text() === "重置密码")).toBe(false);
    expect(wrapper.findAll("button").some((button) => button.text() === "删除")).toBe(false);
    wrapper.unmount();
  });

  it("changes user status through the confirmed switch", async () => {
    api.getUsers.mockResolvedValue({ items: [{ ...user }], total: 1 });
    vi.spyOn(ElMessageBox, "confirm").mockResolvedValue("confirm" as any);
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
    routes: [{ path: "/tenant/users/directory", component: TenantUsersWorkspace }]
  });
  await router.push("/tenant/users/directory");
  await router.isReady();

  const wrapper = mount(TenantUsersWorkspace, {
    attachTo: document.body,
    global: { plugins: [router, ElementPlus] }
  });
  await flushPromises();
  return wrapper;
}
