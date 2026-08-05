import { defineComponent, h } from "vue";
import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { CustomerUsageApi } from "../api";
import type { CustomerUsageLog } from "../model";
import { useCustomerUsage } from "./useCustomerUsage";

const messageError = vi.fn();

type RecordsResponse = Awaited<ReturnType<CustomerUsageApi["listRecords"]>>;

describe("useCustomerUsage", () => {
  beforeEach(() => messageError.mockReset());

  it("keeps source and limit on the server while filtering and paging locally", async () => {
    const records = Array.from({ length: 25 }, (_, index) => customerRow(
      `request-${index + 1}`,
      index === 14
        ? { request_status: "failed", app_name: "Beta App", error_message: "quota exceeded" }
        : { request_status: "success", app_name: "Alpha App" }
    ));
    const api = fakeApi();
    api.listRecords.mockResolvedValue(customerResponse(records));
    const { state, wrapper } = mountComposable(api);
    state.filters.limit = 50;
    state.filters.requestSource = "app";

    await state.changeServerFilter();
    expect(api.listRecords).toHaveBeenLastCalledWith({ limit: 50, request_source: "app" }, expect.any(AbortSignal));

    state.pageSize.value = 10;
    state.page.value = 2;
    expect(state.pagedRecords.value).toHaveLength(10);
    expect(state.pagedRecords.value[0]?.request_id).toBe("request-11");

    const callsBeforeLocalSearch = api.listRecords.mock.calls.length;
    state.filters.requestStatus = "failed";
    state.filters.keyword = "quota";
    state.search();
    expect(api.listRecords).toHaveBeenCalledTimes(callsBeforeLocalSearch);
    expect(state.page.value).toBe(1);
    expect(state.filteredRecords.value.map((row) => row.request_id)).toEqual(["request-15"]);
    expect(state.pagedRecords.value).toHaveLength(1);
    expect(state.stats.value.totalRequests).toBe(1);

    await state.reset();
    expect(state.filters).toMatchObject({ requestSource: "", requestStatus: "", keyword: "", limit: 100 });
    expect(state.page.value).toBe(1);
    expect(state.pageSize.value).toBe(20);
    expect(api.listRecords).toHaveBeenLastCalledWith({ limit: 100, request_source: undefined }, expect.any(AbortSignal));
    wrapper.unmount();
  });

  it("keeps the newest list when responses resolve out of order", async () => {
    const first = deferred<RecordsResponse>();
    const second = deferred<RecordsResponse>();
    const api = fakeApi();
    api.listRecords
      .mockImplementationOnce((_query, signal) => captureSignal(first, signal))
      .mockImplementationOnce((_query, signal) => captureSignal(second, signal));
    const { state, wrapper } = mountComposable(api);

    const firstLoad = state.loadRecords();
    const secondLoad = state.loadRecords();
    expect(first.signal?.aborted).toBe(true);
    second.resolve(customerResponse([customerRow("newest")]));
    await secondLoad;
    first.resolve(customerResponse([customerRow("stale")]));
    await firstLoad;
    expect(state.records.value[0]?.request_id).toBe("newest");
    expect(messageError).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("aborts in-flight work on unmount and ignores AbortError", async () => {
    const records = deferred<RecordsResponse>();
    const api = fakeApi();
    api.listRecords.mockImplementationOnce((_query, signal) => captureSignal(records, signal));
    const { state, wrapper } = mountComposable(api);

    const load = state.loadRecords();
    wrapper.unmount();
    expect(records.signal?.aborted).toBe(true);
    records.reject(new DOMException("cancelled", "AbortError"));
    await load;
    expect(state.records.value).toEqual([]);
    expect(messageError).not.toHaveBeenCalled();
  });
});

function fakeApi() {
  return {
    listRecords: vi.fn<CustomerUsageApi["listRecords"]>().mockResolvedValue(customerResponse([])),
    getSummary: vi.fn<CustomerUsageApi["getSummary"]>().mockResolvedValue({} as Awaited<ReturnType<CustomerUsageApi["getSummary"]>>)
  };
}

function mountComposable(api: CustomerUsageApi) {
  let state: ReturnType<typeof useCustomerUsage> | undefined;
  const wrapper = mount(defineComponent({
    setup() {
      state = useCustomerUsage({ api, immediate: false, onError: messageError });
      return () => h("div");
    }
  }));
  if (!state) throw new Error("composable did not initialize");
  return { state, wrapper };
}

function customerResponse(items: CustomerUsageLog[]): RecordsResponse {
  return { items, total: items.length };
}

function customerRow(requestId: string, overrides: Partial<CustomerUsageLog> = {}): CustomerUsageLog {
  return {
    id: requestId,
    request_id: requestId,
    tenant_id: "tenant",
    request_source: "api_key",
    model_code: "gpt-4o",
    prompt_tokens: 10,
    completion_tokens: 5,
    total_tokens: 15,
    billable_unit_type: "token",
    billable_units: 15,
    user_charged_credits: 1,
    request_status: "success",
    ...overrides
  } as unknown as CustomerUsageLog;
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
