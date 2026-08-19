import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  getAccountBalance: vi.fn(),
  getAnalyticsOverview: vi.fn(),
  getUserConsumption: vi.fn(),
  getDashboardSummary: vi.fn(),
  listRecords: vi.fn()
}));

vi.mock("@/api/platformTenant", () => ({
  platformTenantApi: {
    getAccountBalance: apiMocks.getAccountBalance,
    getAnalyticsOverview: apiMocks.getAnalyticsOverview,
    getUserConsumption: apiMocks.getUserConsumption
  }
}));

vi.mock("@/api/aiTenant", () => ({
  aiTenantApi: { getDashboardSummary: apiMocks.getDashboardSummary }
}));

vi.mock("@/features/ai/usage", () => ({
  tenantUsageApi: { listRecords: apiMocks.listRecords }
}));

import { useTenantOperationsDashboard } from "./useTenantOperationsDashboard";

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

function mountDashboard() {
  let state!: ReturnType<typeof useTenantOperationsDashboard>;
  const wrapper = mount(defineComponent({
    setup() {
      state = useTenantOperationsDashboard();
      return () => h("div");
    }
  }));
  return { state, wrapper };
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
    apiMocks.getAnalyticsOverview.mockResolvedValue({
      endUserCount: 3,
      inviteCodeCount: 1,
      userDeductionUsd: 8.5,
      userTotalBalanceUsd: 12,
      activeUserCount: 2,
      userConsumptionCount: 10,
      settlementIncomeMicroUsd: 7_000_000
    });
    apiMocks.getUserConsumption.mockResolvedValue([]);
    apiMocks.getDashboardSummary.mockResolvedValue(financialSummary);
    apiMocks.listRecords.mockResolvedValue({ records: [], total: 0 });
  });

  it("loads the AI settlement cost into the financial overview", async () => {
    const { state, wrapper } = mountDashboard();
    await flushPromises();

    expect(state.financialSummary.value?.total_tenant_payable_usd).toBe(6.5);
    expect(apiMocks.getDashboardSummary).toHaveBeenCalledWith({
      date_from: expect.any(String),
      date_to: expect.any(String)
    });
    wrapper.unmount();
  });

  it("keeps unavailable settlement data distinct from a real zero cost", async () => {
    const error = new Error("dashboard unavailable");
    apiMocks.getDashboardSummary.mockRejectedValue(error);
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);

    const { state, wrapper } = mountDashboard();
    await flushPromises();

    expect(state.financialSummary.value).toBeNull();
    expect(consoleError).toHaveBeenCalledWith("获取租户 AI 结算概览失败:", error);
    wrapper.unmount();
    consoleError.mockRestore();
  });
});
