// Platform 租户自助业务页类型 —— 字段名以统一后端 Huma DTO 真实返回为准（camelCase）。
// 对照 internal/{billing/pg,tenant/pg,transport} 的 DTO。

export type { AccountBalance, BalanceLot, Page, RechargeRecordItem } from "./account";

// ===== 统计 / 概览 =====
export interface TenantAnalyticsOverview {
	endUserCount: number;
	inviteCodeCount: number;
	userDeductionUsd: number;
	userTotalBalanceUsd: number;
	activeUserCount: number;
	userConsumptionCount: number;
	settlementIncomeMicroUsd: number;
}

// 按应用消耗分布（饼图）。v4 字段为 clientId/clientName（非 v1 的 appKey/appName）。
export interface ClientConsumptionItem {
  clientId: string;
  clientName: string;
	amountUsd: number;
  percentage: string;
}

export interface UserConsumptionItem {
  userId: string;
  username: string;
	amountUsd: number;
  transactionCount: number;
  percentage: string;
}

// ===== 终端用户 =====
export interface EndUserItem {
  userId: string;
  tenantId: string;
  username: string;
  tenantName?: string;
  email?: string;
  phone?: string;
  internalNote?: string;
  nickname?: string;
  avatar?: string;
  status: number; // 1=active, 2=disabled
	credentialState: "active" | "pending_activation";
	balanceUsd: number;
  lastLoginTime?: number | null;
  createdTime: number;
}

export interface CreateEndUserOutput {
  userId: string;
  tenantId: string;
  username: string;
  activationToken: string;
  activationExpiresIn: number;
}

export interface ActivationCredentialOutput {
  activationToken: string;
  activationExpiresIn: number;
}

// ===== 邀请码 =====
export interface InviteCodeItem {
  id: number;
  code: string;
  registrationUrl?: string;
  tenantId: string;
  createdBy: string;
  description: string;
  maxUses: number;
  usedCount: number;
  status: number; // 1=有效, 2=禁用
  expireTime?: number | null;
  createdTime: number;
  updatedTime: number;
}

export interface CreateInviteCodeOutput {
  code: string;
  registrationUrl?: string;
  tenantId: string;
  description: string;
  maxUses: number;
  expireTime?: number | null;
}

// ===== 充值 / 撤销 =====
export interface RechargeOutput {
	orderId: string;
	balanceLotId: string;
	tenantId: string;
	userId: string;
	currency: string;
	amountMicroUsd: number;
	paidAmountMinor: number;
	clearedDebtUsd: number;
	balanceLotUsd: number;
  orderTime: number;
}

export interface ReverseRechargeOutput {
  status: string;
  orderId: string;
	balanceLotId: string;
	reversedAmountUsd: number;
	originalAmountUsd: number;
	lostAmountUsd: number;
	balanceLotStatus: string;
}
