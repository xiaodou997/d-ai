// Platform 租户自助业务页类型 —— 字段名以统一后端 Huma DTO 真实返回为准（camelCase）。
// 对照 internal/{billing/pg,tenant/pg,transport} 的 DTO。

// ===== 统计 / 概览 =====
export interface TenantAnalyticsOverview {
  endUserCount: number;
  inviteCodeCount: number;
  userDeductionCredits: number;
  userTotalCredits: number;
  activeUserCount: number;
  userConsumptionCount: number;
  settlementIncomeCents: number;
}

// 按应用消耗分布（饼图）。v4 字段为 clientId/clientName（非 v1 的 appKey/appName）。
export interface ClientConsumptionItem {
  clientId: string;
  clientName: string;
  credits: number;
  percentage: string;
}

export interface UserConsumptionItem {
  userId: string;
  username: string;
  credits: number;
  transactionCount: number;
  percentage: string;
}

// ===== 账户余额 / 积分包 =====
export interface CreditPackage {
  packageId: string;
  totalCredits: number;
  remainingCredits: number;
  expiresAt?: string | null;
  source: string;
}

export interface AccountBalance {
  totalCredits: number;
  usedCredits: number;
  remainingCredits: number;
  frozenCredits: number;
  availableCredits: number;
  permanentCredits: number;
  timedCredits: number;
  packages?: CreditPackage[];
}

// ===== 交易流水 =====
export interface AccountTransactionItem {
  eventId: string;
  userId: string;
  description: string;
  tenantCredits: number;
  userCredits: number;
  status: string;
  terminalNote: string;
  metadata: string;
  createdTime?: number | null;
  finishedTime?: number | null;
  username: string;
  tenantName: string;
  clientId: string;
  appName: string;
}

// ===== 充值记录 =====
export interface RechargeRecordItem {
  orderId: string;
  orderType: string;
  paidAmount: number;
  creditAmount: number;
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
  credits: number;
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
  packageId: string;
  tenantId: string;
  userId: string;
  creditAmount: number;
  paidAmount: number;
  clearedOverdraft: number;
  packageCredits: number;
  orderTime: number;
}

export interface ReverseRechargeOutput {
  status: string;
  orderId: string;
  packageId: string;
  reversedCredits: number;
  originalCredits: number;
  lostCredits: number;
  packageStatus: string;
}

// ===== 通用分页 =====
export interface Page<T> {
  items: T[];
  total: number;
  page: number;
  size: number;
}
