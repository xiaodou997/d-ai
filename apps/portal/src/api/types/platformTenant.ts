// Platform 租户自助业务页类型 —— 字段名以统一后端 Huma DTO 真实返回为准（camelCase）。
// 对照 internal/{billing/pg,tenant/pg,transport} 的 DTO。

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

// ===== USD 额度账户 / 额度包 =====
export interface BalanceLot {
	balanceLotId: string;
	totalUsd: number;
	remainingUsd: number;
  expiresAt?: string | null;
  source: string;
}

export interface AccountBalance {
	currency: string;
	totalUsd: number;
	usedUsd: number;
	remainingUsd: number;
	availableUsd: number;
	permanentUsd: number;
	timedUsd: number;
	outstandingDebtMicroUsd: number;
	serviceState: string;
	balanceLots?: BalanceLot[];
}

// ===== 充值记录 =====
export interface RechargeRecordItem {
  orderId: string;
  orderType: string;
	paidAmountMinor: number;
	amountUsd: number;
  status: string;
  note: string;
  userId: string;
  username: string;
  tenantName: string;
  createdTime?: number | null;
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
	balanceUsd: number;
  lastLoginTime?: number | null;
  createdTime: number;
}

export interface CreateEndUserOutput {
  userId: string;
  tenantId: string;
  username: string;
  defaultPassword: string;
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

// ===== 通用分页 =====
export interface Page<T> {
  items: T[];
  total: number;
  page: number;
  size: number;
}
