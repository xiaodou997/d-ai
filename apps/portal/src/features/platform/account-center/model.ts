import type { AccountBalance } from "@/api/types/platformTenant";
import {
  formatDisplayMicroUSD,
  formatDisplayUSD,
  MICRO_USD_PER_USD
} from "@/shared/currency";

export type AccountCenterTab = "recharges" | "ledger";

export interface AccountCenterPage<T> {
  items: T[];
  total: number;
  page: number;
  size: number;
}

export { MICRO_USD_PER_USD };

export function emptyAccountBalance(): AccountBalance {
  return {
    currency: "USD", totalUsd: 0, usedUsd: 0, remainingUsd: 0,
    availableUsd: 0, permanentUsd: 0, timedUsd: 0, outstandingDebtMicroUsd: 0,
    serviceState: "active", balanceLots: []
  };
}

export function normalizeAccountTab(value: unknown): AccountCenterTab {
  return value === "recharges" ? value : "ledger";
}

export function formatUSD(value: number | null | undefined): string {
  return formatDisplayUSD(value);
}

export function formatMicroUSD(value: number | null | undefined): string {
  return formatDisplayMicroUSD(value);
}

export function formatTime(value?: number | string | null): string {
  if (!value) return "—";
  return new Date(value).toLocaleString("zh-CN");
}

export function balanceSourceText(orderType: string): string {
  return ({ platform_to_tenant: "平台发放", online_tenant_topup: "在线充值", user_topup_income: "用户充值收入" } as Record<string, string>)[orderType] ?? "余额入账";
}

export function balanceStatusText(status: string): string {
  return status === "reversed" ? "已撤销" : status === "active" ? "已到账" : status || "—";
}

export function balanceTransactionText(type: string): string {
  return ({ topup_income: "用户充值收入", refund_reversal: "退款收入冲正", consumption: "服务消费", withdraw: "提现", adjust: "余额调整" } as Record<string, string>)[type] ?? type;
}
