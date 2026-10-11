import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  getAccountBalance: vi.fn(),
  getAnalyticsOverview: vi.fn(),
  getUserConsumption: vi.fn(),
  getAppConsumption: vi.fn(),
  getUsers: vi.fn(),
  getDashboardSummary: vi.fn(),
  getDashboardTopModels: vi.fn(),
  listRecords: vi.fn(),
  listBalanceLedger: vi.fn()
}));

vi.mock("@/api/platformTenant", () => ({
  platformTenantApi: {
    getAccountBalance: apiMocks.getAccountBalance,
    getAnalyticsOverview: apiMocks.getAnalyticsOverview,
    getUserConsumption: apiMocks.getUserConsumption,
    getAppConsumption: apiMocks.getAppConsumption,
    getUsers: apiMocks.getUsers
  }
}));

vi.mock("@/api/aiTenant", () => ({
  aiTenantApi: {
    getDashboardSummary: apiMocks.getDashboardSummary,
    getDashboardTopModels: apiMocks.getDashboardTopModels
  }
}));

vi.mock("@/api/tenant", () => ({
  tenantApi: { listBalanceLedger: apiMocks.listBalanceLedger }
}));

vi.mock("@/features/ai/usage", () => ({
  listTenantUsageRecords: apiMocks.listRecords
}));

import { useTenantOperationsDashboard } from "./useTenantOperationsDashboard";
import { useTenantFinanceSummary } from "./useTenantFinanceSummary";

const financialSummary = {
  total_requests: 12,
  successful_requests: 10,
  failed_requests: 2,
  total_tokens: 3200,
  total_prompt_tokens: 2000,
  total_completion_tokens: 1200,
  total_catalog_base_usd: 4,
  total_tenant_payable_usd: 6.5,
  total_retail_base_usd: 8,
  total_user_payable_usd: 9,
  total_user_charged_usd: 8.5,
  avg_latency_ms: 420
};

const overviewStats = {
  endUserCount: 3,
  inviteCodeCount: 1,
  userDeductionUsd: 8.5,
  userTotalBalanceUsd: 12,
  lowBalanceUserCount: 0,
  activeUserCount: 2,
  userConsumptionCount: 10,
  settlementIncomeMicroUsd: 7_000_000
};

function mountDashboard() {
  let state!: ReturnType<typeof useTenantOperationsDashboard>;
  let finance!: ReturnType<typeof useTenantFinanceSummary>;
  const wrapper = mount(defineComponent({
    setup() {
      state = useTenantOperationsDashboard();
      finance = useTenantFinanceSummary(state);
      return () => h("div");
    }
  }));
  return { state, finance, wrapper };
}

