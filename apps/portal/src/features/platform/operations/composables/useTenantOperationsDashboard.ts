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
  lowBalanceUserCount: 0,
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

const sourceLabels = {
  balance: "服务额度",
  users: "用户余额明细",
  ledger: "账务记录",
  overview: "经营概览与用户统计",
  summary: "AI 结算汇总",
  ranking: "用户消费排行",
  recent: "近期消费记录",
  apps: "消费来源",
  models: "模型成本"
} as const;
type DashboardSource = keyof typeof sourceLabels;

export type TenantOperationsDashboard = ReturnType<typeof useTenantOperationsDashboard>;

export function useTenantOperationsDashboard() {
  // 财务决策需要一个足够稳定的观察窗口，默认近 30 天比“今天”更能避免
  // 新租户或低频租户打开页面时看到一整屏 0。
  const errors = shallowRef<Partial<Record<DashboardSource, boolean>>>({});

  function recordResults(results: Partial<Record<DashboardSource, PromiseSettledResult<unknown>>>) {
    const next = { ...errors.value };
    for (const key of Object.keys(results) as DashboardSource[]) {
      next[key] = results[key]?.status === "rejected";
    }
    errors.value = next;
  }

  function failedSources(...sources: DashboardSource[]) {
    return sources.filter((source) => errors.value[source]).map((source) => sourceLabels[source]);
  }

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

    recordResults({ balance: balanceResult, users: usersResult, ledger: ledgerResult });
    if (balanceResult.status === "fulfilled") serviceBalance.value = balanceResult.value;

    if (usersResult.status === "fulfilled") users.value = usersResult.value.items ?? [];

    if (ledgerResult.status === "fulfilled") balanceLedger.value = ledgerResult.value.items ?? [];

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

    recordResults({
      overview: overviewResult, summary: financialResult, ranking: rankingResult,
      recent: recentResult, apps: appResult, models: modelResult
    });
    if (overviewResult.status === "fulfilled") {
      overview.value = overviewResult.value;
    }
    if (financialResult.status === "fulfilled") {
      financialSummary.value = financialResult.value;
    }
    if (rankingResult.status === "fulfilled") {
      consumptionRanking.value = rankingResult.value;
    }
    if (recentResult.status === "fulfilled") {
      recentConsumption.value = recentResult.value.records ?? [];
    }
    if (appResult.status === "fulfilled") {
      appConsumption.value = appResult.value;
    }
    if (modelResult.status === "fulfilled") {
      topModels.value = modelResult.value.items ?? [];
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
    errors: computed(() => errors.value),
    failedSources,
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
