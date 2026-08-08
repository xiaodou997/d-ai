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

export function formatNumber(value: number | null | undefined): string {
  if (value === null || value === undefined) return "-";
  const n = Number(value) || 0;
  return n.toLocaleString(undefined, { minimumFractionDigits: 0, maximumFractionDigits: 4 });
}

export function formatUSD(value: number | null | undefined): string {
  if (value === null || value === undefined) return "-";
  const n = Number(value) || 0;
  return `$${n.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 6 })}`;
}

export function formatMultiplier(value: number | null | undefined): string {
  if (value === null || value === undefined) return "-";
  const n = Number(value);
  if (!Number.isFinite(n)) return "-";
  return n.toLocaleString(undefined, { maximumFractionDigits: 4 });
}

export const MICRO_USD_PER_USD = 1_000_000;
export function formatMicroUSD(value: number | null | undefined): string {
  if (value === null || value === undefined) return "-";
  return formatUSD(Number(value) / MICRO_USD_PER_USD);
}
