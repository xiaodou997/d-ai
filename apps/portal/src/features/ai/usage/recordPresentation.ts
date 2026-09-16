import { formatDuration as ms } from "@/platform/ai/usage";
import { ElMessage } from "element-plus";
import type { DsTableColumn } from "@/shared/ui";
import { microUSD, type RequestRecord, type RecordSummary } from "./recordsApi";
export type RecordRole = "admin" | "tenant" | "customer";
export const recordRole = (userType: unknown): RecordRole => Number(userType) === 4 ? "customer" : Number(userType) === 3 ? "tenant" : "admin";
export const recordPath = (role: RecordRole) => role === "customer" ? "/customer/usage" : `/${role}/ai/usage`;
export const recordTitle = (role: RecordRole) => role === "tenant" ? "使用分析" : "使用记录";
export const sourceLabel = (value: string) => ({ api_key: "API Key", web_chat: "网页对话", web_image: "网页生图", web_video: "网页视频", workspace: "工作台", internal: "内部调用" } as Record<string, string>)[value] || value || "—";
export { formatDuration as ms } from "@/platform/ai/usage";
export const compactCount = (value?: number | null) => value == null ? "未报告" : Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 1 }).format(value);
export const compactMs = ms;
export const multiplier = (value?: number | null) => value == null ? "—" : `× ${value}`;
export const timestamp = (value?: string | null) => value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "—";
export const deliveryLabel = (value: string) => ({ complete: "响应已交付", disconnected: "客户端连接中断", write_failed: "响应写入失败", unknown: "未确认完整交付" } as Record<string, string>)[value] || value;
export async function copyRecordText(value: string) {
  try { await navigator.clipboard.writeText(value); ElMessage.success("已复制"); }
  catch { ElMessage.error("复制失败，请手动选择复制"); }
}
export function recordColumns(role: RecordRole, errors: boolean, fixedUser = false): DsTableColumn[] {
  return [
    { key: "time", title: "时间", width: 115 },
    ...(role !== "customer" && !fixedUser ? [{ key: "subject", title: "调用主体", width: 155 }] : []),
    { key: "model", title: "模型与接入", width: 185 },
    { key: "profile", title: "分组与 Key", width: 170 },
    ...(role === "admin" ? [{ key: "upstream", title: "上游", width: 150 }] : []),
    ...(!errors ? [{ key: "tokens", title: "用量", width: 175 }, { key: "timing", title: "耗时", width: 140 }] : []),
    ...(errors ? [{ key: "error", title: "错误说明", width: 240, wrap: true }] : []),
    ...(!errors ? [{ key: "charge", title: "费用", width: 165 }] : []),
    { key: "actions", title: "操作", width: 95 }
  ];
}
export function recordMetrics(s: RecordSummary | null, role: RecordRole, rangeLabel: string) {
  const count = (n?: number | null) => n == null ? "—" : n.toLocaleString();
  const money = (label: string, charged?: number | null, refunded?: number | null) => ({ label, value: microUSD(charged), hint: `已退 ${microUSD(refunded)} · 净扣 ${charged == null || refunded == null ? "—" : microUSD(charged - refunded)}` });
  const common = [
    { label: "请求次数", value: count(s?.requests), hint: role === "customer" ? `平均耗时 ${ms(s?.avg_total_ms)}` : `${rangeLabel} · 含正常与错误请求` },
    { label: "异常情况", value: count(s?.errors), hint: `明确错误 · 客户端中断 ${count(s?.interruptions)}` },
    { label: "Token 用量", value: s?.token_samples ? count(s.total_tokens) : "未报告", hint: s?.token_samples ? `输入 ${count(s.input_tokens)} / 输出 ${count(s.output_tokens)} · 缓存读 ${count(s.cache_read_tokens)} / 写 ${count(s.cache_write_tokens)}` : "当前范围无已报告 Token" }
  ];
  if (role !== "customer") common.push(money("租户实际扣款", s?.tenant_charged_micro, s?.tenant_refunded_micro));
  common.push(money(role === "customer" ? "我的扣款" : "用户实际扣款", s?.user_charged_micro, s?.user_refunded_micro));
  if (role !== "customer") common.push({ label: "平均耗时", value: ms(s?.avg_total_ms), hint: `${count(s?.timing_samples)} 个有效样本 · 首 Token ${ms(s?.avg_first_token_ms)}（${count(s?.first_token_samples)} 个样本）` });
  return common;
}
export const object = (value: unknown): Record<string, unknown> => value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
export function receiptFacts(record: RequestRecord, role: RecordRole): Array<{ label: string; value: string }> {
  const pricing = object(record.pricing), snapshot = object(pricing.snapshot);
  const retail = object(pricing.retail_price || snapshot.RetailEntry);
  const facts: Array<{ label: string; value: string }> = [];
  for (const [label, raw] of [["分组倍率", pricing.user_multiplier ?? snapshot.GroupDefaultUserMultiplier ?? snapshot.EffectiveUserMultiplier], ["订阅额度倍率", pricing.subscription_multiplier], ...(role !== "customer" ? [["租户倍率", pricing.tenant_multiplier]] : [])]) {
    if (typeof raw === "number") facts.push({ label: String(label), value: multiplier(raw) });
  }
  const calculation = object(pricing.calculation);
  const userLine = object(pricing.user_breakdown || calculation.user_payable);
  const priceLines = object(userLine.price_lines);
  if (Object.keys(priceLines).length) {
    if (typeof priceLines.token_price_tier_index === "number") {
      const index = Number(priceLines.token_price_tier_index);
      const tiers = Array.isArray(retail.TokenPriceTiers) ? retail.TokenPriceTiers.map(object) : [];
      const base = Number(tiers[0]?.input_per_token || 0) * 1_000_000;
      const selected = Number(priceLines.input_unit_price_per_1m_usd || 0);
      const ratio = index > 0 && base > 0 ? selected / base : 1;
      const uplift = ratio > 1 ? ` · 输入价 ×${Number(ratio.toFixed(2))}` : "";
      facts.push({ label: "命中档位", value: `第 ${index + 1} 档${uplift}` });
    }
    for (const [key, label] of [["input", "输入"], ["output", "输出"], ["cache_read", "缓存读"], ["cache_write", "缓存写"]]) {
      const value = priceLines[`${key}_unit_price_per_1m_usd`];
      const count = priceLines[`${key}_tokens`];
      if (typeof value === "number" && typeof count === "number" && (Number(count) > 0 || key === "input" || key === "output")) facts.push({ label: `${label} ${compactCount(Number(count))} Token`, value: `$${value} / 百万 Token` });
    }
    for (const [key, label, unit] of [["image", "图像", "图"], ["video", "视频", "秒"]]) {
      const value = priceLines[`${key}_unit_price_usd`];
      if (typeof value === "number") facts.push({ label: `${label}命中单价`, value: `$${value} / ${unit} · ${priceLines[`${key}_resolution`] || "默认规格"}` });
    }
  }
  return facts;
}
