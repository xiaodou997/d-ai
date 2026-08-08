const numberFormatter = new Intl.NumberFormat("zh-CN");
const currencyFormatter = new Intl.NumberFormat("zh-CN", {
  style: "currency",
  currency: "USD",
  minimumFractionDigits: 2,
  maximumFractionDigits: 2
});

export function parseTimestamp(value?: string | number | null): number | null {
  if (value == null || value === "") return null;
  if (typeof value === "number") return Number.isFinite(value) ? value : null;
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? null : parsed;
}

export function formatDateTime(value?: string | number | null): string {
  const timestamp = parseTimestamp(value);
  if (!timestamp) return "—";
  return new Date(timestamp).toLocaleString("zh-CN");
}

export function formatShortDateTime(value?: string | number | null): string {
  const timestamp = parseTimestamp(value);
  if (!timestamp) return "—";
  return new Date(timestamp).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  });
}

export function formatNumber(value?: number | null): string {
  if (value == null) return "0";
  return numberFormatter.format(value);
}

export function formatCurrencyYuanFromCent(value?: number | null): string {
  return currencyFormatter.format((value ?? 0) / 100);
}

export function formatPercent(value: number): string {
  if (!Number.isFinite(value)) return "0%";
  return `${value.toFixed(1).replace(/\.0$/, "")}%`;
}

export function formatLatency(value?: number | null): string {
  if (value == null || value <= 0) return "—";
  if (value >= 1000) return `${(value / 1000).toFixed(1)}s`;
  return `${Math.round(value)}ms`;
}
