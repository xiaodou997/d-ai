// Platform customer self-service contracts. All money is USD; exact ledger
// values use int64 micro-USD and display values use decimal USD.

export interface BalanceLot {
  balanceLotId: string;
  totalUsd: number;
  remainingUsd: number;
  expiresAt?: string | null;
  source: string;
}

export interface CustomerPortalBrand {
  siteName: string;
  faviconPath?: string;
}

export interface AccountBalance {
  currency: "USD" | string;
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

export interface BalanceLotView extends BalanceLot {
  status: number;
}

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

export interface Page<T> {
  items: T[];
  total: number;
  page: number;
  size: number;
}

export interface TopupConfig {
  enabled: boolean;
  currency: "USD" | string;
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

export interface TopupOrderCreated {
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

export interface TopupOrderStatus extends Omit<TopupOrderCreated, "codeUrl" | "expiresAt"> {
  status: "created" | "paying" | "paid" | "closed" | "expired";
  transactionId?: string;
  paidAt?: number | null;
}

export interface TopupOrderItem extends Omit<TopupOrderStatus, "orderId"> {
  orderId: string;
  scene?: "user_topup" | "tenant_topup";
  createdAt: number;
}
