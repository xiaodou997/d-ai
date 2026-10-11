import { type Component } from "vue";
import { formatDisplayMicroUSD, formatDisplayUSD } from "@/shared/currency";
import type { TenantBalanceLedgerItem } from "@/api/types/tenant";
import { balanceTransactionText } from "@/features/platform/account-center/model";

export type TagTone = "neutral" | "accent" | "positive" | "warning" | "danger" | "info";

export interface FinancialMetric {
  key: string;
  label: string;
  value: string;
  hint: string;
  badge: string;
  tone: "accent" | "positive" | "warning" | "danger";
  icon: Component;
}

export interface FinanceUserRow {
  userId: string;
  username: string;
  balanceUsd: number | null;
  consumptionUsd: number;
  transactionCount: number;
  percentage: string;
  statusLabel: string;
  statusTone: TagTone;
}

export interface FinanceSignal {
  key: string;
  title: string;
  detail: string;
  action: string;
  tone: TagTone;
  icon: Component;
  path: string;
}

export interface FlowRow {
  key: string;
  label: string;
  detail: string;
  value: string;
  tone: "income" | "expense" | "neutral";
  icon: Component;
}

export const numberFormatter = new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 });

export const decimalFormatter = new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 1 });

export function buildUserRow(input: Omit<FinanceUserRow, "statusLabel" | "statusTone">): FinanceUserRow {
  if (input.balanceUsd === null) {
    return { ...input, statusLabel: "待同步余额", statusTone: "neutral" };
  }
  if (input.balanceUsd <= 0) {
    return { ...input, statusLabel: "余额不足", statusTone: "danger" };
  }
  if (input.balanceUsd <= 1) {
    return { ...input, statusLabel: "余额偏低", statusTone: "warning" };
  }
  return { ...input, statusLabel: "余额正常", statusTone: "positive" };
}

export function userStatusWeight(row: FinanceUserRow) {
  if (row.statusTone === "danger") return 3;
  if (row.statusTone === "warning") return 2;
  if (row.statusTone === "neutral") return 1;
  return 0;
}

export function sourceLabel(value: string) {
  return (
    {
      api_key: "API 调用",
      web_chat: "网页对话",
      web_image: "网页生图",
      workspace: "工作台",
      "": "D-AI"
    } as Record<string, string>
  )[value] || value || "其他来源";
}

export function formatUSD(value: number | null | undefined) {
  return formatDisplayUSD(value);
}

export function formatMicroUSD(value: number | null | undefined) {
  return formatDisplayMicroUSD(value);
}

export function signedMoney(value: number | null | undefined) {
  const numeric = Number(value) || 0;
  return `${numeric < 0 ? "-" : "+"}${formatUSD(Math.abs(numeric))}`;
}

export function formatPercent(value: number | null | undefined) {
  if (value === null || value === undefined || !Number.isFinite(Number(value))) return "—";
  return `${Number(value).toFixed(1).replace(/\.0$/, "")}%`;
}

export function formatCount(value: number | null | undefined) {
  return numberFormatter.format(Number(value) || 0);
}

export function formatCompactTokens(value: number | null | undefined) {
  const numeric = Number(value) || 0;
  if (Math.abs(numeric) < 1000) return formatCount(numeric);
  if (Math.abs(numeric) < 1_000_000) return `${(numeric / 1000).toFixed(1).replace(/\.0$/, "")}K`;
  return `${(numeric / 1_000_000).toFixed(1).replace(/\.0$/, "")}M`;
}

export function formatTime(value?: number | string | null) {
  if (!value) return "—";
  return new Date(value).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false
  });
}

export function formatShortDate(value?: number | string | null) {
  if (!value) return "—";
  return new Date(value).toLocaleDateString("zh-CN", { month: "2-digit", day: "2-digit" });
}

export function ledgerAmountClass(value: number) {
  return value >= 0 ? "amount-positive" : "amount-negative";
}

export function ledgerAmount(value: number) {
  const numeric = Number(value) || 0;
  return `${numeric >= 0 ? "+" : "-"}${formatMicroUSD(Math.abs(numeric))}`;
}

export function ledgerType(item: TenantBalanceLedgerItem) {
  return balanceTransactionText(item.txnType);
}

export function ledgerNote(item: TenantBalanceLedgerItem) {
  return item.note || item.refId || "—";
}
