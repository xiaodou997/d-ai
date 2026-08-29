import { defineComponent } from "vue";
import { flushPromises, shallowMount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import TenantDashboardWorkspace from "./TenantDashboardWorkspace.vue";

const aiApi = vi.hoisted(() => ({
  getDashboardSummary: vi.fn(),
  getDashboardTopModels: vi.fn(),
  listApiKeys: vi.fn(),
  listDashboardRecentErrors: vi.fn(),
  listAvailableModels: vi.fn(),
  listMyGroups: vi.fn()
}));
const tenantApi = vi.hoisted(() => ({ listEndUsers: vi.fn() }));
const usageApi = vi.hoisted(() => ({ listTenantUsageRecords: vi.fn() }));
const router = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock("@/api/aiTenant", () => ({ aiTenantApi: aiApi, formatUSD: (value: number) => `$${value.toFixed(2)}` }));
vi.mock("@/api/tenant", () => ({ tenantApi }));
vi.mock("@/features/ai/usage", () => ({ listTenantUsageRecords: usageApi.listTenantUsageRecords }));
vi.mock("vue-router", () => ({ useRouter: () => router }));

const PanelStub = defineComponent({ template: "<div><slot name=\"actions\" /><slot /></div>" });
const RangeStub = defineComponent({
  emits: ["update:modelValue"],
  template: "<button data-range @click=\"$emit('update:modelValue', '7d')\">切换范围</button>"
});
const ButtonStub = defineComponent({ emits: ["click"], template: "<button @click=\"$emit('click')\"><slot /></button>" });

const global = {
  stubs: {
    PortalPagePanel: PanelStub,
    TenantWorkbenchRangeTabs: RangeStub,
    AiWorkbenchMetricsSection: true,
    AiWorkbenchChartsSection: true,
    AiWorkbenchQualitySection: true,
    ElButton: ButtonStub
  }
};

describe("TenantDashboardWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    aiApi.listMyGroups.mockResolvedValue({ items: [], total: 2 });
    aiApi.listAvailableModels.mockResolvedValue({ items: [{ model_code: "gpt-5" }], total: 1 });
    aiApi.listApiKeys.mockResolvedValue({ items: [{ id: "key-1" }], total: 1 });
    aiApi.getDashboardSummary.mockResolvedValue({ total_requests: 10, successful_requests: 9, total_tokens: 100, avg_latency_ms: 120 });
    aiApi.getDashboardTopModels.mockResolvedValue({ items: [], total: 0 });
    aiApi.listDashboardRecentErrors.mockResolvedValue({ items: [], total: 0 });
    tenantApi.listEndUsers.mockResolvedValue({ items: [], total: 0, page: 1, size: 200 });
    usageApi.listTenantUsageRecords.mockResolvedValue({ total: 0, stats: {}, records: [] });
  });

  it("loads tenant access, usage and quality data for the default range", async () => {
    const wrapper = shallowMount(TenantDashboardWorkspace, { global });
    await flushPromises();

    expect(aiApi.listMyGroups).toHaveBeenCalledOnce();
    expect(aiApi.listAvailableModels).toHaveBeenCalledOnce();
    expect(aiApi.listApiKeys).toHaveBeenCalledOnce();
    expect(aiApi.getDashboardSummary).toHaveBeenCalledWith({ date_from: expect.any(String), date_to: expect.any(String) });
    expect(tenantApi.listEndUsers).toHaveBeenCalledWith({ page: 1, size: 200 });
    expect(usageApi.listTenantUsageRecords).toHaveBeenCalledWith(expect.objectContaining({ limit: 360, offset: 0, date_from: expect.any(String), date_to: expect.any(String) }));

    wrapper.unmount();
  });

  it("refreshes only range-bound data when the analysis range changes", async () => {
    const wrapper = shallowMount(TenantDashboardWorkspace, { global });
    await flushPromises();

    await wrapper.get("[data-range]").trigger("click");
    await flushPromises();

    expect(aiApi.getDashboardSummary).toHaveBeenCalledTimes(2);
    expect(aiApi.getDashboardTopModels).toHaveBeenCalledTimes(2);
    expect(aiApi.listDashboardRecentErrors).toHaveBeenCalledTimes(2);
    expect(usageApi.listTenantUsageRecords).toHaveBeenCalledTimes(2);
    expect(aiApi.listMyGroups).toHaveBeenCalledOnce();
    expect(tenantApi.listEndUsers).toHaveBeenCalledOnce();
    expect(usageApi.listTenantUsageRecords).toHaveBeenLastCalledWith(expect.objectContaining({ limit: 240, offset: 0 }));

    wrapper.unmount();
  });
});
