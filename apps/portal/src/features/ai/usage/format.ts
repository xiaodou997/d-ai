import type {
  DailyTrendRowDTO,
  UsageLogDTO,
  UsageSummaryRowDTO,
  UsageUnitSummaryRowDTO
} from "./model";

export interface UsageSummaryTotals {
  requestCount: number;
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
  /** 目录基准价合计（倍率 1），谁都不付这个数，仅作参照。 */
  catalogBase: number;
  /** 平台向租户应收合计。 */
  tenantPayable: number;
  userCharged: number;
  quotaCost: number;
}

export interface UsageDistributionItem {
  name: string;
  amountUSD: number;
  requests: number;
  units?: number;
  percent: number;
}

export interface UsageCostSecondaryItem {
  label: string;
  value: string;
}

export function formatNumber(value: number | string | null | undefined): string {
  const numeric = Number(value) || 0;
  return numeric.toLocaleString("zh-CN", {
    minimumFractionDigits: 0,
    maximumFractionDigits: 4
  });
}

export function formatUSD(value: number | string | null | undefined): string {
  const numeric = Number(value) || 0;
  return `$${numeric.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 6 })}`;
}

export function formatPercent(value: number, digits = 1): string {
  if (!Number.isFinite(value)) return "0%";
  return `${value.toFixed(digits)}%`;
}

export function formatTimestamp(value: number | string | null | undefined): string {
  if (!value) return "—";
  return new Date(value).toLocaleString("zh-CN", { hour12: false });
}

export function formatShortDate(value: string): string {
  if (!value) return "—";
  return value.slice(5);
}

