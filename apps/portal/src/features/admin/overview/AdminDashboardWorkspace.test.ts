import { flushPromises, shallowMount } from "@vue/test-utils";
import { ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

const overview = vi.hoisted(() => ({ useAdminOverviewSnapshot: vi.fn(), refresh: vi.fn(), changeRange: vi.fn() }));
vi.mock("./useAdminOverviewSnapshot", () => ({ useAdminOverviewSnapshot: overview.useAdminOverviewSnapshot }));
import AdminDashboardWorkspace from "./AdminDashboardWorkspace.vue";

const snapshot = () => ({
  meta: { view: "dashboard", date_from: 0, date_to: 0, generated_at: 0 },
  summary: { total_requests: 12, successful_requests: 10, failed_requests: 2, total_tokens: 120, total_prompt_tokens: 80, total_completion_tokens: 40, cache_read_tokens: 20, cache_write_tokens: 0, active_tenants: 2, active_users: 4, active_accounts: 1, new_tenants: 1, new_users: 2, total_catalog_base_usd: 1, total_tenant_payable_usd: 2, total_user_charged_usd: 3, gross_margin_usd: 1, avg_request_total_ms: 80, p95_request_total_ms: 100, avg_first_response_byte_ms: 20, p95_first_response_byte_ms: 30 },
  trends: [], accounts: [], models: [], tenants: [], users: [], included: { tenants: {} }
});

describe("AdminDashboardWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks(); overview.refresh.mockResolvedValue(undefined);
    overview.useAdminOverviewSnapshot.mockReturnValue({ selectedRangeId: ref("24h"), selectedRange: ref({ label: "最近 24 小时", hours: 24 }), snapshot: ref(snapshot()), loading: ref(false), lastUpdatedAt: ref(null), error: ref(null), refresh: overview.refresh, changeRange: overview.changeRange });
  });

  it("loads the operating dashboard snapshot without a recent-errors panel", async () => {
    const wrapper = shallowMount(AdminDashboardWorkspace, { global: { stubs: {
      PortalPagePanel: { template: "<main><slot name='actions' /><slot /></main>" },
      OverviewRangeControls: { template: "<button>刷新</button>" }, PortalMetricGrid: { template: "<div><slot /></div>" }, DsMetricCard: { props: ["label"], template: "<span>{{ label }}</span>" }
    } } });
    await flushPromises();
    expect(overview.useAdminOverviewSnapshot).toHaveBeenCalledWith("dashboard", "24h");
    expect(overview.refresh).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain("请求总量");
    expect(wrapper.text()).not.toContain("最近错误");
    wrapper.unmount();
  });

  it("delegates range and refresh actions to the snapshot owner", async () => {
    const wrapper = shallowMount(AdminDashboardWorkspace, { global: { stubs: {
      PortalPagePanel: { template: "<main><slot name='actions' /><slot /></main>" },
      OverviewRangeControls: { emits: ["refresh", "update:modelValue"], template: "<div><button data-testid='refresh' @click='$emit(\"refresh\")'>刷新</button><button data-testid='range' @click='$emit(\"update:modelValue\", \"7d\")'>7d</button></div>" },
      PortalMetricGrid: { template: "<div><slot /></div>" }, DsMetricCard: { props: ["label"], template: "<span>{{ label }}</span>" }
    } } });
    await flushPromises(); await wrapper.get("[data-testid='refresh']").trigger("click"); await wrapper.get("[data-testid='range']").trigger("click");
    expect(overview.refresh).toHaveBeenCalledTimes(2); expect(overview.changeRange).toHaveBeenCalledWith("7d"); wrapper.unmount();
  });
});
