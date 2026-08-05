import { computed, onMounted, reactive, shallowRef } from "vue";
import { ElMessage } from "element-plus";

import type { TenantTopupConfig, TenantTopupOrderCreated, TenantTopupOrderStatus } from "@/api/types/tenant";
import type { AccountBalance, RechargeRecordItem } from "@/api/types/urmTenant";
import { accountCenterApi, type AccountCenterApi } from "../api";
import {
  emptyAccountBalance,
  emptyCashAccount,
  normalizeAccountTab,
  type AccountCenterPage,
  type AccountCenterTab,
  type PurchaseMethod
} from "../model";
import type { TenantCashAccount, TenantCashLedgerItem, TenantWithdrawal } from "@/api/types/tenant";

const pageSize = 20;

function emptyPage<T>(): AccountCenterPage<T> {
  return { items: [], total: 0, page: 1, size: pageSize };
}

export function useTenantAccountCenter(options: { api?: AccountCenterApi; initialTab?: unknown } = {}) {
  const api = options.api ?? accountCenterApi;
  const points = shallowRef<AccountBalance>(emptyAccountBalance());
  const cash = shallowRef<TenantCashAccount>(emptyCashAccount());
  const topupConfig = shallowRef<TenantTopupConfig | null>(null);
  const pointRecords = shallowRef<AccountCenterPage<RechargeRecordItem>>(emptyPage<RechargeRecordItem>());
  const pendingOrders = shallowRef(emptyPage<{ orderId: string; status: string; amount: number; creditAmount: number; topupMode: "custom" | "package"; packageName?: string; createdAt: number; }>());
  const cashLedger = shallowRef<AccountCenterPage<TenantCashLedgerItem>>(emptyPage<TenantCashLedgerItem>());
  const withdrawals = shallowRef<AccountCenterPage<TenantWithdrawal>>(emptyPage<TenantWithdrawal>());
  const activeTab = shallowRef<AccountCenterTab>(normalizeAccountTab(options.initialTab));

  const loading = reactive({
    balances: false,
    topupConfig: false,
    points: false,
    pendingOrders: false,
    cashLedger: false,
    withdrawals: false,
    purchase: false,
    withdrawal: false
  });
  const errors = reactive({ points: "", cash: "", records: "" });
  const loadedTabs = reactive({ points: false, balance: false, withdrawals: false });
  const purchaseMethod = shallowRef<PurchaseMethod>("wechat");
  const activeOrder = shallowRef<TenantTopupOrderCreated | null>(null);
  const qrVisible = shallowRef(false);
  const requestEpoch = shallowRef(0);

  const isLoading = computed(() => Object.values(loading).some(Boolean));
  const cashCanBuy = computed(() => cash.value.available > 0);
  const timedPackages = computed(() => (points.value.packages ?? []).filter((pkg) => pkg.expiresAt));
  const nearestExpiry = computed(() => {
    const candidates = timedPackages.value
      .filter((pkg) => pkg.remainingCredits > 0 && pkg.expiresAt)
      .sort((a, b) => new Date(a.expiresAt as string).getTime() - new Date(b.expiresAt as string).getTime());
    return candidates[0] ?? null;
  });

  async function refreshBalances() {
    const epoch = ++requestEpoch.value;
    loading.balances = true;
    errors.points = "";
    errors.cash = "";
    const [pointsResult, cashResult] = await Promise.allSettled([api.getPoints(true), api.getCash()]);
    if (epoch !== requestEpoch.value) return;
    if (pointsResult.status === "fulfilled") points.value = pointsResult.value;
    else errors.points = "积分暂时无法加载，请重试";
    if (cashResult.status === "fulfilled") cash.value = cashResult.value;
    else errors.cash = "余额暂时无法加载，请重试";
    loading.balances = false;
  }

  async function loadPointsRecords() {
    loading.points = true;
    loading.pendingOrders = true;
    errors.records = "";
    const [recordsResult, pendingResult] = await Promise.allSettled([
      api.listPointRecords(pointRecords.value.page, pageSize),
      api.listPendingOrders(pendingOrders.value.page, pageSize)
    ]);
    if (recordsResult.status === "fulfilled") pointRecords.value = recordsResult.value;
    else errors.records = "积分记录暂时无法加载，请重试";
    if (pendingResult.status === "fulfilled") {
      pendingOrders.value = {
        items: pendingResult.value.items.filter((item) => item.status !== "paid"),
        total: pendingResult.value.total,
        page: pendingResult.value.page,
        size: pendingResult.value.size
      };
    }
    loading.points = false;
    loading.pendingOrders = false;
    loadedTabs.points = true;
  }

  async function loadCashLedger() {
    loading.cashLedger = true;
    const result = await api.listCashLedger(cashLedger.value.page, pageSize).catch(() => null);
    if (result) cashLedger.value = result;
    loading.cashLedger = false;
    loadedTabs.balance = true;
  }

  async function loadWithdrawals() {
    loading.withdrawals = true;
    const result = await api.listWithdrawals(withdrawals.value.page, pageSize).catch(() => null);
    if (result) withdrawals.value = result;
    loading.withdrawals = false;
    loadedTabs.withdrawals = true;
  }

  async function selectTab(tab: AccountCenterTab) {
    activeTab.value = tab;
    if (tab === "points" && !loadedTabs.points) await loadPointsRecords();
    if (tab === "balance" && !loadedTabs.balance) await loadCashLedger();
    if (tab === "withdrawals" && !loadedTabs.withdrawals) await loadWithdrawals();
  }

  async function refresh() {
    loadedTabs.points = false;
    loadedTabs.balance = false;
    loadedTabs.withdrawals = false;
    await refreshBalances();
    if (activeTab.value === "points") await loadPointsRecords();
    if (activeTab.value === "balance") await loadCashLedger();
    if (activeTab.value === "withdrawals") await loadWithdrawals();
  }

  async function loadTopupConfig() {
    if (topupConfig.value || loading.topupConfig) return;
    loading.topupConfig = true;
    topupConfig.value = await api.getTopupConfig().catch(() => null);
    loading.topupConfig = false;
  }

  function openPurchase() {
    purchaseMethod.value = cashCanBuy.value ? "balance" : "wechat";
    void loadTopupConfig();
  }

  async function buyWithBalance(amountYuan: number) {
    const amount = Math.round(amountYuan * 100);
    if (amount <= 0 || amount > cash.value.available) throw new Error("请输入不超过可用余额的金额");
    loading.purchase = true;
    try {
      const result = await api.buyCredits(amount);
      ElMessage.success(`购买成功，到账 ${result.credits.toLocaleString()} 积分`);
      await refreshBalances();
      loadedTabs.points = false;
      loadedTabs.balance = false;
      await Promise.all([loadPointsRecords(), loadCashLedger()]);
    } finally {
      loading.purchase = false;
    }
  }

  async function createWechatOrder(body: { amount?: number; packageId?: string }) {
    loading.purchase = true;
    try {
      activeOrder.value = await api.createTopupOrder(body);
      qrVisible.value = true;
    } finally {
      loading.purchase = false;
    }
  }

  async function pollOrder(): Promise<NonNullable<TenantTopupOrderStatus>> {
    if (!activeOrder.value) throw new Error("没有待支付订单");
    return api.getTopupOrder(activeOrder.value.orderId);
  }

  async function handleTopupSuccess() {
    qrVisible.value = false;
    activeOrder.value = null;
    ElMessage.success("购买成功，积分已到账");
    await refreshBalances();
    loadedTabs.points = false;
    await loadPointsRecords();
  }

  async function submitWithdrawal(form: { amountYuan: number; accountName: string; bankName: string; accountNo: string; note?: string }) {
    const amount = Math.round(form.amountYuan * 100);
    if (amount <= 0 || amount > cash.value.available) throw new Error("请输入不超过可用余额的提现金额");
    if (!form.accountName || !form.bankName || !form.accountNo) throw new Error("请完整填写收款信息");
    loading.withdrawal = true;
    try {
      await api.applyWithdrawal({ amount, accountName: form.accountName, bankName: form.bankName, accountNo: form.accountNo, note: form.note || undefined });
      ElMessage.success("提现申请已提交");
      await refreshBalances();
      loadedTabs.withdrawals = false;
      await loadWithdrawals();
    } finally {
      loading.withdrawal = false;
    }
  }

  async function cancelWithdrawal(id: string) {
    await api.cancelWithdrawal(id);
    ElMessage.success("提现申请已取消");
    await refreshBalances();
    loadedTabs.withdrawals = false;
    await loadWithdrawals();
  }

  async function changePage(tab: AccountCenterTab, page: number) {
    if (tab === "points") {
      pointRecords.value = { ...pointRecords.value, page };
      pendingOrders.value = { ...pendingOrders.value, page };
      await loadPointsRecords();
    } else if (tab === "balance") {
      cashLedger.value = { ...cashLedger.value, page };
      await loadCashLedger();
    } else {
      withdrawals.value = { ...withdrawals.value, page };
      await loadWithdrawals();
    }
  }

  onMounted(() => void refresh());

  return {
    activeTab,
    points,
    cash,
    topupConfig,
    pointRecords,
    pendingOrders,
    cashLedger,
    withdrawals,
    loading,
    errors,
    isLoading,
    cashCanBuy,
    timedPackages,
    nearestExpiry,
    purchaseMethod,
    activeOrder,
    qrVisible,
    refresh,
    selectTab,
    openPurchase,
    buyWithBalance,
    createWechatOrder,
    pollOrder,
    handleTopupSuccess,
    submitWithdrawal,
    cancelWithdrawal,
    changePage,
    loadTopupConfig
  };
}
