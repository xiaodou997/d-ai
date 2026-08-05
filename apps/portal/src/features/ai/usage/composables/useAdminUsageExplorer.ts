import { computed, onMounted, onUnmounted, reactive, shallowRef } from "vue";

import {
  buildWorkbenchRangeWindow,
  DEFAULT_WORKBENCH_RANGE_ID,
  getWorkbenchRangeOption,
  WORKBENCH_RANGE_OPTIONS,
  type WorkbenchRangeId
} from "@/components/workbench/workbenchRanges";
import type { AdminUsageApi } from "../api";
import {
  mapAdminUsageRankingRows,
  mapAdminUsageRows,
  type AdminUsageRankingRow,
  type AdminUsageRow,
  type DailyTrendRowDTO,
  type UsageFilterChip,
  type UsageFilters,
  type UsageHighlight,
  type UsageLogDetailDTO,
  type UsageMetric,
  type UsagePagination,
  type UsageStatsDTO,
  type UsageSummaryRowDTO,
  type UsageUnitSummaryRowDTO,
  type UsageUpstreamSummaryRowDTO,
  type UsageWorkbenchTab
} from "../model";
import {
  buildFailedLogs,
  buildModelDistribution,
  buildRequestTrendSeries,
  buildSlowLogs,
  buildSummaryTotals,
  buildTokenTrendSeries,
  buildUnitDistribution,
  formatCredits,
  formatPercent,
  resolveRequestTotalMs
} from "../format";

const EMPTY_STATS: UsageStatsDTO = {
  total_requests: 0,
  success_count: 0,
  failed_count: 0,
  total_tokens: 0,
  total_catalog_base_credits: 0,
  total_tenant_payable_credits: 0,
  total_user_charged_credits: 0,
  avg_latency_ms: 0,
  avg_request_total_ms: 0,
  avg_first_response_byte_ms: 0
};

interface AdminUsageAuthContext {
  userType: number;
  userInfo?: { tenantId?: string } | null;
}

interface UseAdminUsageExplorerOptions {
  api: AdminUsageApi;
  auth: AdminUsageAuthContext;
  immediate?: boolean;
  onError: (message: string) => void;
  scope?: "records" | "analytics";
}

type RequestKind = "summary" | "logs" | "trend" | "detail" | "errors" | "ranking" | "upstream";

