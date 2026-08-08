import { computed, onMounted, reactive, shallowRef } from "vue";
import { ElMessage } from "element-plus";

import type {
  TenantBalanceLedgerItem, TenantTopupConfig, TenantTopupOrderCreated,
  TenantTopupOrderItem, TenantTopupOrderStatus
} from "@/api/types/tenant";
import type { AccountBalance, RechargeRecordItem } from "@/api/types/platformTenant";
import { accountCenterApi, type AccountCenterApi } from "../api";
import {
  emptyAccountBalance, normalizeAccountTab,
  type AccountCenterPage, type AccountCenterTab
} from "../model";

const pageSize = 20;
const emptyPage = <T>(): AccountCenterPage<T> => ({ items: [], total: 0, page: 1, size: pageSize });

export function useTenantAccountCenter(options: { api?: AccountCenterApi; initialTab?: unknown } = {}) {
  const api = options.api ?? accountCenterApi;
  const balance = shallowRef<AccountBalance>(emptyAccountBalance());
  const topupConfig = shallowRef<TenantTopupConfig | null>(null);
  const rechargeRecords = shallowRef(emptyPage<RechargeRecordItem>());
  const pendingOrders = shallowRef(emptyPage<TenantTopupOrderItem>());
  const balanceLedger = shallowRef(emptyPage<TenantBalanceLedgerItem>());
  const activeTab = shallowRef<AccountCenterTab>(normalizeAccountTab(options.initialTab));
  const loading = reactive({ balances: false, topupConfig: false, recharges: false, pendingOrders: false, balanceLedger: false, purchase: false });
  const errors = reactive({ balance: "", records: "" });
  const loadedTabs = reactive({ recharges: false, ledger: false });
  const activeOrder = shallowRef<TenantTopupOrderCreated | null>(null);
  const qrVisible = shallowRef(false);
  const requestEpoch = shallowRef(0);

  const isLoading = computed(() => Object.values(loading).some(Boolean));
  const timedLots = computed(() => (balance.value.balanceLots ?? []).filter((lot) => lot.expiresAt));
  const nearestExpiry = computed(() => [...timedLots.value]
    .filter((lot) => lot.remainingUsd > 0 && lot.expiresAt)
    .sort((a, b) => new Date(a.expiresAt as string).getTime() - new Date(b.expiresAt as string).getTime())[0] ?? null);

  async function refreshBalances() {
    const epoch = ++requestEpoch.value;
    loading.balances = true;
    errors.balance = "";
    const detail = await api.getBalance(true).then((value) => ({ status: "fulfilled" as const, value })).catch((reason) => ({ status: "rejected" as const, reason }));
    if (epoch === requestEpoch.value) {
      if (detail.status === "fulfilled") balance.value = detail.value;
      else errors.balance = "额度暂时无法加载，请重试";
      loading.balances = false;
    }
  }

  async function loadRechargeRecords() {
    loading.recharges = true;
    loading.pendingOrders = true;
    const [records, pending] = await Promise.allSettled([
      api.listRechargeRecords(rechargeRecords.value.page, pageSize),
      api.listPendingOrders(pendingOrders.value.page, pageSize)
    ]);
    if (records.status === "fulfilled") rechargeRecords.value = records.value;
    else errors.records = "充值记录暂时无法加载，请重试";
    if (pending.status === "fulfilled") pendingOrders.value = { ...pending.value, items: pending.value.items.filter((item) => item.status !== "paid") };
    loading.recharges = false;
    loading.pendingOrders = false;
    loadedTabs.recharges = true;
  }

  async function loadBalanceLedger() {
    loading.balanceLedger = true;
    const result = await api.listBalanceLedger(balanceLedger.value.page, pageSize).catch(() => null);
    if (result) balanceLedger.value = result;
    loading.balanceLedger = false;
    loadedTabs.ledger = true;
  }

  async function selectTab(tab: AccountCenterTab) {
    activeTab.value = tab;
    if (tab === "recharges" && !loadedTabs.recharges) await loadRechargeRecords();
    if (tab === "ledger" && !loadedTabs.ledger) await loadBalanceLedger();
  }

  async function refresh() {
    loadedTabs.recharges = loadedTabs.ledger = false;
    await refreshBalances();
    await selectTab(activeTab.value);
  }

  async function loadTopupConfig() {
    if (topupConfig.value || loading.topupConfig) return;
    loading.topupConfig = true;
    topupConfig.value = await api.getTopupConfig().catch(() => null);
    loading.topupConfig = false;
  }

  function openPurchase() { void loadTopupConfig(); }

  async function createTopupOrder(body: { amountMicroUsd?: number; packageId?: string }) {
    loading.purchase = true;
    try {
      activeOrder.value = await api.createTopupOrder(body);
      qrVisible.value = true;
    } finally { loading.purchase = false; }
  }

  function pollOrder(): Promise<NonNullable<TenantTopupOrderStatus>> {
    if (!activeOrder.value) throw new Error("没有待支付订单");
    return api.getTopupOrder(activeOrder.value.orderId);
  }

  async function handleTopupSuccess() {
    qrVisible.value = false;
    activeOrder.value = null;
    ElMessage.success("充值成功，USD 额度已到账");
    await refreshBalances();
    loadedTabs.recharges = false;
    await loadRechargeRecords();
  }

  async function changePage(tab: AccountCenterTab, page: number) {
    if (tab === "recharges") { rechargeRecords.value = { ...rechargeRecords.value, page }; pendingOrders.value = { ...pendingOrders.value, page }; await loadRechargeRecords(); }
    else { balanceLedger.value = { ...balanceLedger.value, page }; await loadBalanceLedger(); }
  }

  onMounted(() => void refresh());
  return { activeTab, balance, topupConfig, rechargeRecords, pendingOrders, balanceLedger, loading, errors, isLoading, timedLots, nearestExpiry, activeOrder, qrVisible, refresh, selectTab, openPurchase, createTopupOrder, pollOrder, handleTopupSuccess, changePage, loadTopupConfig };
}
