export interface BalanceLot {
  balanceLotId: string;
  totalUsd: number;
  remainingUsd: number;
  createdAt: string;
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
