import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/api",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { aiAdminApi } from "./aiAdmin";

beforeEach(() => mocks.request.mockReset());

const riskConfig = {
  enabled: true,
  mode: "observe",
  config_revision: 4,
  keyword: {
    enabled: true,
    entries: null,
    homoglyph_map_extra: { "ⓐ": "a" },
    pinyin: { enabled: true, entries: [{ word: "bad", level: "block", require_with: null, note: "test" }], include_initials: false }
  },
  provider: { base_url: "https://moderation.example.com", model: "omni", has_api_key: true, timeout_ms: 5000 },
  thresholds: { violence: 0.8 },
  sample_rate: 1,
  verdict_cache_ttl_seconds: 600,
  scope_group_ids: null,
  violation_window_hours: 24,
  risk_event_threshold: 3,
  record_non_hits: false,
  block_status_code: 451,
  block_message: "blocked",
  $schema: "ignored"
};

const configWrite = {
  enabled: true,
  mode: "pre_block" as const,
  keyword: {
    enabled: true,
    entries: [{ word: "bad", level: "block" as const, require_with: [], note: "test" }],
    homoglyph_map_extra: {},
    pinyin: { enabled: false, entries: [], include_initials: false }
  },
  provider: { base_url: "https://moderation.example.com", model: "omni", api_key: "secret", timeout_ms: 5000 },
  thresholds: { violence: 0.8 },
  sample_rate: 0.5,
  verdict_cache_ttl_seconds: 600,
  scope_group_ids: [],
  violation_window_hours: 24,
  risk_event_threshold: 3,
  record_non_hits: false,
  block_status_code: 451,
  block_message: "blocked"
};

const riskEvent = {
  id: "event-1",
  event_type: "repeated_violation",
  severity: "high",
  tenant_id: "tenant-1",
  user_id: "user-1",
  source_log_id: "log-1",
  summary: "Repeated violation",
  detail: '{"category":"violence"}',
  status: "open",
  created_at: 100,
  $schema: "ignored"
};

describe("AI admin risk-control generated operation facade", () => {
  it("maps config responses and writes generated keyword/provider bodies", async () => {
    mocks.request.mockResolvedValueOnce(riskConfig).mockResolvedValueOnce({ ...riskConfig, mode: "pre_block", scope_group_ids: [] });

    await expect(aiAdminApi.getRiskControlConfig()).resolves.toMatchObject({
      mode: "observe",
      keyword: { entries: [], pinyin: { entries: [{ require_with: [] }] } },
      scope_group_ids: []
    });
    await expect(aiAdminApi.updateRiskControlConfig(configWrite)).resolves.toMatchObject({ mode: "pre_block" });

    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/risk-control/config",
      body: {
        mode: "pre_block",
        keyword: { entries: [{ word: "bad", require_with: [] }] },
        provider: { api_key: "secret", model: "omni" }
      }
    });
  });

  it("maps moderation tests and nullable log/event lists with typed query forwarding", async () => {
    mocks.request
      .mockResolvedValueOnce({ flagged: true, from_cache: false, matched_keyword: "bad", $schema: "ignored" })
      .mockResolvedValueOnce({ items: null, total: 0 })
      .mockResolvedValueOnce({ items: [riskEvent], total: 1 })
      .mockResolvedValueOnce({ ...riskEvent, status: "resolved", resolution_note: "reviewed" });

    await expect(aiAdminApi.testRiskControlModeration("bad text")).resolves.toMatchObject({ flagged: true, matched_keyword: "bad" });
    await expect(aiAdminApi.listRiskControlLogs({ tenant_id: "tenant-1", flagged: "true", limit: 100 })).resolves.toEqual({ items: [], total: 0 });
    await expect(aiAdminApi.listRiskControlEvents({ status: "open", limit: 100 })).resolves.toMatchObject({
      items: [{ id: "event-1", severity: "high", status: "open", detail: { category: "violence" } }],
      total: 1
    });
    await expect(aiAdminApi.resolveRiskControlEvent("event/1", { status: "resolved", note: "reviewed" })).resolves.toMatchObject({ status: "resolved" });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({ body: { text: "bad text" } });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      query: { tenant_id: "tenant-1", flagged: "true", limit: 100 }
    });
    expect(mocks.request.mock.calls[3]?.[0]).toMatchObject({
      path: "/api/v1/risk-control/events/event%2F1/resolve",
      pathParams: { eventID: "event/1" },
      body: { status: "resolved", note: "reviewed" }
    });
  });

  it("rejects unknown risk enums at the domain boundary", async () => {
    mocks.request.mockResolvedValueOnce({ ...riskConfig, mode: "audit" });
    await expect(aiAdminApi.getRiskControlConfig()).rejects.toThrow("Unexpected risk control mode");

    mocks.request.mockReset();
    expect(() => aiAdminApi.updateRiskControlConfig({ ...configWrite, mode: "audit" as never })).toThrow("Unexpected risk control mode");
    expect(() => aiAdminApi.resolveRiskControlEvent("event-1", { status: "open" })).toThrow("Unexpected risk event resolution status");
    expect(mocks.request).not.toHaveBeenCalled();

    mocks.request.mockResolvedValueOnce({ items: [{ ...riskEvent, severity: "critical" }], total: 1 });
    await expect(aiAdminApi.listRiskControlEvents({ limit: 10 })).rejects.toThrow("Unexpected risk event severity");
  });
});
