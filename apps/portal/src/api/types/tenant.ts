export interface TenantOverviewStats {
  endUserCount: number;
  inviteCodeCount: number;
  userDeductionCredits: number;
  userTotalCredits: number;
  activeUserCount: number;
}

export interface TenantClientConsumptionItem {
  clientId: string;
  clientName: string;
  credits: number;
  percentage: string;
}

export interface TenantPortalBranding {
  tenantName: string;
  customerSiteName: string;
  faviconPath?: string;
}

export interface CreditPackage {
  packageId: string;
  remaining: number;
  expireTime?: number | null;
  source: string;
}

export interface BalanceResponse {
  packageType: number;
  accountId: string;
  totalCredits: number;
  frozenCredits: number;
  availableCredits: number;
  overdraftLimit: number;
  currentOverdraft: number;
  permanentCredits?: number;
  timedCredits?: number;
  packages?: CreditPackage[];
}

export interface TenantTransactionItem {
  eventId: string;
  userId: string;
  description: string;
  tenantCredits: number;
  userCredits: number;
  status: string;
  terminalNote: string;
  metadata: string;
  createdTime?: number;
  finishedTime?: number;
  username: string;
  tenantName: string;
  clientId: string;
}

export interface PageTenantTransactionItem {
  items: TenantTransactionItem[];
  total: number;
  page: number;
  size: number;
}

export interface TenantRechargeRecordItem {
  orderId: string;
  orderType: string;
  paidAmount: number;
  creditAmount: number;
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
  packageId: string;
  tenantId: string;
  userId: string;
  creditAmount: number;
  paidAmount: number;
  clearedOverdraft: number;
  packageCredits: number;
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
  credits?: number;
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
  quota_limit_credits?: number | null;
  quota_used_credits: number;
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
  exchangeRate: number;
  feeRateBp: number;
  min: number;
  max: number;
  packages: TopupPackage[];
}

export interface TopupPackage {
  id: string;
  name: string;
  amount: number;
  credits: number;
  badge?: string;
  enabled: boolean;
  sortOrder: number;
}

export interface TenantPaymentSettings {
  userCreditsPerCny: number;
  userCustomTopupFeeBp: number;
  userTopupPackages: TopupPackage[];
}

export interface TenantTopupOrderCreated {
  orderId: string;
  codeUrl: string;
  amount: number;
  creditAmount: number;
  grossCredits: number;
  feeCredits: number;
  topupMode: "custom" | "package";
  packageName?: string;
  status: string;
  expiresAt: number;
}

export interface TenantTopupOrderStatus {
  orderId: string;
  status: "created" | "paying" | "paid" | "closed" | "expired";
  amount: number;
  creditAmount: number;
  grossCredits: number;
  feeCredits: number;
  topupMode: "custom" | "package";
  packageName?: string;
  transactionId?: string;
  paidAt?: number | null;
}

export interface TenantTopupOrderItem {
  orderId: string;
  scene?: "user_topup" | "tenant_topup";
  status: string;
  amount: number;
  creditAmount: number;
  grossCredits: number;
  feeCredits: number;
  topupMode: "custom" | "package";
  packageName?: string;
  transactionId?: string;
  createdAt: number;
  paidAt?: number | null;
}

export interface PageTenantTopupOrderItem {
  items: TenantTopupOrderItem[];
  total: number;
  page: number;
  size: number;
}

// ===== 现金账户 =====
export interface TenantCashAccount {
  balance: number;
  frozen: number;
  available: number;
  creditsPerCny: number;
  withdrawFeeBp: number;
}

export interface TenantCashLedgerItem {
  txnId: string;
  txnType: string;
  amount: number;
  balanceAfter: number;
  refType?: string;
  refId?: string;
  note?: string;
  createdAt: number;
}

export interface PageTenantCashLedgerItem {
  items: TenantCashLedgerItem[];
  total: number;
  page: number;
  size: number;
}

export interface TenantBuyCreditsResult {
  creditOrderId: string;
  credits: number;
}

export interface TenantWithdrawal {
  withdrawalId: string;
  amount: number;
  feeAmount: number;
  payoutAmount: number;
  accountName: string;
  bankName: string;
  accountNo: string;
  status: string;
  applyNote?: string;
  reviewNote?: string;
  createdAt: number;
}

export interface PageTenantWithdrawal {
  items: TenantWithdrawal[];
  total: number;
  page: number;
  size: number;
}
