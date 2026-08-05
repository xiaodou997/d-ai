import { defineComponent, h } from "vue";
import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { components } from "@dai/api-client/ai";

import type { AdminUsageApi } from "../api";
import { mapAdminUsageRows, type UsageLogDTO } from "../model";
import { useAdminUsageExplorer } from "./useAdminUsageExplorer";

const messageError = vi.fn();

type Schemas = components["schemas"];
type UsageLogsResponse = Schemas["UsageLogsOutputBody"];
type UsageDetailResponse = Schemas["UsageLogDetailDTO"];

describe("useAdminUsageExplorer", () => {
  beforeEach(() => messageError.mockReset());

  it("sends server pagination filters and resets them consistently", async () => {
    const api = fakeApi();
    const { state, wrapper } = mountComposable(api);
    state.filters.user_id = "user-7";
    state.filters.model_code = "gpt-4o";

    await state.applyFilters();
    expect(api.listLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ user_id: "user-7", model_code: "gpt-4o", limit: 20, offset: 0 }),
      expect.any(AbortSignal)
    );

    await state.changePage(3);
    expect(api.listLogs).toHaveBeenLastCalledWith(expect.objectContaining({ offset: 40 }), expect.any(AbortSignal));

    await state.resetFilters();
    expect(state.pagination.page).toBe(1);
    expect(api.listLogs).toHaveBeenLastCalledWith(expect.objectContaining({ user_id: undefined, model_code: undefined }), expect.any(AbortSignal));
    wrapper.unmount();
  });

  it("keeps the newest list and detail when responses resolve out of order", async () => {
    const firstLogs = deferred<UsageLogsResponse>();
    const secondLogs = deferred<UsageLogsResponse>();
    const firstDetail = deferred<UsageDetailResponse>();
    const secondDetail = deferred<UsageDetailResponse>();
    const api = fakeApi();
    api.listLogs
      .mockImplementationOnce((_query, signal) => captureSignal(firstLogs, signal))
      .mockImplementationOnce((_query, signal) => captureSignal(secondLogs, signal));
    api.getDetail
      .mockImplementationOnce((_requestId, signal) => captureSignal(firstDetail, signal))
      .mockImplementationOnce((_requestId, signal) => captureSignal(secondDetail, signal));
    const { state, wrapper } = mountComposable(api);

    const firstRefresh = state.refresh();
    const secondRefresh = state.refresh();
    expect(firstLogs.signal?.aborted).toBe(true);
    secondLogs.resolve(usageResponse("newest"));
    await secondRefresh;
    firstLogs.resolve(usageResponse("stale"));
    await firstRefresh;
    expect(state.logs.value[0]?.request_id).toBe("newest");

    const [firstRow, secondRow] = mapAdminUsageRows([usageRow("detail-old"), usageRow("detail-new")], null);
    if (!firstRow || !secondRow) throw new Error("missing test rows");
    const firstOpen = state.openDetail(firstRow);
    const secondOpen = state.openDetail(secondRow);
    expect(firstDetail.signal?.aborted).toBe(true);
    secondDetail.resolve(usageDetail("detail-new"));
    await secondOpen;
    firstDetail.resolve(usageDetail("detail-old"));
    await firstOpen;
    expect(state.detail.value?.request_id).toBe("detail-new");
    wrapper.unmount();
  });

  it("cancels stale tab work and ignores AbortError notifications", async () => {
    const errors = deferred<UsageLogsResponse>();
    const api = fakeApi();
    api.listLogs.mockImplementationOnce((_query, signal) => captureSignal(errors, signal));
    const { state, wrapper } = mountComposable(api);

    const errorsLoad = state.changeTab("errors");
    await state.changeTab("ranking");
    expect(errors.signal?.aborted).toBe(true);
    errors.reject(new DOMException("cancelled", "AbortError"));
    await errorsLoad;
    expect(state.activeTab.value).toBe("ranking");
    expect(state.errorRows.value).toEqual([]);
    expect(messageError).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("loads upstream output only once its tab is opened", async () => {
    const api = fakeApi();
    api.listUpstreamSummary.mockResolvedValue({
      items: [{
        target_kind: "direct_upstream",
        target_id: "acct-1",
        target_name: "OpenAI 官方",
        provider_code: "openai",
        request_count: 2,
        success_count: 2,
        failed_count: 0,
        total_prompt_tokens: 100,
        total_completion_tokens: 50,
        total_tokens: 150,
        token_units: 150,
        image_units: 3,
        catalog_base_credits: 1,
        tenant_payable_credits: 2
      }],
      total: 1
    });
    const { state, wrapper } = mountComposable(api);

    // 该报表是对账用的，不应该在别的 tab 上白跑一次查询。
    expect(api.listUpstreamSummary).not.toHaveBeenCalled();

    await state.changeTab("upstream");
    expect(state.upstreamRows.value).toHaveLength(1);
    expect(state.upstreamRows.value[0]!.image_units).toBe(3);
    wrapper.unmount();
  });

  it("aborts in-flight requests when the feature unmounts", async () => {
    const logs = deferred<UsageLogsResponse>();
    const api = fakeApi();
    api.listLogs.mockImplementationOnce((_query, signal) => captureSignal(logs, signal));
    const { state, wrapper } = mountComposable(api);

    const refresh = state.refresh();
    wrapper.unmount();
    expect(logs.signal?.aborted).toBe(true);
    logs.resolve(usageResponse("after-unmount"));
    await refresh;
    expect(state.logs.value).toEqual([]);
  });
});

