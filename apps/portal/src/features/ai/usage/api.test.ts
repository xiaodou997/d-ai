import { beforeEach, describe, expect, it, vi } from "vitest";

import { createAdminUsageApi, createCustomerUsageApi, createTenantUsageApi } from "./api";

const adapter = vi.fn();

beforeEach(() => adapter.mockReset());

describe("usage generated operation clients", () => {
  it("binds admin usage queries and detail path parameters", async () => {
    adapter
      .mockResolvedValueOnce({})
      .mockResolvedValueOnce({});
    const api = createAdminUsageApi(adapter);
    const signal = new AbortController().signal;

    await api.listLogs({ date_from: "2026-01-01T00:00:00Z", limit: 20 }, signal);
    await api.getDetail("request/1", signal);

    expect(adapter.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/usage-logs",
      query: { date_from: "2026-01-01T00:00:00Z", limit: 20 },
      signal
    });
    expect(adapter.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/usage-logs/request%2F1",
      pathParams: { requestID: "request/1" },
      signal
    });
  });

  it("uses tenant operations for records/summary and the generated user page operation", async () => {
    adapter
      .mockResolvedValueOnce({})
      .mockResolvedValueOnce({})
      .mockResolvedValueOnce({});
    const api = createTenantUsageApi(adapter, adapter);

    await api.listRecords({ limit: 20 });
    await api.listSummary({ request_source: "api_key" });
    await api.listUsers();

    expect(adapter.mock.calls[0]?.[0]).toMatchObject({ path: "/api/v1/tenants/me/usage-logs", query: { limit: 20 } });
    expect(adapter.mock.calls[1]?.[0]).toMatchObject({ path: "/api/v1/tenants/me/usage-summary", query: { request_source: "api_key" } });
    expect(adapter.mock.calls[2]?.[0]).toMatchObject({
      path: "/api/v1/users",
      query: { page: 1, size: 200 }
    });
  });

  it("forwards customer usage source filters through generated operations", async () => {
    adapter.mockResolvedValueOnce({}).mockResolvedValueOnce({});
    const api = createCustomerUsageApi(adapter);

    await api.listRecords({ request_source: "web_chat", limit: 10 });
    await api.getSummary("web_chat");

    expect(adapter.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/user-usage-logs",
      query: { request_source: "web_chat", limit: 10 }
    });
    expect(adapter.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/user-usage-summary",
      query: { request_source: "web_chat" }
    });
  });
});
