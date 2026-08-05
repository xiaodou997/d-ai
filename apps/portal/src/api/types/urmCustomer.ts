// URM 用户（终端用户 type=4）自助业务页类型 —— 字段名以统一后端 Huma DTO 真实返回为准（camelCase）。
// 对照 internal/billing/pg/{account.go,billing_repo.go} 的 DTO。

// ===== 账户余额 / 积分包 =====
// 后端 CreditPackage：expiresAt 为 RFC3339 时间字符串（*time.Time），无 status 字段，
// 状态由前端按 expiresAt/remainingCredits 派生（与 v1 一致）。
export interface CreditPackage {
  packageId: string;
  totalCredits: number;
  remainingCredits: number;
  expiresAt?: string | null;
  source: string;
}

export interface CustomerPortalBrand {
  siteName: string;
  faviconPath?: string;
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

// 视图层派生后的积分包（附带 status：1=可用 2=已过期 3=已耗尽）。
export interface PackageView {
  packageId: string;
  totalCredits: number;
  remainingCredits: number;
  expiresAt?: string | null;
  source: string;
  status: number;
}

// ===== 积分流水 =====
// 终端用户侧：后端将 tenantCredits 复写为 userCredits，仅展示 userCredits。
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

// ===== 通用分页 =====
export interface Page<T> {
  items: T[];
  total: number;
  page: number;
  size: number;
}

// ===== 在线充值（微信支付） =====
export interface TopupConfig {
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

export interface TopupOrderCreated {
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

export interface TopupOrderStatus {
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

export interface TopupOrderItem {
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
