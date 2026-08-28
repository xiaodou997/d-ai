import type { OperationResponse } from ".";
import type { AccountBalance, Page, RechargeRecordItem } from "./types/account";

export function toAccountBalance(
  value: OperationResponse<"account-balance">
): AccountBalance {
  return {
    currency: value.currency,
    totalUsd: value.totalUsd,
    usedUsd: value.usedUsd,
    remainingUsd: value.remainingUsd,
    availableUsd: value.availableUsd,
    permanentUsd: value.permanentUsd,
    timedUsd: value.timedUsd,
    outstandingDebtMicroUsd: value.outstandingDebtMicroUsd,
    serviceState: value.serviceState,
    balanceLots: value.balanceLots?.map((item) => ({ ...item }))
  };
}

export function toRechargePage(
  value: OperationResponse<"account-recharge-records">
): Page<RechargeRecordItem> {
  return {
    items: value.items?.map((item) => ({ ...item })) ?? [],
    total: value.total,
    page: value.page,
    size: value.size
  };
}
