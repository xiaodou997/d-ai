import { computed, onMounted, shallowRef } from "vue";

import { platformTenantApi } from "@/api/platformTenant";
import { tenantUsageApi } from "@/features/ai/usage";
import {
  DEFAULT_WORKBENCH_RANGE_ID,
  buildWorkbenchRangeWindow,
  getWorkbenchRangeOption,
  isWorkbenchRangeId,
  type WorkbenchRangeId,
  type WorkbenchRangeOption
} from "@/components/workbench/workbenchRanges";
import type {
  AccountBalance,
  TenantAnalyticsOverview,
  UserConsumptionItem
} from "@/api/types/platformTenant";
import type { TenantUsageLog } from "@/features/ai/usage";

const emptyOverview = (): TenantAnalyticsOverview => ({
  endUserCount: 0,
  inviteCodeCount: 0,
  userDeductionUsd: 0,
  userTotalBalanceUsd: 0,
  activeUserCount: 0,
  userConsumptionCount: 0,
  settlementIncomeMicroUsd: 0
});

const emptyServiceBalance = (): AccountBalance => ({
  currency: "USD", totalUsd: 0, usedUsd: 0, remainingUsd: 0,
  availableUsd: 0, permanentUsd: 0, timedUsd: 0, outstandingDebtMicroUsd: 0,
  serviceState: "active", balanceLots: []
});

function rangeParams(range: WorkbenchRangeOption) {
  const window = buildWorkbenchRangeWindow(range);
  return { timeFrom: window.startTime, timeTo: window.endTime };
}

export function useTenantOperationsDashboard() {
  const selectedRangeId = shallowRef<WorkbenchRangeId>(DEFAULT_WORKBENCH_RANGE_ID);
  const serviceBalance = shallowRef<AccountBalance>(emptyServiceBalance());
  const overview = shallowRef<TenantAnalyticsOverview>(emptyOverview());
  const consumptionRanking = shallowRef<UserConsumptionItem[]>([]);
  const recentConsumption = shallowRef<TenantUsageLog[]>([]);

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
    const balanceResult = await platformTenantApi.getAccountBalance(false).then(
      (value) => ({ status: "fulfilled" as const, value }),
      (reason) => ({ status: "rejected" as const, reason })
    );
    if (balanceResult.status === "fulfilled") {
      serviceBalance.value = balanceResult.value;
    } else {
      console.error("获取 USD 余额失败:", balanceResult.reason);
    }
    serviceBalanceLoading.value = false;
  }

  async function fetchRangeData(range: WorkbenchRangeOption, requestEpoch: number) {
    summaryLoading.value = true;
    rankingLoading.value = true;
    recentLoading.value = true;
    const params = rangeParams(range);
    const [overviewResult, rankingResult, recentResult] = await Promise.allSettled([
      platformTenantApi.getAnalyticsOverview(params),
      platformTenantApi.getUserConsumption({ ...params, limit: 8 }),
      tenantUsageApi.listRecords({
        limit: 8,
        offset: 0,
        request_status: "success",
        date_from: new Date(params.timeFrom).toISOString(),
        date_to: new Date(params.timeTo).toISOString()
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
      recentConsumption.value = recentResult.value.records ?? [];
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
