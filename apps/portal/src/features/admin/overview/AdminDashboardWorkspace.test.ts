import { flushPromises, shallowMount } from "@vue/test-utils";
import { ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

const overview = vi.hoisted(() => ({
  useAdminOverviewData: vi.fn(),
  refresh: vi.fn(),
  changeRange: vi.fn()
}));

vi.mock("./useAdminOverviewData", () => ({
  useAdminOverviewData: overview.useAdminOverviewData
}));

import AdminDashboardWorkspace from "./AdminDashboardWorkspace.vue";

describe("AdminDashboardWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    overview.refresh.mockResolvedValue(undefined);
    overview.useAdminOverviewData.mockReturnValue({
      failedSections: ref([]),
      selectedRangeId: ref("24h"),
      loading: ref(false),
      lastUpdatedAt: ref(null),
      selectedRange: ref({ label: "最近 24 小时", hours: 24 }),
      summary: ref({
        total_requests: 12,
        successful_requests: 10,
        failed_requests: 2,
        total_tokens: 120,
        avg_request_total_ms: 80,
        avg_first_response_byte_ms: 20
      }),
      models: ref([]),
      errors: ref([]),
      trend: ref([]),
      system: ref(null),
      modules: ref([]),
      proxyNodes: ref([]),
      refresh: overview.refresh,
      changeRange: overview.changeRange
    });
  });

  it("loads the dashboard snapshot through the overview composable", async () => {
    const wrapper = shallowMount(AdminDashboardWorkspace, {
      global: {
        stubs: {
          PortalPagePanel: { template: "<main><slot name='actions' /><slot /></main>" },
          OverviewRangeControls: { template: "<button data-testid='dashboard-refresh'>刷新</button>" },
          PortalMetricGrid: { template: "<div><slot /></div>" },
          DsMetricCard: { props: ["label"], template: "<span>{{ label }}</span>" }
        }
      }
    });
    await flushPromises();

    expect(overview.useAdminOverviewData).toHaveBeenCalledWith(
      ["summary", "models", "errors", "trend", "system", "modules", "proxy"],
      "24h"
    );
    expect(overview.refresh).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain("请求量");
    wrapper.unmount();
  });

  it("delegates range and refresh actions to the feature data owner", async () => {
    const wrapper = shallowMount(AdminDashboardWorkspace, {
      global: {
        stubs: {
          PortalPagePanel: { template: "<main><slot name='actions' /><slot /></main>" },
          OverviewRangeControls: {
            emits: ["refresh", "update:modelValue"],
            template: "<div><button data-testid='dashboard-refresh' @click='$emit(\"refresh\")'>刷新</button><button data-testid='dashboard-range' @click='$emit(\"update:modelValue\", \"7d\")'>7d</button></div>"
          },
          PortalMetricGrid: { template: "<div><slot /></div>" },
          DsMetricCard: { props: ["label"], template: "<span>{{ label }}</span>" }
        }
      }
    });
    await flushPromises();

    await wrapper.get("[data-testid='dashboard-refresh']").trigger("click");
    await wrapper.get("[data-testid='dashboard-range']").trigger("click");

    expect(overview.refresh).toHaveBeenCalledTimes(2);
    expect(overview.changeRange).toHaveBeenCalledWith("7d");
    wrapper.unmount();
  });
});
