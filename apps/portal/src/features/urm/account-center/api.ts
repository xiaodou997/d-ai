import { tenantApi } from "../../../api/tenant";
import { urmTenantApi } from "../../../api/urmTenant";
import type { AccountBalance, RechargeRecordItem } from "../../../types/urmTenant";
import type {
  PageTenantCashLedgerItem,
  PageTenantTopupOrderItem,
  PageTenantWithdrawal,
  TenantBuyCreditsResult,
  TenantCashAccount,
  TenantTopupConfig,
  TenantTopupOrderCreated,
  TenantTopupOrderStatus,
  TenantWithdrawal
} from "../../../types/tenant";
import type { AccountCenterPage } from "./model";

export interface AccountCenterApi {
  getPoints(detail?: boolean): Promise<AccountBalance>;
  getCash(): Promise<TenantCashAccount>;
  listPointRecords(page: number, size: number): Promise<AccountCenterPage<RechargeRecordItem>>;
  listPendingOrders(page: number, size: number): Promise<PageTenantTopupOrderItem>;
  listCashLedger(page: number, size: number): Promise<PageTenantCashLedgerItem>;
  listWithdrawals(page: number, size: number): Promise<PageTenantWithdrawal>;
  getTopupConfig(): Promise<TenantTopupConfig>;
  createTopupOrder(body: { amount?: number; packageId?: string }): Promise<TenantTopupOrderCreated>;
  getTopupOrder(orderId: string): Promise<TenantTopupOrderStatus>;
  buyCredits(amount: number): Promise<TenantBuyCreditsResult>;
  applyWithdrawal(body: {
    amount: number;
    accountName: string;
    bankName: string;
    accountNo: string;
    note?: string;
  }): Promise<TenantWithdrawal>;
  cancelWithdrawal(id: string): Promise<{ message: string }>;
}

export const accountCenterApi: AccountCenterApi = {
  getPoints: (detail = true) => urmTenantApi.getAccountBalance(detail),
  getCash: () => tenantApi.getCashAccount(),
  listPointRecords: (page, size) => urmTenantApi.getRechargeRecords({ page, size, rechargeType: "1" }),
  listPendingOrders: (page, size) => tenantApi.listTopupOrders({ page, size }),
  listCashLedger: (page, size) => tenantApi.listCashLedger({ page, size }),
  listWithdrawals: (page, size) => tenantApi.listWithdrawals({ page, size }),
  getTopupConfig: () => tenantApi.getTopupConfig(),
  createTopupOrder: (body) => tenantApi.createTopupOrder(body),
  getTopupOrder: (orderId) => tenantApi.getTopupOrder(orderId),
  buyCredits: (amount) => tenantApi.buyCredits({ amount }),
  applyWithdrawal: (body) => tenantApi.applyWithdrawal(body),
  cancelWithdrawal: (id) => tenantApi.cancelWithdrawal(id)
};
