import { flushPromises, mount } from "@vue/test-utils";
import ElementPlus from "element-plus";
import { beforeEach, describe, expect, it, vi } from "vitest";

import CustomerProfileWorkspace from "./CustomerProfileWorkspace.vue";

const auth = vi.hoisted(() => ({
  username: "alice",
  userInfo: { sub: "user-1" },
  logout: vi.fn()
}));
const api = vi.hoisted(() => ({
  updateProfile: vi.fn(),
  changePassword: vi.fn()
}));
const getPasswordPolicy = vi.hoisted(() => vi.fn());

vi.mock("@/stores/auth", () => ({ useAuthStore: () => auth }));
vi.mock("@/api/platformCustomer", () => ({ platformCustomerApi: api }));
vi.mock("@/api/platformPublic", () => ({
  platformPublicApi: { getPasswordPolicy }
}));

describe("CustomerProfileWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    auth.logout.mockResolvedValue(true);
    api.updateProfile.mockResolvedValue({ success: true });
    api.changePassword.mockResolvedValue({ success: true });
    getPasswordPolicy.mockResolvedValue({
      minLength: 12,
      maxBytes: 72,
      requiredCharacterClasses: 3,
      description: "至少 12 个字符，至少包含三类字符"
    });
  });

  it("renders the authenticated customer identity and password policy", async () => {
    const wrapper = await mountWorkspace();

    expect(wrapper.text()).toContain("alice");
    expect(wrapper.text()).toContain("user-1");
    expect(wrapper.text()).toContain("至少 12 个字符，至少包含三类字符");
    expect(getPasswordPolicy).toHaveBeenCalledOnce();
    wrapper.unmount();
  });

  it("submits changed profile fields and logs the customer out", async () => {
    const wrapper = await mountWorkspace();
    await wrapper.find('input[placeholder="请输入用户名"]').setValue("alice-new");
    await wrapper.find('input[placeholder="请输入邮箱（可留空）"]').setValue("alice@example.com");
    await wrapper.findAll("button").find((button) => button.text().includes("保存资料"))!.trigger("click");
    await flushPromises();

    expect(api.updateProfile).toHaveBeenCalledWith({ username: "alice-new", email: "alice@example.com" });
    expect(auth.logout).toHaveBeenCalledOnce();
    wrapper.unmount();
  });
});

async function mountWorkspace() {
  const wrapper = mount(CustomerProfileWorkspace, {
    attachTo: document.body,
    global: { plugins: [ElementPlus] }
  });
  await flushPromises();
  return wrapper;
}
