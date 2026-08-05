import type { AccountBalance } from "../../../types/urmTenant";
import type { TenantCashAccount } from "../../../types/tenant";

export type AccountCenterTab = "points" | "balance" | "withdrawals";
export type PurchaseMethod = "balance" | "wechat";

export interface AccountCenterPage<T> {
  items: T[];
  total: number;
  page: number;
  size: number;
}

export function emptyAccountBalance(): AccountBalance {
  return {
    totalCredits: 0,
    usedCredits: 0,
    remainingCredits: 0,
    frozenCredits: 0,
    availableCredits: 0,
    permanentCredits: 0,
    timedCredits: 0,
    packages: []
  };
}

export function emptyCashAccount(): TenantCashAccount {
  return {
    balance: 0,
    frozen: 0,
    available: 0,
    creditsPerCny: 100,
    withdrawFeeBp: 160
  };
}

export function normalizeAccountTab(value: unknown): AccountCenterTab {
  return value === "balance" || value === "withdrawals" ? value : "points";
}

export function formatCredits(value: number | null | undefined): string {
  return Number(value ?? 0).toLocaleString("zh-CN", { maximumFractionDigits: 2 });
}

export function formatCents(value: number | null | undefined): string {
  return (Number(value ?? 0) / 100).toLocaleString("zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  });
}

export function formatTime(value?: number | string | null): string {
  if (!value) return "—";
  return new Date(value).toLocaleString("zh-CN");
}

export function creditSourceText(orderType: string): string {
  return {
    platform_to_tenant: "平台发放",
    online_tenant_topup: "微信购买",
    cash_purchase: "余额购买"
  }[orderType] ?? "积分到账";
}

export function creditStatusText(status: string): string {
  return status === "reversed" ? "已撤销" : status === "active" ? "已到账" : status || "—";
}

export function cashTransactionText(type: string): string {
  return {
    topup_income: "用户充值到账",
    buy_credits: "购买积分",
    withdraw: "提现",
    adjust: "余额调整"
  }[type] ?? type;
}

export function withdrawalStatusText(status: string): string {
  return {
    pending: "待审核",
    approved: "审核通过，待打款",
    paid: "已打款",
    rejected: "已驳回",
    cancelled: "已取消"
  }[status] ?? status;
}

export function withdrawalStatusTone(status: string): "success" | "warning" | "danger" | "info" {
  if (status === "paid") return "success";
  if (status === "pending" || status === "approved") return "warning";
  if (status === "rejected") return "danger";
  return "info";
}

export function maskAccount(value: string): string {
  if (!value) return "—";
  if (value.length <= 4) return value;
  return `${"*".repeat(Math.max(4, value.length - 4))}${value.slice(-4)}`;
}