export function formatJSON(value: unknown): string {
  if (value == null || value === "") return "—";
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

export function unitLabel(unit: string | null | undefined): string {
  const map: Record<string, string> = {
    token: "按量（Token）",
    input_token: "按输入（Token）",
    output_token: "按输出（Token）",
    image: "按次（图片）",
    second: "按时长（秒）",
    request: "按次（请求）"
  };
  return map[unit || ""] || unit || "—";
}

export function modelRouteLabel(row: Pick<UsageLogDTO, "requested_model" | "model_code" | "resolved_logical_model">): string {
  const requested = row.requested_model || row.model_code;
  const resolved = row.resolved_logical_model || row.model_code;
  return requested === resolved ? requested : `${requested} → ${resolved}`;
}

export function costSecondary(row: Pick<UsageLogDTO, "tenant_payable_usd" | "api_key_quota_usd" | "catalog_base_usd">): UsageCostSecondaryItem[] {
  const items: UsageCostSecondaryItem[] = [
    { label: "租户结算应收", value: formatUSD(row.tenant_payable_usd) },
    { label: "Key", value: formatUSD(row.api_key_quota_usd) },
    { label: "上游参考成本", value: formatUSD(row.catalog_base_usd) }
  ];
  return items.filter((item) => item.value !== "0");
}

export function resolveRequestTotalMs(
  row: Pick<
    UsageLogDTO,
    "request_total_ms" | "final_attempt_total_ms" | "first_token_latency_ms" | "latency_ms"
  >
): number {
  return Number(row.request_total_ms ?? row.final_attempt_total_ms ?? row.first_token_latency_ms ?? row.latency_ms ?? 0) || 0;
}

export function resolveFirstResponseByteMs(
  row: Pick<UsageLogDTO, "first_response_byte_ms" | "first_token_latency_ms">
): number {
  // Admin records currently persist the gateway's first response byte milestone;
  // keep this distinct from first-token latency to avoid presenting it as a token metric.
  return Number(row.first_response_byte_ms ?? row.first_token_latency_ms ?? 0) || 0;
}

export function resolveFirstTokenMs(row: Pick<UsageLogDTO, "first_token_latency_ms">): number {
  return Number(row.first_token_latency_ms ?? 0) || 0;
}

export function resolveHeaderMs(
  row: Pick<UsageLogDTO, "final_attempt_header_ms" | "latency_ms">
): number {
  return Number(row.final_attempt_header_ms ?? row.latency_ms ?? 0) || 0;
}

export function resolveRequestSetupMs(
  row: Pick<UsageLogDTO, "request_setup_ms">
): number {
  return Number(row.request_setup_ms ?? 0) || 0;
}

export function resolveResponseTailMs(
  row: Pick<UsageLogDTO, "response_tail_ms">
): number {
  return Number(row.response_tail_ms ?? 0) || 0;
}

export function buildSummaryTotals(summaryRows: UsageSummaryRowDTO[]): UsageSummaryTotals {
  return summaryRows.reduce<UsageSummaryTotals>(
    (acc, row) => {
      acc.requestCount += Number(row.request_count) || 0;
      acc.promptTokens += Number(row.total_prompt_tokens) || 0;
      acc.completionTokens += Number(row.total_completion_tokens) || 0;
      acc.totalTokens += Number(row.total_tokens) || 0;
      acc.catalogBase += Number(row.total_catalog_base_usd) || 0;
      acc.tenantPayable += Number(row.total_tenant_payable_usd) || 0;
      acc.userCharged += Number(row.total_user_charged_usd) || 0;
      acc.quotaCost += Number(row.total_quota_usd) || 0;
      return acc;
    },
    {
      requestCount: 0,
      promptTokens: 0,
      completionTokens: 0,
      totalTokens: 0,
      catalogBase: 0,
      tenantPayable: 0,
      userCharged: 0,
      quotaCost: 0
    }
  );
}

export function buildModelDistribution(summaryRows: UsageSummaryRowDTO[], totalUserCost: number): UsageDistributionItem[] {
  const sorted = [...summaryRows].sort((a, b) => Number(b.total_user_charged_usd || 0) - Number(a.total_user_charged_usd || 0));
  const top = sorted.slice(0, 6);
  const rest = sorted.slice(6);
  const items = top.map((row) => ({
    name: row.model_code,
    amountUSD: Number(row.total_user_charged_usd) || 0,
    requests: Number(row.request_count) || 0,
    percent: totalUserCost ? ((Number(row.total_user_charged_usd) || 0) * 100) / totalUserCost : 0
  }));

  if (rest.length) {
    items.push({
      name: `其他 ${rest.length} 个`,
      amountUSD: rest.reduce((sum, row) => sum + (Number(row.total_user_charged_usd) || 0), 0),
      requests: rest.reduce((sum, row) => sum + (Number(row.request_count) || 0), 0),
      percent: totalUserCost
        ? (rest.reduce((sum, row) => sum + (Number(row.total_user_charged_usd) || 0), 0) * 100) / totalUserCost
        : 0
    });
  }

  return items;
}

export function buildUnitDistribution(unitRows: UsageUnitSummaryRowDTO[], totalUserCost: number): UsageDistributionItem[] {
  return [...unitRows]
    .sort((a, b) => Number(b.total_user_charged_usd || 0) - Number(a.total_user_charged_usd || 0))
    .map((row) => ({
      name: unitLabel(row.billable_unit_type),
      amountUSD: Number(row.total_user_charged_usd) || 0,
      units: Number(row.total_billable_units) || 0,
      requests: Number(row.request_count) || 0,
      percent: totalUserCost ? ((Number(row.total_user_charged_usd) || 0) * 100) / totalUserCost : 0
    }));
}

export function buildSlowLogs<T extends UsageLogDTO>(logs: T[]): T[] {
  return [...logs]
    .filter((row) => resolveRequestTotalMs(row) > 0)
    .sort((a, b) => resolveRequestTotalMs(b) - resolveRequestTotalMs(a))
    .slice(0, 6);
}

export function buildFailedLogs<T extends UsageLogDTO>(logs: T[]): T[] {
  return logs.filter((row) => row.request_status !== "success").slice(0, 6);
}

/**
 * 运行时从设计 token 解析颜色；无 DOM（SSR / vitest）时返回空串，
 * 由图表组件侧自行 fallback。
 */
function resolveTokenColor(token: string): string {
  if (typeof document === "undefined" || typeof getComputedStyle !== "function") return "";
  return getComputedStyle(document.documentElement).getPropertyValue(token).trim();
}

export function buildRequestTrendSeries(rows: DailyTrendRowDTO[]) {
  return [
    { key: "request_count", color: resolveTokenColor("--ds-info"), label: "总请求" },
    { key: "success_count", color: resolveTokenColor("--ds-positive"), label: "成功" },
    { key: "failed_count", color: resolveTokenColor("--ds-danger"), label: "失败" }
  ].map((series) => ({
    ...series,
    points: rows.map((row) => Number(row[series.key as keyof DailyTrendRowDTO]) || 0)
  }));
}

export function buildTokenTrendSeries(rows: DailyTrendRowDTO[]) {
  return [
    { key: "prompt_tokens", color: resolveTokenColor("--ds-accent"), label: "输入 Token" },
    { key: "completion_tokens", color: resolveTokenColor("--ds-warning"), label: "输出 Token" }
  ].map((series) => ({
    ...series,
    points: rows.map((row) => Number(row[series.key as keyof DailyTrendRowDTO]) || 0)
  }));
}
