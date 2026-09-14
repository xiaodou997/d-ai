import { flushPromises, mount } from "@vue/test-utils";
import ElementPlus from "element-plus";
import { beforeEach, describe, expect, it, vi } from "vitest";
import RequestRecordingSection from "./RequestRecordingSection.vue";
const startDebug = vi.hoisted(() => vi.fn());
vi.mock("@/features/ai/usage/recordsApi", () => ({ recordsApi: { startDebug } }));
describe("scoped debug recording", () => {
  beforeEach(() => { startDebug.mockReset(); startDebug.mockResolvedValue({ id: "debug1", expires_at: "2026-09-14T12:00:00Z" }); });
  it("requires scope and defaults to one hour", async () => {
    const wrapper = mount(RequestRecordingSection, { global: { plugins: [ElementPlus] } });
    expect(wrapper.get("button.el-button").attributes("disabled")).toBeDefined();
    await wrapper.findAll("input")[0].setValue("tenant1");
    await wrapper.get("button.el-button").trigger("click");
    await flushPromises();
    expect(startDebug).toHaveBeenCalledWith({ tenant_id: "tenant1", api_key_id: "", model: "", hours: 1 });
    expect(wrapper.text()).toContain("自动停止");
    wrapper.unmount();
  });
});
