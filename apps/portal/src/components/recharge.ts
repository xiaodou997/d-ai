export interface RechargeFormPayload {
  paidAmount: number;
  creditAmount: number;
  note?: string;
  expireTime: number | null;
}
