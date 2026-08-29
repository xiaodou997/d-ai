import { flushPromises, mount } from "@vue/test-utils";
import ElementPlus from "element-plus";
import { createMemoryHistory, createRouter } from "vue-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import AdminUsersWorkspace from "./AdminUsersWorkspace.vue";

const api = vi.hoisted(() => ({
  listSystemAdmins: vi.fn(),
  createSystemAdmin: vi.fn(),
  updateSystemAdmin: vi.fn(),
  deleteSystemAdmin: vi.fn(),
  resetSystemAdminPassword: vi.fn()
}));

vi.mock("@/api/platformAdmin", () => ({ platformAdminApi: api }));
vi.mock("@/platform/auth/activation", () => ({ showActivationCredential: vi.fn() }));

const admin = {
  userId: "admin-1",
  username: "operator",
  email: "operator@example.com",
  status: 1,
  statusText: "正常",
  credentialState: "active",
  createdTime: 1_700_000_000_000
};

describe("AdminUsersWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listSystemAdmins.mockResolvedValue({ items: [admin], total: 1, page: 1, size: 20 });
  });

  afterEach(() => {
    document.body.innerHTML = "";
  });

  it("loads administrators through the typed list boundary", async () => {
    const { wrapper } = await mountWorkspace();

    expect(api.listSystemAdmins).toHaveBeenCalledWith({
      keyword: undefined,
      page: 1,
      size: 20
    });
    expect(wrapper.text()).toContain("operator");
    expect(wrapper.text()).toContain("正常");
    wrapper.unmount();
  });

  it("opens the create-admin workflow from the workspace action", async () => {
    const { wrapper } = await mountWorkspace();
    const createButton = wrapper.findAll("button").find((button) => button.text().includes("添加管理员"));
    expect(createButton).toBeDefined();

    await createButton!.trigger("click");
    await flushPromises();
    expect(document.body.textContent).toContain("创建管理员");
    wrapper.unmount();
  });
});

async function mountWorkspace() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/admin/system/admins", component: AdminUsersWorkspace }]
  });
  await router.push("/admin/system/admins");
  await router.isReady();
  const wrapper = mount(AdminUsersWorkspace, {
    attachTo: document.body,
    global: { plugins: [router, ElementPlus] }
  });
  await flushPromises();
  return { router, wrapper };
}
