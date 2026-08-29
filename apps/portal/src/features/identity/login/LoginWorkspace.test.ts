import { flushPromises, mount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import LoginWorkspace from "./LoginWorkspace.vue";

const login = vi.hoisted(() => vi.fn());

vi.mock("@/stores/auth", () => ({
  useAuthStore: () => ({ login })
}));

describe("unified Portal login", () => {
  beforeEach(() => {
    login.mockReset();
    login.mockResolvedValue(undefined);
  });

  it("shows one username and password form instead of portal selectors", async () => {
    const { wrapper } = await mountLogin();

    expect(wrapper.find('input[name="username"]').exists()).toBe(true);
    expect(wrapper.find('input[name="password"]').exists()).toBe(true);
    expect(wrapper.findAll('button[type="submit"]')).toHaveLength(1);
    expect(wrapper.findAll(".login-option")).toHaveLength(0);
    expect(wrapper.text()).not.toContain("选择登录入口");
    expect(wrapper.text()).not.toContain("统一 AI 服务平台");
    expect(wrapper.text()).not.toContain("使用账号登录，系统会根据你的身份和权限展示对应的工作区。");
    expect(wrapper.find("#login-title").exists()).toBe(false);
  });

  it("authenticates with the entered credentials and opens the requested workspace", async () => {
    const { router, wrapper } = await mountLogin("/login?redirect=/workspace");

    await wrapper.find('input[name="username"]').setValue("alice");
    await wrapper.find('input[name="password"]').setValue("secret");
    await wrapper.find("form").trigger("submit");
    await flushPromises();

    expect(login).toHaveBeenCalledWith("alice", "secret");
    expect(router.currentRoute.value.path).toBe("/workspace");
  });
});

async function mountLogin(initialPath = "/login") {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/login", component: LoginWorkspace },
        { path: "/workspace", component: { template: "<div>Workspace</div>" } }
      ]
    });
    await router.push(initialPath);
    await router.isReady();

    const wrapper = mount(LoginWorkspace, { global: { plugins: [router] } });
    return { router, wrapper };
}