function fakeApi() {
  return {
    listLogs: vi.fn<AdminUsageApi["listLogs"]>().mockResolvedValue(usageResponse("default")),
    getDetail: vi.fn<AdminUsageApi["getDetail"]>().mockResolvedValue(usageDetail("default")),
    listSummary: vi.fn<AdminUsageApi["listSummary"]>().mockResolvedValue({ items: [], total: 0 }),
    listUnitSummary: vi.fn<AdminUsageApi["listUnitSummary"]>().mockResolvedValue({ items: [], total: 0 }),
    listUpstreamSummary: vi.fn<AdminUsageApi["listUpstreamSummary"]>().mockResolvedValue({ items: [], total: 0 }),
    listUserRanking: vi.fn<AdminUsageApi["listUserRanking"]>().mockResolvedValue({
      items: [],
      total: 0,
      included: { tenants: {}, users: {} }
    }),
    listDailyTrend: vi.fn<AdminUsageApi["listDailyTrend"]>().mockResolvedValue({ items: [], total: 0 })
  };
}

function mountComposable(api: AdminUsageApi) {
  let state: ReturnType<typeof useAdminUsageExplorer> | undefined;
  const wrapper = mount(defineComponent({
    setup() {
      state = useAdminUsageExplorer({
        api,
        auth: { userType: 1, userInfo: null },
        immediate: false,
        onError: messageError
      });
      return () => h("div");
    }
  }));
  if (!state) throw new Error("composable did not initialize");
  return { state, wrapper };
}

function usageResponse(requestId: string): UsageLogsResponse {
  return {
    total: 1,
    stats: {
      total_requests: 1,
      success_count: 1,
      failed_count: 0,
      total_tokens: 1,
      total_catalog_base_credits: 1,
      total_tenant_payable_credits: 1,
      total_user_charged_credits: 1,
      avg_latency_ms: 1,
      avg_request_total_ms: 1,
      avg_first_response_byte_ms: 1
    },
    records: [usageRow(requestId)],
    included: {
      tenants: { tenant: { tenant_id: "tenant", tenant_name: "Tenant" } },
      users: { user: { user_id: "user", tenant_id: "tenant", username: "alice" } }
    }
  };
}

function usageRow(requestId: string): UsageLogDTO {
  return {
    request_id: requestId,
    tenant_id: "tenant",
    user_id: "user",
    request_source: "api_key",
    model_code: "gpt-4o",
    request_status: "success"
  } as unknown as UsageLogDTO;
}

function usageDetail(requestId: string): UsageDetailResponse {
  return { request_id: requestId, request_status: "success" } as unknown as UsageDetailResponse;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject, signal: undefined as AbortSignal | undefined };
}

function captureSignal<T>(target: ReturnType<typeof deferred<T>>, signal?: AbortSignal): Promise<T> {
  target.signal = signal;
  return target.promise;
}
