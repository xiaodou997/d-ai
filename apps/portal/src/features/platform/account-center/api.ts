import { tenantApi } from "@/api/tenant";
import { platformTenantApi } from "@/api/platformTenant";
import type { AccountBalance, RechargeRecordItem } from "@/api/types/platformTenant";
import type {
  PageTenantBalanceLedgerItem, PageTenantTopupOrderItem, TenantTopupConfig,
  TenantTopupOrderCreated, TenantTopupOrderStatus
} from "@/api/types/tenant";
import type { AccountCenterPage } from "./model";

export interface AccountCenterApi {
  getBalance(detail?: boolean): Promise<AccountBalance>;
  listRechargeRecords(page: number, size: number): Promise<AccountCenterPage<RechargeRecordItem>>;
  listPendingOrders(page: number, size: number): Promise<PageTenantTopupOrderItem>;
  listBalanceLedger(page: number, size: number): Promise<PageTenantBalanceLedgerItem>;
  getTopupConfig(): Promise<TenantTopupConfig>;
  createTopupOrder(body: { amountMicroUsd?: number; packageId?: string }): Promise<TenantTopupOrderCreated>;
  getTopupOrder(orderId: string): Promise<TenantTopupOrderStatus>;
}

export const accountCenterApi: AccountCenterApi = {
  getBalance: (detail = true) => platformTenantApi.getAccountBalance(detail),
  listRechargeRecords: (page, size) => platformTenantApi.getRechargeRecords({ page, size, rechargeType: "1" }),
  listPendingOrders: (page, size) => tenantApi.listTopupOrders({ page, size }),
  listBalanceLedger: (page, size) => tenantApi.listBalanceLedger({ page, size }),
  getTopupConfig: () => tenantApi.getTopupConfig(),
  createTopupOrder: (body) => tenantApi.createTopupOrder(body),
  getTopupOrder: (orderId) => tenantApi.getTopupOrder(orderId)
};
