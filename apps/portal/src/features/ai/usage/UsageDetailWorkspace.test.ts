import { flushPromises, mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import RequestRecordsWorkspace from "./RequestRecordsWorkspace.vue";
const api = vi.hoisted(() => ({ list: vi.fn(), summary: vi.fn(), detail: vi.fn(), refund: vi.fn(), debug: vi.fn() }));
vi.mock("./recordsApi", async importOriginal => ({ ...await importOriginal<typeof import("./recordsApi")>(), recordsApi: api }));
vi.mock("@/stores/auth", () => ({ useAuthStore: () => ({ userInfo: { userType: 4 } }) }));
describe("request and fee details", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue({ records: [] });
    api.summary.mockResolvedValue({ requests: 1, errors: 0, interruptions: 1 });
    api.detail.mockResolvedValue({ request_id: "r1", model: "model", tokens: { input: 10, output: 2 }, charge: { state: "pending", reason: "reported_usage_before_disconnect", user_charged_micro: 0, subscription_used_micro: 0 }, delivery: "disconnected", execution_available: true });
  });
  it("explains interrupted delivery separately from pending fees", async () => {
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: "/:requestId", component: RequestRecordsWorkspace }] });
    await router.push("/r1"); await router.isReady();
    const wrapper = mount(RequestRecordsWorkspace, { global: { plugins: [router, createPinia()], stubs: {
      PortalPagePanel: { template: "<main><slot /></main>" }, PortalMetricGrid: true, DsTabs: true, DsTable: true, DsFilterBar: true,
      DsDrawer: { template: "<aside><slot /></aside>" }, DsTag: { template: "<span><slot /></span>" }, "el-button": true
    } } });
    await flushPromises();
    expect(api.detail).toHaveBeenCalledWith("r1", expect.any(AbortSignal));
    expect(wrapper.text()).toContain("结算处理中");
    expect(wrapper.text()).toContain("仅结算取消前已报告用量");
    expect(wrapper.text()).toContain("客户端连接中断");
    expect(wrapper.text()).not.toContain("失败阶段");
    expect(wrapper.text()).not.toContain("全额退款");
    wrapper.unmount();
  });
});
