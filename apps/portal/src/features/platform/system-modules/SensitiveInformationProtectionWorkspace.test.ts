import { flushPromises, mount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { DsButton, DsSwitch } from "@/shared/ui";
import SensitiveInformationProtectionWorkspace from "./SensitiveInformationProtectionWorkspace.vue";

const api = vi.hoisted(() => ({
  getPIIProtectionConfig: vi.fn(),
  getPIIProtectionDefaults: vi.fn(),
  updatePIIProtectionConfig: vi.fn(),
  previewPIIProtection: vi.fn()
}));

vi.mock("@/api/systemModules", () => ({ systemModulesApi: api }));

const rule = {
  id: "email",
  name: "邮箱",
  pattern: "email-pattern",
  enabled: true,
  system: true
};

describe("SensitiveInformationProtectionWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getPIIProtectionConfig.mockResolvedValue({ enabled: false, rules: [rule], placeholderPrefix: "DAI" });
    api.getPIIProtectionDefaults.mockResolvedValue({ enabled: false, rules: [rule], placeholderPrefix: "DAI" });
    api.updatePIIProtectionConfig.mockImplementation(async (config) => config);
    api.previewPIIProtection.mockResolvedValue({ protectedText: "contact __DAI_PII_EMAIL_1__" });
  });

  it("loads, updates and previews the effective configuration", async () => {
    const wrapper = await mountView();

    expect(wrapper.text()).toContain("1/1 条启用");
    wrapper.findAllComponents(DsSwitch)[0].vm.$emit("update:modelValue", true);
    await flushPromises();

    const saveButton = wrapper.findAllComponents(DsButton).find((button) => button.text().includes("保存配置"));
    expect(saveButton).toBeDefined();
    await saveButton!.trigger("click");
    await flushPromises();
    expect(api.updatePIIProtectionConfig).toHaveBeenCalledWith(expect.objectContaining({ enabled: true }));

    const previewButton = wrapper.findAllComponents(DsButton).find((button) => button.text().includes("执行预览"));
    await previewButton!.trigger("click");
    await flushPromises();
    expect(api.previewPIIProtection).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain("替换结果");
    expect((wrapper.find("textarea[readonly]").element as HTMLTextAreaElement).value).toBe("contact __DAI_PII_EMAIL_1__");

    wrapper.unmount();
  });
});

async function mountView() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/admin/system-modules/pii-protection", component: SensitiveInformationProtectionWorkspace }]
  });
  await router.push("/admin/system-modules/pii-protection");
  await router.isReady();
  const wrapper = mount(SensitiveInformationProtectionWorkspace, { global: { plugins: [router] } });
  await flushPromises();
  return wrapper;
}