export function useAdminUsageExplorer(options: UseAdminUsageExplorerOptions) {
  const api = options.api;
  const authStore = options.auth;
  const notifyError = options.onError;
  const scope = options.scope ?? "records";

  const activeTab = shallowRef<UsageWorkbenchTab>("records");
  const selectedRangeId = shallowRef<WorkbenchRangeId>(DEFAULT_WORKBENCH_RANGE_ID);
  const filters = reactive<UsageFilters>({
    tenant_id: "",
    user_id: "",
    model_code: "",
    request_status: "",
    request_source: ""
  });
  const pagination = reactive<UsagePagination>({ page: 1, size: 20, total: 0 });
  const errorPagination = reactive<UsagePagination>({ page: 1, size: 20, total: 0 });

  const logsLoading = shallowRef(false);
  const summaryLoading = shallowRef(false);
  const trendLoading = shallowRef(false);
  const detailLoading = shallowRef(false);
  const errorsLoading = shallowRef(false);
  const rankingLoading = shallowRef(false);

  const logs = shallowRef<AdminUsageRow[]>([]);
  const errorRows = shallowRef<AdminUsageRow[]>([]);
  const rankingRows = shallowRef<AdminUsageRankingRow[]>([]);
  const upstreamLoading = shallowRef(false);
  const upstreamRows = shallowRef<UsageUpstreamSummaryRowDTO[]>([]);
  const rankingLimit = shallowRef(50);
  const rankingTotal = shallowRef(0);
  const logStats = shallowRef<UsageStatsDTO>({ ...EMPTY_STATS });
  const summaryRows = shallowRef<UsageSummaryRowDTO[]>([]);
  const unitRows = shallowRef<UsageUnitSummaryRowDTO[]>([]);
  const trendRows = shallowRef<DailyTrendRowDTO[]>([]);

  const detailOpen = shallowRef(false);
  const activeLog = shallowRef<AdminUsageRow | null>(null);
  const detail = shallowRef<UsageLogDetailDTO | null>(null);

  const isPlatformAdmin = computed(() => authStore.userType === 1 || authStore.userType === 2);
  const selfTenantId = computed(() => authStore.userInfo?.tenantId || "");
  const selectedRange = computed(() => getWorkbenchRangeOption(selectedRangeId.value));
  const periodLabel = computed(() => selectedRange.value.label);
  const rangeWindow = shallowRef(buildWorkbenchRangeWindow(selectedRange.value));

  const usageParams = computed(() => ({
    tenant_id: isPlatformAdmin.value ? filters.tenant_id || undefined : selfTenantId.value || undefined,
    user_id: filters.user_id || undefined,
    model_code: filters.model_code || undefined,
    request_status: filters.request_status || undefined,
    request_source: filters.request_source || undefined,
    date_from: rangeWindow.value.date_from,
    date_to: rangeWindow.value.date_to
  }));

  const summaryTotals = computed(() => buildSummaryTotals(summaryRows.value));

  const modelDistribution = computed(() => buildModelDistribution(summaryRows.value, summaryTotals.value.userCharged));
  const unitDistribution = computed(() => buildUnitDistribution(unitRows.value, summaryTotals.value.userCharged));
  const failedLogs = computed(() => buildFailedLogs(logs.value));
  const slowLogs = computed(() => buildSlowLogs(logs.value));
  const requestTrendSeries = computed(() => buildRequestTrendSeries(trendRows.value));
  const tokenTrendSeries = computed(() => buildTokenTrendSeries(trendRows.value));
  const totalRequests = computed(() => Number(logStats.value.total_requests) || 0);
  const successRate = computed(() => totalRequests.value ? (Number(logStats.value.success_count) * 100) / totalRequests.value : 0);
  const failureRate = computed(() => totalRequests.value ? (Number(logStats.value.failed_count) * 100) / totalRequests.value : 0);
  const topModel = computed(() => modelDistribution.value[0] || null);
  const topUnit = computed(() => unitDistribution.value[0] || null);
  const topSource = computed(() => {
    const counts = new Map<string, number>();
    for (const row of logs.value) counts.set(row.request_source, (counts.get(row.request_source) || 0) + 1);
    const winner = [...counts.entries()].sort((left, right) => right[1] - left[1])[0];
    return winner ? { key: winner[0], count: winner[1] } : null;
  });

  const filterChips = computed<UsageFilterChip[]>(() => {
    const chips: UsageFilterChip[] = [];
    if (isPlatformAdmin.value && filters.tenant_id) chips.push({ key: "tenant_id", label: "租户", value: filters.tenant_id });
    if (filters.user_id) chips.push({ key: "user_id", label: "用户", value: filters.user_id });
    if (filters.model_code) chips.push({ key: "model_code", label: "模型", value: filters.model_code });
    if (filters.request_status) chips.push({ key: "request_status", label: "状态", value: filters.request_status });
    if (filters.request_source) chips.push({ key: "request_source", label: "来源", value: filters.request_source });
    return chips;
  });

  const summaryNote = computed(() => {
    if (!pagination.total) return "当前窗口内没有命中的请求记录。可以调整时间窗口或放宽筛选条件。";
    const modelText = topModel.value ? `主力模型是 ${topModel.value.name}` : "暂无稳定主力模型";
    const sourceText = topSource.value ? `当前页最常见来源是 ${topSource.value.key}` : "当前页来源分布尚不明显";
    return `${periodLabel.value} 内共命中 ${formatCredits(pagination.total)} 条请求，成功率 ${formatPercent(successRate.value)}，${modelText}，${sourceText}。`;
  });

  const explorerHighlights = computed<UsageHighlight[]>(() => [
    { label: "观察窗口", value: periodLabel.value, hint: selectedRange.value.caption },
    { label: "失败请求", value: formatCredits(logStats.value.failed_count), hint: `失败率 ${formatPercent(failureRate.value)}` },
    {
      label: "主力模型",
      value: topModel.value?.name || "—",
      hint: topModel.value ? `${formatPercent(topModel.value.percent)} · ${formatCredits(topModel.value.requests)} 次` : "当前没有稳定主力模型"
    },
    {
      label: "主计费单位",
      value: topUnit.value?.name || "—",
      hint: topUnit.value ? `${formatCredits(topUnit.value.units || 0)} 计费量 · ${formatCredits(topUnit.value.credits)} 积分` : "当前没有计费结构数据"
    }
  ]);

  const explorerMetrics = computed<UsageMetric[]>(() => [
    { label: "命中请求", value: formatCredits(totalRequests.value), hint: `分页总数 ${formatCredits(pagination.total)}` },
    { label: "成功率", value: formatPercent(successRate.value), hint: `失败 ${formatCredits(logStats.value.failed_count)}` },
    { label: "用户实际扣款", value: formatCredits(summaryTotals.value.userCharged), hint: `Key 配额 ${formatCredits(summaryTotals.value.quotaCost)}` },
    { label: "Token 总量", value: formatCredits(summaryTotals.value.totalTokens), hint: `输入 ${formatCredits(summaryTotals.value.promptTokens)} · 输出 ${formatCredits(summaryTotals.value.completionTokens)}` },
    {
      label: "平均总耗时",
      value: `${Math.round(Number(logStats.value.avg_request_total_ms) || 0)} ms`,
      hint: slowLogs.value[0] ? `首响均值 ${Math.round(Number(logStats.value.avg_first_response_byte_ms) || 0)} ms · 最慢 ${resolveRequestTotalMs(slowLogs.value[0])} ms` : "当前无慢请求样本"
    }
  ]);

  const analyticsMetrics = computed<UsageMetric[]>(() => [
    { label: "请求总量", value: formatCredits(totalRequests.value), hint: `成功 ${formatCredits(logStats.value.success_count)} · 失败 ${formatCredits(logStats.value.failed_count)}` },
    { label: "用户实际扣款", value: formatCredits(summaryTotals.value.userCharged), hint: "租户零售视角" },
    { label: "租户结算应收", value: formatCredits(summaryTotals.value.tenantPayable), hint: `目录基准价 ${formatCredits(summaryTotals.value.catalogBase)}` },
    { label: "Key 配额", value: formatCredits(summaryTotals.value.quotaCost), hint: "可用于识别 key 消耗结构" },
    { label: "平均总耗时", value: `${Math.round(Number(logStats.value.avg_request_total_ms) || 0)} ms`, hint: `平均首响 ${Math.round(Number(logStats.value.avg_first_response_byte_ms) || 0)} ms` },
    { label: "主来源", value: topSource.value?.key || "—", hint: topSource.value ? `当前页 ${formatCredits(topSource.value.count)} 次` : "当前页暂无来源样本" }
  ]);

  const activeIndex = computed(() => activeLog.value ? logs.value.findIndex((row) => row.request_id === activeLog.value?.request_id) : -1);
  const canOpenPrevious = computed(() => activeIndex.value > 0);
  const canOpenNext = computed(() => activeIndex.value >= 0 && activeIndex.value < logs.value.length - 1);

  const controllers: Partial<Record<RequestKind, AbortController>> = {};
  const generations: Record<RequestKind, number> = { summary: 0, logs: 0, trend: 0, detail: 0, errors: 0, ranking: 0, upstream: 0 };
  let disposed = false;

  function beginRequest(kind: RequestKind) {
    controllers[kind]?.abort();
    const controller = new AbortController();
    controllers[kind] = controller;
    return { controller, generation: ++generations[kind] };
  }

  function requestIsCurrent(kind: RequestKind, generation: number, signal: AbortSignal) {
    return !disposed && !signal.aborted && generations[kind] === generation;
  }

  function cancelRequest(kind: RequestKind) {
    controllers[kind]?.abort();
    delete controllers[kind];
    generations[kind]++;
  }

  function syncRangeWindow() {
    rangeWindow.value = buildWorkbenchRangeWindow(selectedRange.value);
  }

  async function fetchSummaries(params = { ...usageParams.value }) {
    const { controller, generation } = beginRequest("summary");
    summaryLoading.value = true;
    try {
      const [unitSummary, summary] = await Promise.all([
        api.listUnitSummary(params, controller.signal),
        api.listSummary(params, controller.signal)
      ]);
      if (!requestIsCurrent("summary", generation, controller.signal)) return;
      unitRows.value = unitSummary.items ?? [];
      summaryRows.value = summary.items ?? [];
    } finally {
      if (requestIsCurrent("summary", generation, controller.signal)) summaryLoading.value = false;
    }
  }

  async function fetchLogs(params = { ...usageParams.value }) {
    const { controller, generation } = beginRequest("logs");
    logsLoading.value = true;
    try {
      const response = await api.listLogs({
        ...params,
        limit: pagination.size,
        offset: (pagination.page - 1) * pagination.size
      }, controller.signal);
      if (!requestIsCurrent("logs", generation, controller.signal)) return;
      logs.value = mapAdminUsageRows(response.records, response.included);
      logStats.value = response.stats ?? { ...EMPTY_STATS };
      pagination.total = response.total ?? 0;
    } finally {
      if (requestIsCurrent("logs", generation, controller.signal)) logsLoading.value = false;
    }
  }

  async function fetchTrend(window = rangeWindow.value) {
    const { controller, generation } = beginRequest("trend");
    trendLoading.value = true;
    try {
      const response = await api.listDailyTrend({ date_from: window.date_from, date_to: window.date_to }, controller.signal);
      if (!requestIsCurrent("trend", generation, controller.signal)) return;
      trendRows.value = response.items ?? [];
    } finally {
      if (requestIsCurrent("trend", generation, controller.signal)) trendLoading.value = false;
    }
  }

  async function refreshRecords({ resetPage = false }: { resetPage?: boolean } = {}) {
    if (resetPage) pagination.page = 1;
    syncRangeWindow();
    const results = await Promise.allSettled([fetchLogs({ ...usageParams.value })]);
    const rejected = results.find((result) => result.status === "rejected" && !isAbortError(result.reason));
    if (rejected?.status === "rejected") notifyError(errorMessage(rejected.reason, "加载请求记录失败"));
  }

  async function refreshAnalytics({ resetPage = false }: { resetPage?: boolean } = {}) {
    if (resetPage) pagination.page = 1;
    syncRangeWindow();
    const results = await Promise.allSettled([
      fetchSummaries({ ...usageParams.value }),
      fetchLogs({ ...usageParams.value }),
      fetchTrend(rangeWindow.value)
    ]);
    const rejected = results.find((result) => result.status === "rejected" && !isAbortError(result.reason));
    if (rejected?.status === "rejected") notifyError(errorMessage(rejected.reason, "加载用量分析失败"));
  }

  async function refreshWorkspace({ resetPage = false }: { resetPage?: boolean } = {}) {
    if (scope === "analytics") return refreshAnalytics({ resetPage });
    return refreshRecords({ resetPage });
  }

  async function refreshLogsOnly() {
    try {
      await fetchLogs({ ...usageParams.value });
    } catch (error) {
      if (!isAbortError(error)) notifyError(errorMessage(error, "加载请求记录失败"));
    }
  }

  async function loadErrors({ resetPage = false }: { resetPage?: boolean } = {}) {
    if (resetPage) errorPagination.page = 1;
    const { controller, generation } = beginRequest("errors");
    errorsLoading.value = true;
    try {
      const response = await api.listLogs({
        ...usageParams.value,
        request_status: "failed",
        limit: errorPagination.size,
        offset: (errorPagination.page - 1) * errorPagination.size
      }, controller.signal);
      if (!requestIsCurrent("errors", generation, controller.signal)) return;
      errorRows.value = mapAdminUsageRows(response.records, response.included);
      errorPagination.total = response.total ?? 0;
    } catch (error) {
      if (requestIsCurrent("errors", generation, controller.signal) && !isAbortError(error)) {
        notifyError(errorMessage(error, "加载错误记录失败"));
      }
    } finally {
      if (requestIsCurrent("errors", generation, controller.signal)) errorsLoading.value = false;
    }
  }

  async function loadRanking() {
    const { controller, generation } = beginRequest("ranking");
    rankingLoading.value = true;
    try {
      const response = await api.listUserRanking({ ...usageParams.value, limit: rankingLimit.value }, controller.signal);
      if (!requestIsCurrent("ranking", generation, controller.signal)) return;
      rankingRows.value = mapAdminUsageRankingRows(response.items, response.included);
      rankingTotal.value = response.total ?? 0;
    } catch (error) {
      if (requestIsCurrent("ranking", generation, controller.signal) && !isAbortError(error)) {
        notifyError(errorMessage(error, "加载用户排行失败"));
      }
    } finally {
      if (requestIsCurrent("ranking", generation, controller.signal)) rankingLoading.value = false;
    }
  }

  async function loadUpstream() {
    const { controller, generation } = beginRequest("upstream");
    upstreamLoading.value = true;
    try {
      const response = await api.listUpstreamSummary(usageParams.value, controller.signal);
      if (!requestIsCurrent("upstream", generation, controller.signal)) return;
      upstreamRows.value = response.items ?? [];
    } catch (error) {
      if (requestIsCurrent("upstream", generation, controller.signal) && !isAbortError(error)) {
        notifyError(errorMessage(error, "加载上游资源产出失败"));
      }
    } finally {
      if (requestIsCurrent("upstream", generation, controller.signal)) upstreamLoading.value = false;
    }
  }

  async function loadDetail(requestId: string) {
    const { controller, generation } = beginRequest("detail");
    detailLoading.value = true;
    detail.value = null;
    try {
      const response = await api.getDetail(requestId, controller.signal);
      if (requestIsCurrent("detail", generation, controller.signal)) detail.value = response;
    } catch (error) {
      if (requestIsCurrent("detail", generation, controller.signal) && !isAbortError(error)) {
        notifyError(errorMessage(error, "加载请求详情失败"));
      }
    } finally {
      if (requestIsCurrent("detail", generation, controller.signal)) detailLoading.value = false;
    }
  }

  async function applyFilters() {
    await refreshRecords({ resetPage: true });
  }

  async function resetFilters() {
    Object.assign(filters, { tenant_id: "", user_id: "", model_code: "", request_status: "", request_source: "" });
    await refreshRecords({ resetPage: true });
  }

  async function changePage(page: number) {
    if (page === pagination.page) return;
    pagination.page = page;
    await refreshLogsOnly();
  }

  async function changePageSize(size: number) {
    pagination.size = size;
    pagination.page = 1;
    await refreshLogsOnly();
  }

  async function changeErrorPage(page: number) {
    errorPagination.page = page;
    await loadErrors();
  }

  async function changeErrorPageSize(size: number) {
    errorPagination.size = size;
    errorPagination.page = 1;
    await loadErrors();
  }

  async function changeRankingLimit(limit: number) {
    rankingLimit.value = limit;
    await loadRanking();
  }

  async function changeRange(rangeId: WorkbenchRangeId) {
    selectedRangeId.value = rangeId;
    if (scope === "records" && activeTab.value === "errors") {
      syncRangeWindow();
      await loadErrors({ resetPage: true });
      return;
    }
    await refreshWorkspace({ resetPage: true });
  }

  async function changeTab(tab: UsageWorkbenchTab) {
    activeTab.value = tab;
    if (tab !== "errors") cancelRequest("errors");
    if (tab !== "ranking") cancelRequest("ranking");
    if (tab !== "upstream") cancelRequest("upstream");
    if (tab === "errors") await loadErrors({ resetPage: true });
    if (tab === "ranking") await loadRanking();
    if (tab === "upstream") await loadUpstream();
  }

  async function openDetail(row: AdminUsageRow) {
    activeLog.value = row;
    detailOpen.value = true;
    await loadDetail(row.request_id);
  }

  function closeDetail() {
    detailOpen.value = false;
    cancelRequest("detail");
    detailLoading.value = false;
  }

  async function openPreviousDetail() {
    const row = logs.value[activeIndex.value - 1];
    if (canOpenPrevious.value && row) await openDetail(row);
  }

  async function openNextDetail() {
    const row = logs.value[activeIndex.value + 1];
    if (canOpenNext.value && row) await openDetail(row);
  }

  onMounted(() => {
    if (options.immediate !== false) void refreshWorkspace({ resetPage: true });
  });

  onUnmounted(() => {
    disposed = true;
    for (const kind of Object.keys(controllers) as RequestKind[]) cancelRequest(kind);
  });

  return {
    WORKBENCH_RANGE_OPTIONS,
    activeLog,
    activeTab,
    analyticsMetrics,
    applyFilters,
    canOpenNext,
    canOpenPrevious,
    changeErrorPage,
    changeErrorPageSize,
    changePage,
    changePageSize,
    changeRange,
    changeRankingLimit,
    changeTab,
    closeDetail,
    detail,
    detailLoading,
    detailOpen,
    errorPagination,
    errorRows,
    errorsLoading,
    explorerHighlights,
    explorerMetrics,
    failedLogs,
    filterChips,
    filters,
    isPlatformAdmin,
    loadErrors,
    loadRanking,
    loadUpstream,
    logs,
    logsLoading,
    modelDistribution,
    openDetail,
    openNextDetail,
    openPreviousDetail,
    pagination,
    periodLabel,
    rankingLimit,
    rankingLoading,
    rankingRows,
    upstreamLoading,
    upstreamRows,
    rankingTotal,
    refresh: refreshWorkspace,
    requestTrendSeries,
    resetFilters,
    selectedRangeId,
    slowLogs,
    summaryLoading,
    summaryNote,
    tokenTrendSeries,
    trendLoading,
    trendRows,
    unitDistribution,
    usageParams
  };
}

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException
    ? error.name === "AbortError"
    : Boolean(error && typeof error === "object" && "name" in error && error.name === "AbortError");
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}
