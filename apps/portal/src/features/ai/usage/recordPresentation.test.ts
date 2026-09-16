import { describe, expect, it } from "vitest";
import { compactCount, recordColumns, recordMetrics, receiptFacts } from "./recordPresentation";
import type { RequestRecord, RecordSummary } from "./recordsApi";
describe("role-specific record presentation", () => {
  it("keeps two-level fees and internal routing separate from customer data", () => {
    const admin = recordColumns("admin", false);
    const customer = recordColumns("customer", false);
    expect(admin.map(c => c.key)).toContain("upstream");
    expect(admin.map(c => c.title)).not.toContain("结果");
    expect(customer.map(c => c.key)).not.toContain("upstream");
    expect(customer.map(c => c.key)).not.toContain("subject");
    const errors = recordColumns("tenant", true).map(c => c.key);
    expect(errors).toContain("error");
    expect(errors).not.toContain("tokens");
    expect(errors).not.toContain("timing");
    expect(errors).not.toContain("charge");
  });
  it("shows charged, refunded and net amounts without adding quota usage", () => {
    const summary = { requests: 2, errors: 0, interruptions: 1, token_samples: 1, total_tokens: 35, input_tokens: 10, output_tokens: 20, cache_read_tokens: 3, cache_write_tokens: 2, tenant_charged_micro: 1000000, tenant_refunded_micro: 250000, user_charged_micro: 2000000, user_refunded_micro: 500000, avg_total_ms: 123, avg_first_token_ms: 10, timing_samples: 1, first_token_samples: 1 } satisfies RecordSummary;
    const admin = recordMetrics(summary, "admin", "今天");
    expect(admin).toHaveLength(6);
    expect(admin.find(m => m.label === "租户实际扣款")).toMatchObject({ value: "$1.000000", hint: "已退 $0.250000 · 净扣 $0.750000" });
    const customer = recordMetrics(summary, "customer", "今天");
    expect(customer).toHaveLength(4);
    expect(customer.find(m => m.label === "我的扣款")?.value).toBe("$2.000000");
    expect(JSON.stringify(customer)).not.toContain("租户");
    expect(recordMetrics(null, "customer", "今天")[2].value).toBe("未报告");
  });
  it("shows the stored selected tier without selecting from today's price book", () => {
    const record = { tokens: { input: 10 }, pricing: { retail_price: { TokenPriceTiers: [{ input_per_token: 0.0000025 }, { input_per_token: 0.000005 }] }, user_breakdown: { raw_usd: 0.25, price_lines: { input_context_tokens: 100, token_price_tier_index: 1, input_tokens: 80, output_tokens: 20, cache_read_tokens: 0, cache_write_tokens: 0, input_unit_price_per_1m_usd: 5, output_unit_price_per_1m_usd: 15, cache_read_unit_price_per_1m_usd: 0.5, cache_write_unit_price_per_1m_usd: 6.25 } } } } as unknown as RequestRecord;
    const facts = receiptFacts(record, "customer");
    expect(facts).toContainEqual({ label: "命中档位", value: "第 2 档 · 输入价 ×2" });
    expect(facts).toContainEqual({ label: "输入 80 Token", value: "$5 / 百万 Token" });
    expect(facts).toContainEqual({ label: "输出 20 Token", value: "$15 / 百万 Token" });
    expect(facts.some(fact => fact.label.startsWith("缓存"))).toBe(false);
  });
  it("uses K/M/B abbreviations for dense table values", () => {
    expect(compactCount(120_728)).toBe("120.7K");
    expect(compactCount(6_768_761)).toBe("6.8M");
    expect(compactCount(1_200_000_000)).toBe("1.2B");
  });
});
