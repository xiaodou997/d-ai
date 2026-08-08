export interface RechargeFormPayload {
  paidAmountMinor: number;
  amountMicroUsd: number;
  note?: string;
  expireTime: number | null;
}
