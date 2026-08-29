import { flushPromises, shallowMount } from "@vue/test-utils";
import { ref } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

const overview = vi.hoisted(() => ({
  refresh: vi.fn(),
  refreshHealth: vi.fn(),
  useAdminOverviewData: vi.fn()
}));

vi.mock("@/features/admin/overview/useAdminOverviewData", () => ({
  useAdminOverviewData: overview.useAdminOverviewData
}));

import { DsTabs } from "@/shared/ui";
import OperationsOverviewWorkspace from "./OperationsOverviewWorkspace.vue";

function overviewData(refresh: () => Promise<void>) {
  return {
    failedSections: ref<string[]>([]),
    selectedRangeId: ref("24h"),
    loading: ref(false),
    lastUpdatedAt: ref<Date | null>(null),
    selectedRange: ref({ label: "最近 24 小时", hours: 24 }),
    summary: ref({
      total_requests: 0,
      failed_requests: 0,
      avg_request_total_ms: 0,
      avg_first_response_byte_ms: 0
    }),
    trend: ref([]),
    system: ref(null),
    errors: ref([]),
    proxyNodes: ref([]),
    modules: ref([]),
    refresh,
    changeRange: vi.fn()
  };
}

describe("OperationsOverviewView", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    overview.refresh.mockResolvedValue(undefined);
    overview.refreshHealth.mockResolvedValue(undefined);
    overview.useAdminOverviewData
      .mockReturnValueOnce(overviewData(overview.refresh))
      .mockReturnValueOnce(overviewData(overview.refreshHealth));
  });

  it("loads health data only after switching to the health tab", async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/admin/overview/operations", component: OperationsOverviewWorkspace }]
    });
    await router.push("/admin/overview/operations");
    await router.isReady();

    const wrapper = shallowMount(OperationsOverviewWorkspace, {
      global: {
        plugins: [router],
        stubs: {
          PortalPagePanel: { template: "<main><slot name='actions' /><slot /></main>" }
        }
      }
    });
    await flushPromises();

    expect(overview.refresh).toHaveBeenCalledOnce();
    expect(overview.refreshHealth).not.toHaveBeenCalled();

    wrapper.findComponent(DsTabs).vm.$emit("update:modelValue", "health");
    await flushPromises();

    expect(router.currentRoute.value.query.tab).toBe("health");
    expect(overview.refreshHealth).toHaveBeenCalledOnce();

    wrapper.findComponent(DsTabs).vm.$emit("update:modelValue", "overview");
    await flushPromises();
    wrapper.findComponent(DsTabs).vm.$emit("update:modelValue", "health");
    await flushPromises();

    expect(overview.refresh).toHaveBeenCalledOnce();
    expect(overview.refreshHealth).toHaveBeenCalledOnce();
    wrapper.unmount();
  });
});
