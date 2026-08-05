export interface PortalSelectOption<TValue extends string = string> {
  label: string;
  value: TValue;
}

export const portalStatusOptions: PortalSelectOption<"active" | "disabled">[] = [
  { label: "启用", value: "active" },
  { label: "停用", value: "disabled" }
];

export function appendPortalQuery(
  path: string,
  query?: Record<string, string | number | undefined | null>
): string {
  if (!query) return path;
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === null || value === "") continue;
    params.set(key, String(value));
  }
  const search = params.toString();
  return search ? `${path}?${search}` : path;
}

// 实际消耗/统计结果允许最多 4 位小数。
export function formatCredits(value: number | null | undefined): string {
  if (value === null || value === undefined) return "-";
  const n = Number(value) || 0;
  return n.toLocaleString(undefined, { minimumFractionDigits: 0, maximumFractionDigits: 4 });
}

export function formatMultiplier(value: number | null | undefined): string {
  if (value === null || value === undefined) return "-";
  const n = Number(value);
  if (!Number.isFinite(n)) return "-";
  return n.toLocaleString(undefined, { maximumFractionDigits: 4 });
}

// 售价/配额配置统一显示为整数积分。
export function formatWholeCredits(value: number | null | undefined): string {
  if (value === null || value === undefined) return "-";
  const n = Number(value) || 0;
  return Math.round(n).toLocaleString(undefined, { maximumFractionDigits: 0 });
}

export const MICRO_CREDITS_PER_CREDIT = 10_000;

export function formatMicroCredits(value: number | null | undefined): string {
  if (value === null || value === undefined) return "-";
  return formatCredits(Number(value) / MICRO_CREDITS_PER_CREDIT);
}
