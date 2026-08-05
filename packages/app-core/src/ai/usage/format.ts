/** 毫秒展示：>=1s 转秒保留 1 位，空值/0 返回 "-"。 */
export function formatMs(n?: number | null): string {
  if (n == null || n === 0) return "-";
  if (n >= 1000) return (n / 1000).toFixed(2) + " s";
  return n + " ms";
}

/** token 数展示：千分位；空值返回 "0"。 */
export function formatTokenCount(n?: number | null): string {
  return Number(n || 0).toLocaleString("zh-CN");
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
