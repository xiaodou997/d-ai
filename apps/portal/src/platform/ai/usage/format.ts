export function formatDuration(value?: number | null): string {
  if (value == null || !Number.isFinite(value) || value < 0) return "—";
  if (value < 1000) return `${Math.round(value)} ms`;
  const seconds = Math.round(value / 100) / 10;
  if (seconds < 60) return `${seconds} 秒`;
  const minutes = Math.floor(seconds / 60);
  const remainder = Math.round((seconds - minutes * 60) * 10) / 10;
  return remainder ? `${minutes} 分 ${remainder} 秒` : `${minutes} 分`;
}

/** 毫秒展示：>=1s 转秒保留 1 位，空值/0 返回 "-"。 */
export function formatMs(n?: number | null): string {
  if (n == null || n === 0) return "-";
  if (n >= 1000) return (n / 1000).toFixed(2) + " s";
  return n + " ms";
}

/** token 数展示：小于 1K 使用整数，达到 1K 后使用 K/M/B 并保留两位小数。 */
export function formatTokenCount(n?: number | null): string {
  return Number(n || 0).toLocaleString("zh-CN");
}

/** USD 金额展示：固定两位小数，超出部分按常规规则进位。 */
export function formatUSD2(n?: number | string | null): string {
  const value = Number(n) || 0;
  return `$${value.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

export function formatCompactToken(n?: number | null): string {
  const value = Number(n || 0);
  const abs = Math.abs(value);
  if (abs < 1000) {
    const rounded = Math.round(value);
    return Math.abs(rounded) >= 1000 ? `${(value / 1000).toFixed(2)}K` : String(rounded);
  }

  const units = [
    { threshold: 1e9, suffix: "B", divisor: 1e9 },
    { threshold: 1e6, suffix: "M", divisor: 1e6 },
    { threshold: 1e3, suffix: "K", divisor: 1e3 }
  ];
  let unit = units.find((candidate) => abs >= candidate.threshold) ?? units[units.length - 1];
  let scaled = value / unit.divisor;
  const rounded = Number(scaled.toFixed(2));

  // 999.99K 四舍五入后应展示为 1.00M，而不是 1000.00K。
  if (Math.abs(rounded) >= 1000) {
    const nextUnit = units[units.indexOf(unit) - 1];
    if (nextUnit) {
      unit = nextUnit;
      scaled = value / unit.divisor;
    }
  }
  return `${scaled.toFixed(2)}${unit.suffix}`;
}

/** API key 仅展示固定的安全前缀与后四位，不暴露完整密钥。 */
export function formatMaskedApiKey(lastFour?: string | null): string {
  const suffix = String(lastFour ?? "").trim();
  return suffix ? `sk-xxx${suffix}` : "—";
}

/** 表格时间列：月-日 时:分:秒。 */
export function formatUsageTimestamp(t?: number | string | null): string {
  if (!t) return "-";
  return new Date(t).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false
  });
}
