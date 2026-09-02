import { flushPromises, mount } from "@vue/test-utils";
import ElementPlus, { ElMessageBox } from "element-plus";
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
  purgeRequestBodies: vi.fn(),
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
      generatedAt: "2026-08-28T04:00:00Z",
      requestBodyPurge: { eligibleRows: 2, occupiedBytes: 2048 },
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
    expect(wrapper.text()).toContain("立即清空请求体");
    expect(wrapper.text()).toContain("预计处理 2 条");
    expect(wrapper.text()).toContain("占用约 2 KB");
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

  it("refreshes the body purge preview before asking for confirmation", async () => {
    const prompt = vi.spyOn(ElMessageBox, "prompt").mockResolvedValue({ value: "CLEANUP_DATA", action: "confirm" } as never);
    api.purgeRequestBodies.mockResolvedValue({ id: "run-1", summary: {} });
    const { wrapper } = await mountWorkspace();

    const purgeButton = wrapper.findAllComponents(DsButton).find((button) => button.text().includes("清空请求体"));
    expect(purgeButton).toBeDefined();
    await purgeButton!.trigger("click");
    await flushPromises();

    expect(api.previewCleanup).toHaveBeenCalledTimes(2);
    expect(prompt).toHaveBeenCalledWith(expect.stringContaining("占用约 2 KB"), "清空请求体", expect.any(Object));
    expect(api.purgeRequestBodies).toHaveBeenCalledWith({ confirmation: "CLEANUP_DATA" });

    prompt.mockRestore();
    wrapper.unmount();
  });

  it("keeps the workspace compact and opens the full cleanup history in a drawer", async () => {
    api.listCleanupRuns.mockResolvedValue(
      Array.from({ length: 10 }, (_, index) => ({
        id: `run-${index}`,
        status: "completed",
        trigger: "automatic",
        createdAt: `2026-08-${String(28 - index).padStart(2, "0")}T04:00:00Z`,
        summary: { request_body: index }
      }))
    );

    const { wrapper } = await mountWorkspace();

    expect(wrapper.findAll(".cleanup-runs__viewport .ds-table__row")).toHaveLength(8);

    const viewAllButton = wrapper.findAllComponents(DsButton).find((button) => button.text().includes("查看全部"));
    expect(viewAllButton).toBeDefined();
    await viewAllButton!.trigger("click");
    await flushPromises();

    expect(document.body.querySelectorAll(".ds-drawer .ds-table__row")).toHaveLength(10);
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
