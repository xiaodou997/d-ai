import { beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  getDashboardSummary: vi.fn(),
  listDashboardTopModels: vi.fn(),
  listDashboardTopTenants: vi.fn(),
  listDashboardRecentErrors: vi.fn(),
  getSystemStatus: vi.fn(),
  getGlobalStats: vi.fn(),
  listDailyTrend: vi.fn(),
  listUpstreamSummary: vi.fn(),
  listModules: vi.fn(),
  listProxyNodes: vi.fn()
}));

vi.mock("@/api/aiAdmin", () => ({
  aiAdminApi: {
    getDashboardSummary: api.getDashboardSummary,
    listDashboardTopModels: api.listDashboardTopModels,
    listDashboardTopTenants: api.listDashboardTopTenants,
    listDashboardRecentErrors: api.listDashboardRecentErrors,
    getSystemStatus: api.getSystemStatus
  }
}));

vi.mock("@/api/platformAdmin", () => ({
  platformAdminApi: { getGlobalStats: api.getGlobalStats }
}));

vi.mock("@/api/systemModules", () => ({
  systemModulesApi: {
    list: api.listModules,
    listProxyNodes: api.listProxyNodes
  }
}));

vi.mock("@/features/ai/usage", () => ({
  adminUsageApi: {
    listDailyTrend: api.listDailyTrend,
    listUpstreamSummary: api.listUpstreamSummary
  }
}));

import { useAdminOverviewData } from "./useAdminOverviewData";

function summary(totalRequests: number) {
  return {
    total_requests: totalRequests,
    successful_requests: totalRequests,
    failed_requests: 0,
    total_tokens: totalRequests * 10,
    total_prompt_tokens: totalRequests * 6,
    total_completion_tokens: totalRequests * 4,
    total_catalog_base_usd: 0,
    total_tenant_payable_usd: 0,
    total_retail_base_usd: 0,
    total_user_payable_usd: 0,
    total_user_charged_usd: 0,
    avg_latency_ms: 0,
    avg_request_total_ms: 0,
    avg_first_response_byte_ms: 0
  };
}

function trendRow(date: string, requestCount: number) {
  return {
    date,
    request_count: requestCount,
    success_count: requestCount,
    failed_count: 0,
    total_tokens: requestCount * 10,
    prompt_tokens: requestCount * 6,
    completion_tokens: requestCount * 4,
    reasoning_tokens: 0,
    cache_read_tokens: 0,
    cache_write_tokens: 0,
    image_units: 0,
    audio_input_units: 0,
    audio_output_units: 0,
    total_billable_units: requestCount * 10,
    catalog_base_usd: 0,
    tenant_payable_usd: 0,
    retail_base_usd: 0,
    user_payable_usd: 0,
    user_charged_usd: 0,
    avg_latency_ms: 0,
    avg_request_total_ms: 0,
    avg_first_response_byte_ms: 0
  };
}

describe("useAdminOverviewData", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("loads requested trend data", async () => {
    api.listDailyTrend.mockResolvedValue({ items: [trendRow("2026-08-19", 12)], total: 1 });
    const data = useAdminOverviewData(["trend"], "24h");

    await data.refresh();

    expect(api.listDailyTrend).toHaveBeenCalledOnce();
    expect(data.trend.value).toHaveLength(1);
    expect(data.failedSections.value).toEqual([]);
  });

  it("clears failed sections instead of retaining data from the previous range", async () => {
    api.getDashboardSummary.mockResolvedValueOnce(summary(30)).mockResolvedValueOnce(summary(7));
    api.listDailyTrend
      .mockResolvedValueOnce({ items: [trendRow("2026-08-19", 30)], total: 1 })
      .mockRejectedValueOnce(new Error("trend unavailable"));
    const data = useAdminOverviewData(["summary", "trend"], "30d");

    await data.refresh();
    data.selectedRangeId.value = "7d";
    await data.refresh();

    expect(data.summary.value.total_requests).toBe(7);
    expect(data.trend.value).toEqual([]);
    expect(data.failedSections.value).toEqual(["trend"]);
    expect(data.lastUpdatedAt.value).toBeInstanceOf(Date);
  });

  it("does not report a fresh update when every requested section fails", async () => {
    api.getDashboardSummary.mockRejectedValue(new Error("summary unavailable"));
    api.listDailyTrend.mockRejectedValue(new Error("trend unavailable"));
    const data = useAdminOverviewData(["summary", "trend"], "24h");

    await data.refresh();

    expect(data.summary.value.total_requests).toBe(0);
    expect(data.trend.value).toEqual([]);
    expect(data.failedSections.value).toEqual(["summary", "trend"]);
    expect(data.lastUpdatedAt.value).toBeNull();
  });
});