describe("useTenantOperationsDashboard", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    apiMocks.getAccountBalance.mockResolvedValue({
      currency: "USD",
      totalUsd: 20,
      usedUsd: 5,
      remainingUsd: 15,
      availableUsd: 15,
      permanentUsd: 15,
      timedUsd: 0,
      outstandingDebtMicroUsd: 0,
      serviceState: "active",
      balanceLots: []
    });
    apiMocks.getAnalyticsOverview.mockResolvedValue(overviewStats);
    apiMocks.getUserConsumption.mockResolvedValue([]);
    apiMocks.getAppConsumption.mockResolvedValue([]);
    apiMocks.getUsers.mockResolvedValue({ items: [], total: 0, page: 1, size: 200 });
    apiMocks.getDashboardSummary.mockResolvedValue(financialSummary);
    apiMocks.getDashboardTopModels.mockResolvedValue({ items: [], total: 0 });
    apiMocks.listRecords.mockResolvedValue({ records: [], total: 0 });
    apiMocks.listBalanceLedger.mockResolvedValue({ items: [], total: 0, page: 1, size: 8 });
  });

  it("loads the AI settlement cost into the financial overview", async () => {
    const { state, finance, wrapper } = mountDashboard();
    await flushPromises();

    expect(state.financialSummary.value?.total_tenant_payable_usd).toBe(6.5);
    expect(finance.userRevenue.value).toBe(8.5);
    expect(finance.netCashReference.value).toBe(0.5);
    expect(apiMocks.getDashboardSummary).toHaveBeenCalledWith({
      date_from: expect.any(String),
      date_to: expect.any(String)
    });
    expect(apiMocks.getDashboardTopModels).toHaveBeenCalledWith({
      date_from: expect.any(String),
      date_to: expect.any(String),
      limit: 6
    });
    expect(apiMocks.getAccountBalance).toHaveBeenCalledWith(true);
    expect(apiMocks.listBalanceLedger).toHaveBeenCalledWith({ page: 1, size: 8 });
    wrapper.unmount();
  });

  it("keeps unavailable settlement data distinct from a real zero cost", async () => {
    const error = new Error("dashboard unavailable");
    apiMocks.getDashboardSummary.mockRejectedValue(error);

    const { state, finance, wrapper } = mountDashboard();
    await flushPromises();

    expect(state.financialSummary.value).toBeNull();
    expect(finance.platformCost.value).toBeNull();
    expect(finance.netCashReference.value).toBeNull();
    expect(finance.userRevenue.value).toBe(8.5);
    expect(state.failedSources("overview", "summary")).toEqual(["AI 结算汇总"]);
    wrapper.unmount();
  });

  it("updates the extracted finance model when the reporting period changes", async () => {
    const { state, finance, wrapper } = mountDashboard();
    await flushPromises();
    apiMocks.getDashboardSummary.mockResolvedValue({
      ...financialSummary,
      total_user_charged_usd: 0,
      total_tenant_payable_usd: 0
    });

    await state.selectRange("7d");
    await flushPromises();

    expect(finance.rangeLabel.value).toBe(state.selectedRangeLabel.value);
    expect(finance.userRevenue.value).toBe(0);
    expect(finance.platformCost.value).toBe(0);
    expect(finance.netCashReference.value).toBe(7);
    wrapper.unmount();
  });

  it("uses the full tenant aggregate rather than the paginated directory for low-balance counts", async () => {
    apiMocks.getAnalyticsOverview.mockResolvedValue({ ...overviewStats, lowBalanceUserCount: 251 });
    const { state, finance, wrapper } = mountDashboard();
    await flushPromises();
    expect(state.users.value).toEqual([]);
    expect(finance.lowBalanceCount.value).toBe(251);
    expect(finance.signals.value.find((signal) => signal.key === "users")?.detail).toContain("251 名启用用户");
    wrapper.unmount();
  });

  it("marks unavailable sources without replacing known data with zeros and recovers on retry", async () => {
    const { state, wrapper } = mountDashboard();
    await flushPromises();
    apiMocks.getAnalyticsOverview.mockRejectedValue(new Error("offline"));
    apiMocks.listBalanceLedger.mockRejectedValue(new Error("offline"));
    await state.refresh();
    expect(state.overview.value.userTotalBalanceUsd).toBe(12);
    expect(state.failedSources("overview", "summary", "ledger")).toEqual(["经营概览与用户统计", "账务记录"]);
    expect(state.loading.value).toBe(false);

    apiMocks.getAnalyticsOverview.mockResolvedValue(overviewStats);
    apiMocks.listBalanceLedger.mockResolvedValue({ items: [] });
    await state.refresh();
    expect(state.failedSources("overview", "ledger")).toEqual([]);
    wrapper.unmount();
  });

  it("ignores a late failure from an older reporting period", async () => {
    const { state, wrapper } = mountDashboard();
    await flushPromises();
    let rejectOld!: (error: Error) => void;
    apiMocks.getAnalyticsOverview.mockReturnValueOnce(new Promise((_resolve, reject) => { rejectOld = reject; }));
    const oldRange = state.selectRange("7d");
    apiMocks.getAnalyticsOverview.mockResolvedValue({ ...overviewStats, lowBalanceUserCount: 42 });
    await state.selectRange("today");
    rejectOld(new Error("old request failed"));
    await oldRange;
    expect(state.overview.value.lowBalanceUserCount).toBe(42);
    expect(state.failedSources("overview")).toEqual([]);
    wrapper.unmount();
  });
});
