import { computed, onMounted, shallowRef } from "vue";

import { tenantApi } from "../../../../api/tenant";
import { urmTenantApi } from "../../../../api/urmTenant";
import {
  DEFAULT_WORKBENCH_RANGE_ID,
  buildWorkbenchRangeWindow,
  getWorkbenchRangeOption,
  isWorkbenchRangeId,
  type WorkbenchRangeId,
  type WorkbenchRangeOption
} from "../../../../components/workbench/workbenchRanges";
import type { TenantCashAccount } from "../../../../types/tenant";
import type {
  AccountBalance,
  AccountTransactionItem,
  TenantAnalyticsOverview,
  UserConsumptionItem
} from "../../../../types/urmTenant";

const emptyOverview = (): TenantAnalyticsOverview => ({
  endUserCount: 0,
  inviteCodeCount: 0,
  userDeductionCredits: 0,
  userTotalCredits: 0,
  activeUserCount: 0,
  userConsumptionCount: 0,
  settlementIncomeCents: 0
});

const emptyCashAccount = (): TenantCashAccount => ({
  balance: 0,
  frozen: 0,
  available: 0,
  creditsPerCny: 100,
  withdrawFeeBp: 0
});

const emptyServiceBalance = (): AccountBalance => ({
  totalCredits: 0,
  usedCredits: 0,
  remainingCredits: 0,
  frozenCredits: 0,
  availableCredits: 0,
  permanentCredits: 0,
  timedCredits: 0
});

function rangeParams(range: WorkbenchRangeOption) {
  const window = buildWorkbenchRangeWindow(range);
  return { timeFrom: window.startTime, timeTo: window.endTime };
}

export function useTenantOperationsDashboard() {
  const selectedRangeId = shallowRef<WorkbenchRangeId>(DEFAULT_WORKBENCH_RANGE_ID);
  const cashAccount = shallowRef<TenantCashAccount>(emptyCashAccount());
  const serviceBalance = shallowRef<AccountBalance>(emptyServiceBalance());
  const overview = shallowRef<TenantAnalyticsOverview>(emptyOverview());
  const consumptionRanking = shallowRef<UserConsumptionItem[]>([]);
  const recentConsumption = shallowRef<AccountTransactionItem[]>([]);

  const summaryLoading = shallowRef(false);
  const serviceBalanceLoading = shallowRef(false);
  const rankingLoading = shallowRef(false);
  const recentLoading = shallowRef(false);
  let latestRangeRequestEpoch = 0;

  const selectedRange = computed(() => getWorkbenchRangeOption(selectedRangeId.value));
  const selectedRangeLabel = computed(() => selectedRange.value.label);
  const loading = computed(
    () => summaryLoading.value || serviceBalanceLoading.value || rankingLoading.value || recentLoading.value
  );

  async function fetchAccountSnapshots() {
    summaryLoading.value = true;
    serviceBalanceLoading.value = true;
    const [cashResult, balanceResult] = await Promise.allSettled([
      tenantApi.getCashAccount(),
      urmTenantApi.getAccountBalance(false)
    ]);
    if (cashResult.status === "fulfilled") {
      cashAccount.value = cashResult.value;
    } else {
      console.error("获取账户余额失败:", cashResult.reason);
    }
    if (balanceResult.status === "fulfilled") {
      serviceBalance.value = balanceResult.value;
    } else {
      console.error("获取积分失败:", balanceResult.reason);
    }
    serviceBalanceLoading.value = false;
  }

  async function fetchRangeData(range: WorkbenchRangeOption, requestEpoch: number) {
    summaryLoading.value = true;
    rankingLoading.value = true;
    recentLoading.value = true;
    const params = rangeParams(range);
    const [overviewResult, rankingResult, recentResult] = await Promise.allSettled([
      urmTenantApi.getAnalyticsOverview(params),
      urmTenantApi.getUserConsumption({ ...params, limit: 8 }),
      urmTenantApi.getTransactions({
        page: 1,
        size: 8,
        status: "succeeded",
        timeFrom: params.timeFrom,
        timeTo: params.timeTo
      })
    ]);
    if (requestEpoch !== latestRangeRequestEpoch) return;

    if (overviewResult.status === "fulfilled") {
      overview.value = overviewResult.value;
    } else {
      overview.value = emptyOverview();
      console.error("获取租户经营概览失败:", overviewResult.reason);
    }
    if (rankingResult.status === "fulfilled") {
      consumptionRanking.value = rankingResult.value;
    } else {
      consumptionRanking.value = [];
      console.error("获取用户消费贡献榜失败:", rankingResult.reason);
    }
    if (recentResult.status === "fulfilled") {
      recentConsumption.value = recentResult.value.items;
    } else {
      recentConsumption.value = [];
      console.error("获取近期用户消费失败:", recentResult.reason);
    }

    summaryLoading.value = false;
    rankingLoading.value = false;
    recentLoading.value = false;
  }

  function nextRequestEpoch() {
    latestRangeRequestEpoch += 1;
    return latestRangeRequestEpoch;
  }

  async function refresh() {
    const requestEpoch = nextRequestEpoch();
    await Promise.all([fetchAccountSnapshots(), fetchRangeData(selectedRange.value, requestEpoch)]);
    if (requestEpoch === latestRangeRequestEpoch) {
      summaryLoading.value = false;
    }
  }

  async function selectRange(rangeId: string) {
    if (!isWorkbenchRangeId(rangeId) || rangeId === selectedRangeId.value) return;
    selectedRangeId.value = rangeId;
    await fetchRangeData(selectedRange.value, nextRequestEpoch());
  }

  onMounted(() => {
    void refresh();
  });

  return {
    selectedRangeId: computed(() => selectedRangeId.value),
    selectedRangeLabel,
    cashAccount: computed(() => cashAccount.value),
    serviceBalance: computed(() => serviceBalance.value),
    overview: computed(() => overview.value),
    consumptionRanking: computed(() => consumptionRanking.value),
    recentConsumption: computed(() => recentConsumption.value),
    summaryLoading: computed(() => summaryLoading.value),
    serviceBalanceLoading: computed(() => serviceBalanceLoading.value),
    rankingLoading: computed(() => rankingLoading.value),
    recentLoading: computed(() => recentLoading.value),
    loading,
    refresh,
    selectRange
  };
}
