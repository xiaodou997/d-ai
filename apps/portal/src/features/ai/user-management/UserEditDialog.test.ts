import { flushPromises, mount } from "@vue/test-utils";
import ElementPlus from "element-plus";
import { beforeEach, describe, expect, it, vi } from "vitest";

import UserEditDialog from "./UserEditDialog.vue";

const platformApi = vi.hoisted(() => ({
  updateEndUser: vi.fn()
}));
const aiApi = vi.hoisted(() => ({
  listUserLimitPolicies: vi.fn(),
  upsertUserLimitPolicy: vi.fn()
}));

vi.mock("@/api/platformTenant", () => ({
  platformTenantApi: platformApi
}));

vi.mock("@/api/aiTenant", () => ({
  aiTenantApi: aiApi
}));

const user = {
  userId: "user-1",
  tenantId: "tenant-1",
  username: "alice",
  email: "old@example.com",
  phone: "13800000000",
  internalNote: "旧备注",
  status: 1,
  credits: 0,
  createdTime: 1_700_000_000_000
};

describe("UserEditDialog", () => {
  beforeEach(() => {
    platformApi.updateEndUser.mockReset();
    platformApi.updateEndUser.mockResolvedValue({ message: "ok" });
    aiApi.listUserLimitPolicies.mockReset();
    aiApi.listUserLimitPolicies.mockResolvedValue({
      items: [{ concurrency_limit: 3, status: "active" }]
    });
    aiApi.upsertUserLimitPolicy.mockReset();
    aiApi.upsertUserLimitPolicy.mockResolvedValue({});
  });

  it("saves contact fields, tenant-only note, and concurrency", async () => {
    const wrapper = mount(UserEditDialog, {
      props: { open: true, user },
      global: {
        plugins: [ElementPlus],
        stubs: { teleport: true }
      }
    });
    await flushPromises();

    const contactInputs = wrapper.findAll('input[placeholder="未填写"]');
    expect(contactInputs).toHaveLength(2);
    await contactInputs[0].setValue("new@example.com");
    await contactInputs[1].setValue("13900000000");
    await wrapper.get("textarea").setValue("  新内部备注  ");
    const concurrencyInput = wrapper.get('input[placeholder="不填表示不限"]');
    expect((concurrencyInput.element as HTMLInputElement).value).toBe("3");
    await concurrencyInput.setValue("5");

    const saveButtons = wrapper.findAll("button").filter((button) => button.text() === "保存");
    expect(saveButtons).toHaveLength(1);
    await saveButtons[0].trigger("click");
    await flushPromises();

    expect(platformApi.updateEndUser).toHaveBeenCalledWith("user-1", {
      email: "new@example.com",
      phone: "13900000000",
      internalNote: "新内部备注"
    });
    expect(aiApi.upsertUserLimitPolicy).toHaveBeenCalledWith("user-1", {
      concurrency_limit: 5,
      status: "active"
    });
    expect(wrapper.emitted("saved")).toHaveLength(1);
    expect(wrapper.emitted("close")).toHaveLength(1);
  });
});
