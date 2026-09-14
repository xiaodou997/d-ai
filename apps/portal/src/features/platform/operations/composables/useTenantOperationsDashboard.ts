import { computed, onMounted, shallowRef } from "vue";

import { platformTenantApi } from "@/api/platformTenant";
import { aiTenantApi } from "@/api/aiTenant";
import { tenantApi } from "@/api/tenant";
import { listTenantUsageRecords } from "@/features/ai/usage";
import {
  buildWorkbenchRangeWindow,
  getWorkbenchRangeOption,
  isWorkbenchRangeId,
  type WorkbenchRangeId,
  type WorkbenchRangeOption
} from "@/components/workbench/workbenchRanges";
import type {
  ClientConsumptionItem,
  AccountBalance,
  EndUserItem,
  TenantAnalyticsOverview,
  UserConsumptionItem
} from "@/api/types/platformTenant";
import type { TenantBalanceLedgerItem } from "@/api/types/tenant";
import type { TenantUsageLog } from "@/features/ai/usage";
import type { TenantAiDashboardSummary, TenantAiDashboardTopModel } from "@/api/types/aiTenant";

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
  // 财务决策需要一个足够稳定的观察窗口，默认近 30 天比“今天”更能避免
  // 新租户或低频租户打开页面时看到一整屏 0。
  const selectedRangeId = shallowRef<WorkbenchRangeId>("30d");
  const serviceBalance = shallowRef<AccountBalance>(emptyServiceBalance());
  const overview = shallowRef<TenantAnalyticsOverview>(emptyOverview());
  const financialSummary = shallowRef<TenantAiDashboardSummary | null>(null);
  const consumptionRanking = shallowRef<UserConsumptionItem[]>([]);
  const recentConsumption = shallowRef<TenantUsageLog[]>([]);
  const appConsumption = shallowRef<ClientConsumptionItem[]>([]);
  const topModels = shallowRef<TenantAiDashboardTopModel[]>([]);
  const users = shallowRef<EndUserItem[]>([]);
  const balanceLedger = shallowRef<TenantBalanceLedgerItem[]>([]);

  const summaryLoading = shallowRef(false);
  const serviceBalanceLoading = shallowRef(false);
  const rankingLoading = shallowRef(false);
  const recentLoading = shallowRef(false);
  const structureLoading = shallowRef(false);
  const usersLoading = shallowRef(false);
  const ledgerLoading = shallowRef(false);
  let latestRangeRequestEpoch = 0;
  let latestAccountRequestEpoch = 0;

  const selectedRange = computed(() => getWorkbenchRangeOption(selectedRangeId.value));
  const selectedRangeLabel = computed(() => selectedRange.value.label);
  const loading = computed(
    () => summaryLoading.value || serviceBalanceLoading.value || rankingLoading.value || recentLoading.value ||
      structureLoading.value || usersLoading.value || ledgerLoading.value
  );

  async function fetchAccountSnapshots() {
    const requestEpoch = ++latestAccountRequestEpoch;
    serviceBalanceLoading.value = true;
    usersLoading.value = true;
    ledgerLoading.value = true;

    const [balanceResult, usersResult, ledgerResult] = await Promise.allSettled([
      // 需要 detail=true 才能拿到限时额度，用于提醒即将到期的资金。
      platformTenantApi.getAccountBalance(true),
      platformTenantApi.getUsers({ page: 1, size: 200 }),
      tenantApi.listBalanceLedger({ page: 1, size: 8 })
    ]);

    if (requestEpoch !== latestAccountRequestEpoch) return;

    if (balanceResult.status === "fulfilled") serviceBalance.value = balanceResult.value;
    else console.error("获取 USD 余额失败:", balanceResult.reason);

    if (usersResult.status === "fulfilled") users.value = usersResult.value.items ?? [];
    else {
      users.value = [];
      console.error("获取终端用户余额失败:", usersResult.reason);
    }

    if (ledgerResult.status === "fulfilled") balanceLedger.value = ledgerResult.value.items ?? [];
    else {
      balanceLedger.value = [];
      console.error("获取余额流水失败:", ledgerResult.reason);
    }

    serviceBalanceLoading.value = false;
    usersLoading.value = false;
    ledgerLoading.value = false;
  }

  async function fetchRangeData(range: WorkbenchRangeOption, requestEpoch: number) {
    summaryLoading.value = true;
    rankingLoading.value = true;
    recentLoading.value = true;
    structureLoading.value = true;
    const params = rangeParams(range);
    const dateRange = {
      date_from: new Date(params.timeFrom).toISOString(),
      date_to: new Date(params.timeTo).toISOString()
    };
    const [overviewResult, financialResult, rankingResult, recentResult, appResult, modelResult] = await Promise.allSettled([
      platformTenantApi.getAnalyticsOverview(params),
      aiTenantApi.getDashboardSummary(dateRange),
      platformTenantApi.getUserConsumption({ ...params, limit: 8 }),
      listTenantUsageRecords({
        limit: 8,
        offset: 0,
        ...dateRange
      }),
      platformTenantApi.getAppConsumption(params),
      aiTenantApi.getDashboardTopModels({ ...dateRange, limit: 6 })
    ]);
    if (requestEpoch !== latestRangeRequestEpoch) return;

    if (overviewResult.status === "fulfilled") {
      overview.value = overviewResult.value;
    } else {
      overview.value = emptyOverview();
      console.error("获取租户经营概览失败:", overviewResult.reason);
    }
    if (financialResult.status === "fulfilled") {
      financialSummary.value = financialResult.value;
    } else {
      financialSummary.value = null;
      console.error("获取租户 AI 结算概览失败:", financialResult.reason);
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
    if (appResult.status === "fulfilled") {
      appConsumption.value = appResult.value;
    } else {
      appConsumption.value = [];
      console.error("获取消费来源结构失败:", appResult.reason);
    }
    if (modelResult.status === "fulfilled") {
      topModels.value = modelResult.value.items ?? [];
    } else {
      topModels.value = [];
      console.error("获取模型成本结构失败:", modelResult.reason);
    }

    summaryLoading.value = false;
    rankingLoading.value = false;
    recentLoading.value = false;
    structureLoading.value = false;
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
    financialSummary: computed(() => financialSummary.value),
    consumptionRanking: computed(() => consumptionRanking.value),
    recentConsumption: computed(() => recentConsumption.value),
    appConsumption: computed(() => appConsumption.value),
    topModels: computed(() => topModels.value),
    users: computed(() => users.value),
    balanceLedger: computed(() => balanceLedger.value),
    summaryLoading: computed(() => summaryLoading.value),
    serviceBalanceLoading: computed(() => serviceBalanceLoading.value),
    rankingLoading: computed(() => rankingLoading.value),
    recentLoading: computed(() => recentLoading.value),
    structureLoading: computed(() => structureLoading.value),
    usersLoading: computed(() => usersLoading.value),
    ledgerLoading: computed(() => ledgerLoading.value),
    loading,
    refresh,
    selectRange
  };
}
