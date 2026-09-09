import { flushPromises, shallowMount } from "@vue/test-utils";
import { ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

const overview = vi.hoisted(() => ({ useAdminOverviewSnapshot: vi.fn(), refresh: vi.fn(), changeRange: vi.fn() }));
vi.mock("./useAdminOverviewSnapshot", () => ({ useAdminOverviewSnapshot: overview.useAdminOverviewSnapshot }));
import BusinessOverviewWorkspace from "./BusinessOverviewWorkspace.vue";

const snapshot = () => ({ meta: { view: "business", date_from: 0, date_to: 0, generated_at: 0 }, summary: { total_requests: 100, successful_requests: 90, failed_requests: 10, total_tokens: 1000, total_prompt_tokens: 700, total_completion_tokens: 300, cache_read_tokens: 100, cache_write_tokens: 0, active_tenants: 4, active_users: 12, active_accounts: 2, new_tenants: 1, new_users: 5, total_catalog_base_usd: 1, total_tenant_payable_usd: 2, total_user_charged_usd: 3, gross_margin_usd: 1, avg_request_total_ms: 80, p95_request_total_ms: 100, avg_first_response_byte_ms: 20, p95_first_response_byte_ms: 30 }, trends: [], accounts: [], models: [], tenants: [], users: [], included: { tenants: {} } });

describe("BusinessOverviewWorkspace", () => {
  beforeEach(() => { vi.clearAllMocks(); overview.refresh.mockResolvedValue(undefined); overview.useAdminOverviewSnapshot.mockReturnValue({ selectedRangeId: ref("30d"), selectedRange: ref({ label: "最近 30 天", hours: 720 }), snapshot: ref(snapshot()), loading: ref(false), lastUpdatedAt: ref(null), error: ref(null), refresh: overview.refresh, changeRange: overview.changeRange }); });
  it("loads the business snapshot", async () => {
    const wrapper = shallowMount(BusinessOverviewWorkspace, { global: { stubs: { PortalPagePanel: { template: "<main><slot name='actions' /><slot /></main>" }, PortalMetricGrid: { template: "<div><slot /></div>" }, DsMetricCard: { props: ["label"], template: "<span>{{ label }}</span>" }, OverviewRangeControls: { template: "<button>刷新</button>" } } } });
    await flushPromises(); expect(overview.useAdminOverviewSnapshot).toHaveBeenCalledWith("business", "30d"); expect(overview.refresh).toHaveBeenCalledOnce(); expect(wrapper.text()).toContain("活跃租户"); wrapper.unmount();
  });
  it("delegates range and refresh actions", async () => {
    const wrapper = shallowMount(BusinessOverviewWorkspace, { global: { stubs: { PortalPagePanel: { template: "<main><slot name='actions' /><slot /></main>" }, PortalMetricGrid: { template: "<div><slot /></div>" }, DsMetricCard: { props: ["label"], template: "<span>{{ label }}</span>" }, OverviewRangeControls: { emits: ["refresh", "update:modelValue"], template: "<div><button data-testid='refresh' @click='$emit(\"refresh\")'>刷新</button><button data-testid='range' @click='$emit(\"update:modelValue\", \"7d\")'>7d</button></div>" } } } });
    await flushPromises(); await wrapper.get("[data-testid='refresh']").trigger("click"); await wrapper.get("[data-testid='range']").trigger("click"); expect(overview.refresh).toHaveBeenCalledTimes(2); expect(overview.changeRange).toHaveBeenCalledWith("7d"); wrapper.unmount();
  });
});
