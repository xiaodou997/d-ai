import { flushPromises, mount } from "@vue/test-utils";
import ElementPlus from "element-plus";
import { createMemoryHistory, createRouter } from "vue-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { DsButton, DsSwitch } from "@/shared/ui";
import SystemModulesWorkspace from "./SystemModulesWorkspace.vue";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  setEnabled: vi.fn(),
  getCleanupPolicy: vi.fn(),
  previewCleanup: vi.fn(),
  listCleanupRuns: vi.fn(),
  updateCleanupPolicy: vi.fn(),
  startCleanup: vi.fn(),
  sendNotification: vi.fn()
}));

vi.mock("@/api/systemModules", () => ({ systemModulesApi: api }));

const module = {
  name: "announcements",
  displayName: "公告服务",
  description: "统一公告投递",
  category: "platform",
  adminRoute: "/admin/announcements",
  order: 1,
  available: true,
  enabled: true,
  active: true,
  configValidated: true,
  health: "healthy"
};
const policy = {
  enabled: true,
  requestBodyDays: 30,
  requestPayloadDays: 180,
  notificationDays: 90,
  moderationDays: 90,
  riskEventDays: 365,
  adminAuditDays: 365,
  auditBlobDays: 180,
  usageRollupDays: 730,
  batchSize: 1000
};

describe("SystemModulesWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue([module]);
    api.setEnabled.mockResolvedValue({ ...module, enabled: false, active: false });
    api.getCleanupPolicy.mockResolvedValue(policy);
    api.previewCleanup.mockResolvedValue({
      items: [{ target: "request_body", label: "请求正文", retentionDays: 30, eligibleRows: 2 }]
    });
    api.listCleanupRuns.mockResolvedValue([]);
    api.updateCleanupPolicy.mockResolvedValue(policy);
  });

  afterEach(() => {
    document.body.innerHTML = "";
  });

  it("loads module status and cleanup preview", async () => {
    const { wrapper } = await mountWorkspace();

    expect(api.list).toHaveBeenCalledOnce();
    expect(api.getCleanupPolicy).toHaveBeenCalledOnce();
    expect(api.previewCleanup).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain("公告服务");
    expect(wrapper.text()).toContain("数据生命周期");
    expect(wrapper.text()).toContain("预计处理 2 条");
    wrapper.unmount();
  });

  it("toggles a module and saves the cleanup policy from the workspace", async () => {
    const { wrapper } = await mountWorkspace();
    wrapper.findComponent(DsSwitch).vm.$emit("update:modelValue", false);
    await flushPromises();
    expect(api.setEnabled).toHaveBeenCalledWith("announcements", false);

    const saveButton = wrapper.findAllComponents(DsButton).find((button) => button.text().includes("保存策略"));
    expect(saveButton).toBeDefined();
    await saveButton!.trigger("click");
    await flushPromises();
    expect(api.updateCleanupPolicy).toHaveBeenCalledWith(policy);
    wrapper.unmount();
  });
});

async function mountWorkspace() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/admin/system-modules", component: SystemModulesWorkspace }]
  });
  await router.push("/admin/system-modules");
  await router.isReady();
  const wrapper = mount(SystemModulesWorkspace, {
    attachTo: document.body,
    global: { plugins: [router, ElementPlus] }
  });
  await flushPromises();
  return { router, wrapper };
}
