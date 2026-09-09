import { flushPromises, mount } from "@vue/test-utils";
import ElementPlus from "element-plus";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { DsButton } from "@/shared/ui";
import RequestRecordingSection from "./RequestRecordingSection.vue";

const api = vi.hoisted(() => ({
  getRequestRecordingSettings: vi.fn(),
  updateRequestRecordingSettings: vi.fn()
}));

vi.mock("@/api/systemModules", () => ({ systemModulesApi: api }));

describe("RequestRecordingSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getRequestRecordingSettings.mockResolvedValue({
      level: "basic",
      sensitive_headers: ["authorization", "cookie"]
    });
    api.updateRequestRecordingSettings.mockImplementation(async (value) => value);
  });

  it("loads and saves normalized settings inside the system modules card", async () => {
    const wrapper = mount(RequestRecordingSection, { global: { plugins: [ElementPlus] } });
    await flushPromises();

    expect(api.getRequestRecordingSettings).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain("配置已载入");
    await wrapper.get("select").setValue("headers");
    await wrapper.get("#request-sensitive-headers").setValue(" Authorization, X-Private, x-private ");

    const saveButton = wrapper.findAllComponents(DsButton).find((button) => button.text() === "保存");
    expect(saveButton).toBeDefined();
    await saveButton!.trigger("click");
    await flushPromises();

    expect(api.updateRequestRecordingSettings).toHaveBeenCalledWith({
      level: "headers",
      sensitive_headers: ["authorization", "x-private"]
    });
  });

  it("keeps a local retry state when loading fails", async () => {
    api.getRequestRecordingSettings
      .mockRejectedValueOnce(new Error("接口不可用"))
      .mockResolvedValueOnce({ level: "full", sensitive_headers: ["authorization"] });
    const wrapper = mount(RequestRecordingSection, { global: { plugins: [ElementPlus] } });
    await flushPromises();

    expect(wrapper.text()).toContain("无法读取请求记录设置");
    expect(wrapper.text()).toContain("接口不可用");
    const retryButton = wrapper.findAllComponents(DsButton).find((button) => button.text() === "重新加载");
    expect(retryButton).toBeDefined();
    await retryButton!.trigger("click");
    await flushPromises();

    expect(api.getRequestRecordingSettings).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).toContain("配置已载入");
    expect(wrapper.text()).toContain("FULL 会保存请求和响应正文");
  });
});
