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

import BusinessOverviewWorkspace from "./BusinessOverviewWorkspace.vue";

describe("BusinessOverviewWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    overview.refresh.mockResolvedValue(undefined);
    overview.useAdminOverviewData.mockReturnValue({
      failedSections: ref([]),
      selectedRangeId: ref("30d"),
      selectedRange: ref({ label: "最近 30 天", hours: 24 * 30 }),
      loading: ref(false),
      lastUpdatedAt: ref(null),
      summary: ref({
        total_requests: 100,
        successful_requests: 90,
        failed_requests: 10,
        total_tokens: 1_000,
        total_catalog_base_usd: 1,
        total_tenant_payable_usd: 2,
        total_user_charged_usd: 3
      }),
      models: ref([]),
      tenants: ref([]),
      tenantIncluded: ref({}),
      trend: ref([]),
      global: ref({ activeTenants: 4, newUsers: 12 }),
      refresh: overview.refresh,
      changeRange: overview.changeRange
    });
  });

  it("loads business dashboard data through the overview feature", async () => {
    const wrapper = shallowMount(BusinessOverviewWorkspace, {
      global: {
        stubs: {
          PortalPagePanel: { template: "<main><slot name='actions' /><slot /></main>" },
          PortalMetricGrid: { template: "<div><slot /></div>" },
          DsMetricCard: { props: ["label"], template: "<span>{{ label }}</span>" },
          OverviewRangeControls: { template: "<button data-testid='business-refresh'>刷新</button>" }
        }
      }
    });
    await flushPromises();

    expect(overview.useAdminOverviewData).toHaveBeenCalledWith(
      ["summary", "models", "tenants", "trend", "global"],
      "30d"
    );
    expect(overview.refresh).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain("活跃租户");
    wrapper.unmount();
  });

  it("keeps range changes and refresh actions in the data owner", async () => {
    const wrapper = shallowMount(BusinessOverviewWorkspace, {
      global: {
        stubs: {
          PortalPagePanel: { template: "<main><slot name='actions' /><slot /></main>" },
          PortalMetricGrid: { template: "<div><slot /></div>" },
          DsMetricCard: { props: ["label"], template: "<span>{{ label }}</span>" },
          OverviewRangeControls: {
            emits: ["refresh", "update:modelValue"],
            template: "<div><button data-testid='business-refresh' @click='$emit(\"refresh\")'>刷新</button><button data-testid='business-range' @click='$emit(\"update:modelValue\", \"7d\")'>7d</button></div>"
          }
        }
      }
    });
    await flushPromises();

    await wrapper.get("[data-testid='business-refresh']").trigger("click");
    await wrapper.get("[data-testid='business-range']").trigger("click");

    expect(overview.refresh).toHaveBeenCalledTimes(2);
    expect(overview.changeRange).toHaveBeenCalledWith("7d");
    wrapper.unmount();
  });
});
