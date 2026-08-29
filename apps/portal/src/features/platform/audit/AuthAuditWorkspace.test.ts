import { flushPromises, mount } from "@vue/test-utils";
import ElementPlus from "element-plus";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import AuthAuditWorkspace from "./AuthAuditWorkspace.vue";

const getAuthAuditLogs = vi.hoisted(() => vi.fn());

vi.mock("@/api/platformAdmin", () => ({
  platformAdminApi: { getAuthAuditLogs }
}));

const log = {
  id: 1,
  eventType: "user_login",
  principalType: "user",
  decision: "deny",
  userId: "user-1",
  reasonCode: "invalid_password",
  createdAt: 1_700_000_000_000
};

describe("AuthAuditWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getAuthAuditLogs.mockResolvedValue({ items: [log], total: 1, page: 1, size: 20 });
  });

  it("loads authentication audit entries with typed filters", async () => {
    const { wrapper } = await mountWorkspace();

    expect(getAuthAuditLogs).toHaveBeenCalledWith({
      page: 1,
      size: 20,
      eventType: undefined,
      principalType: undefined,
      decision: undefined,
      userId: undefined
    });
    expect(wrapper.text()).toContain("invalid_password");
    expect(wrapper.text()).toContain("拒绝");
    wrapper.unmount();
  });

  it("reloads with a user filter when searching", async () => {
    const { wrapper } = await mountWorkspace();
    await wrapper.find('input[placeholder="User ID"]').setValue("user-1");
    await wrapper.find('input[placeholder="User ID"]').trigger("keyup", { key: "Enter" });
    await flushPromises();

    expect(getAuthAuditLogs).toHaveBeenLastCalledWith({
      page: 1,
      size: 20,
      eventType: undefined,
      principalType: undefined,
      decision: undefined,
      userId: "user-1"
    });
    wrapper.unmount();
  });
});

async function mountWorkspace() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/admin/identity/audit", component: AuthAuditWorkspace }]
  });
  await router.push("/admin/identity/audit");
  await router.isReady();
  const wrapper = mount(AuthAuditWorkspace, {
    attachTo: document.body,
    global: { plugins: [router, ElementPlus] }
  });
  await flushPromises();
  return { router, wrapper };
}
