export interface TenantOverviewStats {
  endUserCount: number;
  inviteCodeCount: number;
  userDeductionUsd: number;
  userTotalBalanceUsd: number;
  activeUserCount: number;
  userConsumptionCount: number;
  settlementIncomeMicroUsd: number;
}

export interface TenantClientConsumptionItem {
  clientId: string;
  clientName: string;
  amountUsd: number;
  percentage: string;
}

export interface TenantPortalBranding {
  tenantName: string;
  customerSiteName: string;
  faviconPath?: string;
}

export interface TenantRechargeRecordItem {
  orderId: string;
  orderType: string;
  paidAmountMinor: number;
  amountUsd: number;
  status: string;
  note: string;
  userId: string;
  username: string;
  tenantName: string;
  createdTime?: number;
}

export interface PageTenantRechargeRecordItem {
  items: TenantRechargeRecordItem[];
  total: number;
  page: number;
  size: number;
}

export interface TenantRechargeOutputBody {
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

export interface TenantEndUserItem {
  userId: string;
  tenantId: string;
  tenantName?: string;
  username: string;
  email?: string;
  phone?: string;
  nickname?: string;
  avatar?: string;
  status: number;
  balanceUsd?: number;
  lastLoginTime?: number;
  createdTime?: number;
}

export interface PageTenantEndUserItem {
  items: TenantEndUserItem[];
  total: number;
  page: number;
  size: number;
}

export interface CreateTenantEndUserOutputBody {
  userId: string;
  tenantId: string;
  username: string;
  defaultPassword: string;
}

export interface TenantInvitationItem {
  id: number;
  code: string;
  tenantId: string;
  createdBy: string;
  description: string;
  maxUses: number;
  usedCount: number;
  status: number;
  expireTime?: number | null;
  createdTime: number;
  updatedTime: number;
}

export interface PageTenantInvitationItem {
  items: TenantInvitationItem[];
  total: number;
  page: number;
  size: number;
}

export interface CreateTenantInvitationOutputBody {
  code: string;
  tenantId: string;
  description: string;
  maxUses: number;
  expireTime?: number | null;
}

export interface TenantAiApiKey {
  id: string;
  owner_type: string;
  tenant_id: string;
  user_id?: string | null;
  group_id: string;
  last_four?: string | null;
  name: string;
  quota_limit_micro_usd?: number | null;
  quota_used_micro_usd: number;
  status: string;
  expires_at?: number | null;
  last_used_at?: number | null;
  limit_policy?: {
    id?: string;
    scope_type?: string;
    scope_id?: string;
    concurrency_limit?: number | null;
    status?: "active" | "disabled" | null;
  } | null;
  created_by?: string | null;
  created_at?: number | null;
  updated_at?: number | null;
}

export interface TenantAiApiKeysOutputBody {
  items: TenantAiApiKey[];
  total: number;
}

// ===== 在线充值（微信支付） =====
export interface TenantTopupConfig {
  enabled: boolean;
  currency: string;
  feeRateBp: number;
  minMicroUsd: number;
  maxMicroUsd: number;
  validityDays?: number | null;
  packages: TopupPackage[];
}

export interface TopupPackage {
  id: string;
  name: string;
  paymentAmountMicroUsd: number;
  giftAmountMicroUsd: number;
  validityDays?: number | null;
  badge?: string;
  enabled: boolean;
  sortOrder: number;
}

export interface TenantPaymentSettings {
  userCustomTopupFeeBp: number;
  userCustomValidityDays?: number | null;
  userTopupPackages: TopupPackage[];
}

export interface TenantTopupOrderCreated {
  orderId: string;
  codeUrl: string;
  paymentCurrency: string;
  paymentAmountMinor: number;
  grossAmountMicroUsd: number;
  feeAmountMicroUsd: number;
  giftAmountMicroUsd: number;
  creditedAmountMicroUsd: number;
  topupMode: "custom" | "package";
  packageName?: string;
  status: string;
  expiresAt: number;
  balanceExpiresAt?: number | null;
}

export interface TenantTopupOrderStatus {
  orderId: string;
  status: "created" | "paying" | "paid" | "closed" | "expired";
  paymentCurrency: string;
  paymentAmountMinor: number;
  grossAmountMicroUsd: number;
  feeAmountMicroUsd: number;
  giftAmountMicroUsd: number;
  creditedAmountMicroUsd: number;
  topupMode: "custom" | "package";
  packageName?: string;
  transactionId?: string;
  paidAt?: number | null;
  balanceExpiresAt?: number | null;
}

export interface TenantTopupOrderItem {
  orderId: string;
  scene?: "user_topup" | "tenant_topup";
  status: string;
  paymentCurrency: string;
  paymentAmountMinor: number;
  grossAmountMicroUsd: number;
  feeAmountMicroUsd: number;
  giftAmountMicroUsd: number;
  creditedAmountMicroUsd: number;
  topupMode: "custom" | "package";
  packageName?: string;
  transactionId?: string;
  createdAt: number;
  paidAt?: number | null;
  balanceExpiresAt?: number | null;
}

export interface PageTenantTopupOrderItem {
  items: TenantTopupOrderItem[];
  total: number;
  page: number;
  size: number;
}

// ===== 统一 USD 余额 =====
export interface TenantBalanceAccount {
  currency: string;
  balanceMicroUsd: number;
}

export interface TenantBalanceLedgerItem {
  txnId: string;
  txnType: string;
  currency: string;
  amountMicroUsd: number;
  balanceAfterMicroUsd: number;
  refType?: string;
  refId?: string;
  note?: string;
  createdAt: number;
}

export interface PageTenantBalanceLedgerItem {
  items: TenantBalanceLedgerItem[];
  total: number;
  page: number;
  size: number;
}
