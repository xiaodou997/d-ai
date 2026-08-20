import { flushPromises, mount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { PublicInvitation } from "@/api/types/platformPublic";

import RegisterView from "./RegisterView.vue";

const getInvitation = vi.hoisted(() => vi.fn());
const getPasswordPolicy = vi.hoisted(() => vi.fn());
const registerInvitation = vi.hoisted(() => vi.fn());

vi.mock("@/api/platformPublic", () => ({
  platformPublicApi: {
    getInvitation,
    getPasswordPolicy,
    registerInvitation
  }
}));

describe("public invitation registration", () => {
  beforeEach(() => {
    getInvitation.mockReset();
    getPasswordPolicy.mockReset();
    registerInvitation.mockReset();
    getInvitation.mockResolvedValue(activeInvitation());
    getPasswordPolicy.mockResolvedValue({
      minLength: 12,
      maxBytes: 72,
      requiredCharacterClasses: 3,
      description: "至少 12 个字符，至少包含三类字符"
    });
    registerInvitation.mockResolvedValue({ success: true, userId: "user-1", message: "registered" });
  });

  it("loads an active invitation and shows the registration form", async () => {
    const { wrapper } = await mountRegister();

    expect(getInvitation).toHaveBeenCalledWith("ABCD2345");
    expect(getPasswordPolicy).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain("示例工作区");
    expect(wrapper.find('input[name="username"]').exists()).toBe(true);
    expect(wrapper.find('input[name="password"]').exists()).toBe(true);
    expect(wrapper.find('input[name="confirmPassword"]').exists()).toBe(true);
  });

  it("explains why an unavailable invitation cannot be used", async () => {
    getInvitation.mockResolvedValue({
      ...activeInvitation(),
      status: "expired",
      canRegister: false,
      message: "邀请码已过期"
    });

    const { wrapper } = await mountRegister();

    expect(wrapper.text()).toContain("邀请码已过期");
    expect(wrapper.find('input[name="username"]').exists()).toBe(false);
  });

  it("registers the account with the invitation legal versions", async () => {
    const { wrapper } = await mountRegister();

    await wrapper.find('input[name="username"]').setValue("alice");
    await wrapper.find('input[name="password"]').setValue("Correct-Horse-47");
    await wrapper.find('input[name="confirmPassword"]').setValue("Correct-Horse-47");
    await wrapper.find('input[name="email"]').setValue("alice@example.com");
    await wrapper.find('input[name="accepted"]').setValue(true);
    await wrapper.find("form").trigger("submit");
    await flushPromises();

    expect(registerInvitation).toHaveBeenCalledWith("ABCD2345", {
      username: "alice",
      password: "Correct-Horse-47",
      email: "alice@example.com",
      termsVersion: "2026-07-19",
      privacyVersion: "2026-07-19"
    });
    expect(wrapper.text()).toContain("注册成功");
    expect(wrapper.text()).toContain("alice");
  });

  it("does not submit until the password confirmation and legal consent are valid", async () => {
    const { wrapper } = await mountRegister();

    await wrapper.find('input[name="username"]').setValue("alice");
    await wrapper.find('input[name="password"]').setValue("Correct-Horse-47");
    await wrapper.find('input[name="confirmPassword"]').setValue("different");
    await wrapper.find("form").trigger("submit");
    await flushPromises();

    expect(registerInvitation).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("两次输入的密码不一致");
  });
});

async function mountRegister(initialPath = "/register/ABCD2345") {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/register/:code", component: RegisterView },
      { path: "/login", component: { template: "<div>Login</div>" } }
    ]
  });
  await router.push(initialPath);
  await router.isReady();

  const wrapper = mount(RegisterView, { global: { plugins: [router] } });
  await flushPromises();
  return { router, wrapper };
}

function activeInvitation(): PublicInvitation {
  return {
    code: "ABCD2345",
    tenantName: "示例租户",
    customerSiteName: "示例工作区",
    description: "加入团队后即可使用 AI 服务。",
    status: "active",
    canRegister: true,
    message: "",
    legal: {
      termsUrl: "/legal/terms",
      termsVersion: "2026-07-19",
      privacyUrl: "/legal/privacy",
      privacyVersion: "2026-07-19"
    }
  };
}
