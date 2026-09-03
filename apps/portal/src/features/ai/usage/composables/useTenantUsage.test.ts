import { defineComponent, h } from "vue";
import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { TenantUsageApi } from "../api";
import type { TenantUsageLog } from "../model";
import { useTenantUsage } from "./useTenantUsage";

const messageError = vi.fn();

type RecordsResponse = Awaited<ReturnType<TenantUsageApi["listRecords"]>>;
type UsersResponse = Awaited<ReturnType<TenantUsageApi["listUsers"]>>;

describe("useTenantUsage", () => {
  beforeEach(() => messageError.mockReset());

  it("sends server pagination filters and restores defaults on reset", async () => {
    const api = fakeApi();
    const { state, wrapper } = mountComposable(api);
    const from = new Date("2026-06-01T00:00:00.000Z").getTime();
    const to = new Date("2026-06-07T23:59:59.999Z").getTime();
    state.filters.dateRange = [from, to];
    state.filters.userId = "user-7";
    state.filters.modelCode = "gpt-4o";
    state.filters.requestStatus = "failed";
    state.filters.requestSource = "api_key";

    await state.search();
    expect(api.listRecords).toHaveBeenLastCalledWith({
      limit: 20,
      offset: 0,
      user_id: "user-7",
      model_code: "gpt-4o",
      request_status: "failed",
      request_source: "api_key",
      date_from: new Date(from).toISOString(),
      date_to: new Date(to).toISOString()
    }, expect.any(AbortSignal));

    await state.changePage(3);
    expect(api.listRecords).toHaveBeenLastCalledWith(expect.objectContaining({ limit: 20, offset: 40 }), expect.any(AbortSignal));

    await state.changePageSize(50);
    expect(state.page.value).toBe(1);
    expect(api.listRecords).toHaveBeenLastCalledWith(expect.objectContaining({ limit: 50, offset: 0 }), expect.any(AbortSignal));

    await state.reset();
    expect(state.filters.userId).toBe("");
    expect(state.filters.modelCode).toBe("");
    expect(state.filters.requestStatus).toBe("");
    expect(state.filters.requestSource).toBe("");
    expect(state.filters.dateRange).not.toBeNull();
    expect(api.listRecords).toHaveBeenLastCalledWith(expect.objectContaining({
      user_id: undefined,
      model_code: undefined,
      request_status: undefined,
      request_source: undefined,
      offset: 0
    }), expect.any(AbortSignal));
    wrapper.unmount();
  });

  it("enriches users from the directory and preserves external and id fallbacks", async () => {
    const api = fakeApi();
    api.listRecords.mockResolvedValue(tenantResponse([
      tenantRow("known", { user_id: "known-user" }),
      tenantRow("email", { user_id: "email-user" }),
      tenantRow("unknown", { user_id: "unknown-user" }),
      tenantRow("external", { user_id: undefined, external_user_id: "external-42" }),
      tenantRow("record-name", { user_id: "record-user", username: "record-name" })
    ]));
    api.listUsers.mockResolvedValue(usersResponse([
      { userId: "known-user", username: "alice", email: "alice@example.test" },
      { userId: "email-user", username: "", email: "email-only@example.test" },
      { userId: "record-user", username: "directory-name", email: "record@example.test" }
    ]));
    const { state, wrapper } = mountComposable(api);

    await state.loadRecords();
    expect(state.rows.value.map((row) => row.userLabel)).toEqual([
      "known-user",
      "email-user",
      "unknown-user",
      "external-42",
      "record-name"
    ]);

    await state.loadUsers();
    expect(api.listUsers).toHaveBeenLastCalledWith(expect.any(AbortSignal));
    expect(state.rows.value.map((row) => row.userLabel)).toEqual([
      "alice",
      "email-only@example.test",
      "unknown-user",
      "external-42",
      "record-name"
    ]);
    wrapper.unmount();
  });

  it("keeps the newest records when responses resolve out of order", async () => {
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
    second.resolve(tenantResponse([tenantRow("newest")]));
    await secondLoad;
    first.resolve(tenantResponse([tenantRow("stale")]));
    await firstLoad;
    expect(state.rows.value[0]?.request_id).toBe("newest");
    expect(messageError).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("aborts records and directory requests on unmount without notifying AbortError", async () => {
    const records = deferred<RecordsResponse>();
    const users = deferred<UsersResponse>();
    const api = fakeApi();
    api.listRecords.mockImplementationOnce((_query, signal) => captureSignal(records, signal));
    api.listUsers.mockImplementationOnce((signal) => captureSignal(users, signal));
    const { state, wrapper } = mountComposable(api);

    const recordsLoad = state.loadRecords();
    const usersLoad = state.loadUsers();
    wrapper.unmount();
    expect(records.signal?.aborted).toBe(true);
    expect(users.signal?.aborted).toBe(true);
    records.reject(new DOMException("cancelled", "AbortError"));
    users.reject(new DOMException("cancelled", "AbortError"));
    await Promise.all([recordsLoad, usersLoad]);
    expect(state.rows.value).toEqual([]);
    expect(state.users.value).toEqual([]);
    expect(messageError).not.toHaveBeenCalled();
  });
});

function fakeApi() {
  return {
    listRecords: vi.fn<TenantUsageApi["listRecords"]>().mockResolvedValue(tenantResponse([])),
    listSummary: vi.fn<TenantUsageApi["listSummary"]>().mockResolvedValue({ items: [], total: 0 }),
    listUsers: vi.fn<TenantUsageApi["listUsers"]>().mockResolvedValue(usersResponse([]))
  };
}

function mountComposable(api: TenantUsageApi) {
  let state: ReturnType<typeof useTenantUsage> | undefined;
  const wrapper = mount(defineComponent({
    setup() {
      state = useTenantUsage({ api, immediate: false, onError: messageError });
      return () => h("div");
    }
  }));
  if (!state) throw new Error("composable did not initialize");
  return { state, wrapper };
}

function tenantResponse(records: TenantUsageLog[]): RecordsResponse {
  return {
    records,
    total: records.length,
    stats: {
      total_requests: records.length,
      success_count: records.length,
      failed_count: 0,
      total_tokens: 0,
      total_catalog_base_usd: 0,
      total_tenant_payable_usd: 0,
      total_user_charged_usd: 0,
      avg_latency_ms: 0,
      avg_request_total_ms: 0,
      avg_first_response_byte_ms: 0
    }
  };
}

function tenantRow(requestId: string, overrides: Partial<TenantUsageLog> = {}): TenantUsageLog {
  return {
    request_id: requestId,
    tenant_id: "tenant",
    user_id: "user",
    request_source: "api_key",
    model_code: "gpt-4o",
    request_status: "success",
    ...overrides
  } as unknown as TenantUsageLog;
}

function usersResponse(items: Array<{ userId: string; username: string; email: string }>): UsersResponse {
  return { items, total: items.length, page: 1, size: 200 } as unknown as UsersResponse;
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
