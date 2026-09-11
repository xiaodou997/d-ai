import { describe, expect, it } from "vitest";

import { normalizeUsageAttempts } from "./model";

describe("normalizeUsageAttempts", () => {
  it("retains the upstream request id for incident correlation", () => {
    expect(normalizeUsageAttempts([{ outcome: "failed", upstream_request_id: "upstream-123" }])[0]?.upstream_request_id).toBe("upstream-123");
  });
  it("reads persisted priority and retains server event order", () => {
    const rows = normalizeUsageAttempts([
      { route_id: "failed", group_id: "group-1", group_rank: 0, priority: 0, outcome: "server_error", skipped: false },
      { route_id: "skipped", priority: 10, outcome: "circuit_open", skipped: true },
      { route_id: "success", priority: 100, outcome: "success", skipped: false }
    ]);
    expect(rows.map((row) => [row.route_id, row.target_priority, row.skipped])).toEqual([
      ["failed", 0, false], ["skipped", 10, true], ["success", 100, false]
    ]);
    expect(rows[0]).toMatchObject({ group_id: "group-1", group_rank: 0 });
  });

  it("accepts the alternate field name and older records without priority", () => {
    expect(normalizeUsageAttempts([
      { target_priority: 10, outcome: "success" },
      { outcome: "success" },
      { priority: null, outcome: "success" }
    ]).map((row) => row.target_priority)).toEqual([10, undefined, undefined]);
  });
});
