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

import CostAnalysisWorkspace from "./CostAnalysisWorkspace.vue";

describe("CostAnalysisWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    overview.refresh.mockResolvedValue(undefined);
    overview.useAdminOverviewData.mockReturnValue({
      failedSections: ref([]),
      selectedRangeId: ref("30d"),
      lastUpdatedAt: ref(null),
      loading: ref(false),
      summary: ref({
        total_catalog_base_usd: 4,
        total_tenant_payable_usd: 8,
        total_user_charged_usd: 10
      }),
      trend: ref([]),
      upstreams: ref([]),
      refresh: overview.refresh,
      changeRange: overview.changeRange
    });
  });

  it("loads cost analysis data through the shared overview composable", async () => {
    const wrapper = shallowMount(CostAnalysisWorkspace, {
      global: {
        stubs: {
          PortalPagePanel: { template: "<main><slot name='actions' /><slot /></main>" },
          PortalMetricGrid: { template: "<div><slot /></div>" },
          DsMetricCard: { props: ["label"], template: "<span>{{ label }}</span>" },
          OverviewRangeControls: { template: "<button data-testid='cost-refresh'>刷新</button>" }
        }
      }
    });
    await flushPromises();

    expect(overview.useAdminOverviewData).toHaveBeenCalledWith(["summary", "trend", "upstreams"], "30d");
    expect(overview.refresh).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain("上游参考成本");
    wrapper.unmount();
  });

  it("delegates range changes to the overview data owner", async () => {
    const wrapper = shallowMount(CostAnalysisWorkspace, {
      global: {
        stubs: {
          PortalPagePanel: { template: "<main><slot name='actions' /><slot /></main>" },
          PortalMetricGrid: { template: "<div><slot /></div>" },
          DsMetricCard: { props: ["label"], template: "<span>{{ label }}</span>" },
          OverviewRangeControls: {
            emits: ["update:modelValue"],
            template: "<button data-testid='cost-range' @click='$emit(\"update:modelValue\", \"7d\")'>7d</button>"
          }
        }
      }
    });
    await flushPromises();

    await wrapper.get("[data-testid='cost-range']").trigger("click");
    expect(overview.changeRange).toHaveBeenCalledWith("7d");
    wrapper.unmount();
  });
});
