import { computed, ref, type Ref } from "vue";

import { aiAdminApi } from "@/api/aiAdmin";
import { platformAdminApi } from "@/api/platformAdmin";
import { systemModulesApi } from "@/api/systemModules";
import { normalizeIdentityIncluded, type IdentityIncluded } from "@/platform/ai/identity";
import { adminUsageApi } from "@/features/ai/usage";
import {
  buildWorkbenchRangeWindow,
  DEFAULT_WORKBENCH_RANGE_ID,
  getWorkbenchRangeOption,
  WORKBENCH_RANGE_OPTIONS,
  type WorkbenchRangeId
} from "@/components/workbench/workbenchRanges";
import type {
  DashboardRecentErrorDTO,
  DashboardSummaryDTO,
  DashboardTopModelDTO,
  DashboardTopTenantDTO,
  SystemStatusDTO
} from "@/api/types/ai";
import type { GlobalStatsRow } from "@/api/types/admin";
import type { DailyTrendRowDTO, UsageUpstreamSummaryRowDTO } from "@/features/ai/usage/model";
import type { OverviewSection } from "./overviewTypes";

const emptySummary = (): DashboardSummaryDTO => ({
  total_requests: 0,
  successful_requests: 0,
  failed_requests: 0,
  total_tokens: 0,
  total_prompt_tokens: 0,
  total_completion_tokens: 0,
  total_catalog_base_usd: 0,
  total_tenant_payable_usd: 0,
  total_retail_base_usd: 0,
  total_user_payable_usd: 0,
  total_user_charged_usd: 0,
  avg_latency_ms: 0,
  avg_request_total_ms: 0,
  avg_first_response_byte_ms: 0
});

export function useAdminOverviewData(
  sections: readonly OverviewSection[],
  initialRangeId: WorkbenchRangeId = DEFAULT_WORKBENCH_RANGE_ID
) {
  const selectedRangeId = ref<WorkbenchRangeId>(initialRangeId);
  const selectedRange = computed(() => getWorkbenchRangeOption(selectedRangeId.value));
  const loading = ref(false);
  const lastUpdatedAt = ref<Date | null>(null);
  const failedSections = ref<OverviewSection[]>([]);
  const requestSequence = ref(0);
  const wanted = new Set(sections);

  const summary = ref<DashboardSummaryDTO>(emptySummary());
  const models = ref<DashboardTopModelDTO[]>([]);
  const tenants = ref<DashboardTopTenantDTO[]>([]);
  const tenantIncluded = ref<IdentityIncluded>(normalizeIdentityIncluded(undefined));
  const errors = ref<DashboardRecentErrorDTO[]>([]);
  const trend = ref<DailyTrendRowDTO[]>([]);
  const upstreams = ref<UsageUpstreamSummaryRowDTO[]>([]);
  const system = ref<SystemStatusDTO | null>(null);
  const global = ref<GlobalStatsRow | null>(null);
  const modules = ref<Awaited<ReturnType<typeof systemModulesApi.list>>>([]);
  const proxyNodes = ref<Awaited<ReturnType<typeof systemModulesApi.listProxyNodes>>>([]);

  function include(section: OverviewSection) {
    return wanted.has(section);
  }

  function take<T>(result: PromiseSettledResult<T>, section: OverviewSection): T | undefined {
    if (result.status === "fulfilled") return result.value;
    failedSections.value.push(section);
    return undefined;
  }

  async function refresh() {
    const sequence = requestSequence.value + 1;
    requestSequence.value = sequence;
    loading.value = true;
    failedSections.value = [];
    const window = buildWorkbenchRangeWindow(selectedRange.value);
    const dashboardParams = { date_from: window.date_from, date_to: window.date_to };

    const results = await Promise.allSettled([
      include("summary") ? aiAdminApi.getDashboardSummary(dashboardParams) : Promise.resolve(undefined),
      include("models") ? aiAdminApi.listDashboardTopModels({ ...dashboardParams, limit: 8 }) : Promise.resolve(undefined),
      include("tenants") ? aiAdminApi.listDashboardTopTenants({ ...dashboardParams, limit: 8 }) : Promise.resolve(undefined),
      include("errors") ? aiAdminApi.listDashboardRecentErrors({ ...dashboardParams, limit: 8 }) : Promise.resolve(undefined),
      include("trend") ? adminUsageApi.listDailyTrend(dashboardParams) : Promise.resolve(undefined),
      include("upstreams") ? adminUsageApi.listUpstreamSummary(dashboardParams) : Promise.resolve(undefined),
      include("system") ? aiAdminApi.getSystemStatus() : Promise.resolve(undefined),
      include("global") ? platformAdminApi.getGlobalStats({ timeFrom: window.startTime, timeTo: window.endTime }) : Promise.resolve(undefined),
      include("modules") ? systemModulesApi.list() : Promise.resolve(undefined),
      include("proxy") ? systemModulesApi.listProxyNodes() : Promise.resolve(undefined)
    ]);

    if (requestSequence.value !== sequence) return;

    const summaryValue = take(results[0], "summary");
    if (summaryValue) summary.value = summaryValue;
    const modelsValue = take(results[1], "models");
    if (modelsValue) models.value = modelsValue.items ?? [];
    const tenantsValue = take(results[2], "tenants");
    if (tenantsValue) {
      tenants.value = tenantsValue.items ?? [];
      tenantIncluded.value = normalizeIdentityIncluded(tenantsValue.included);
    }
    const errorsValue = take(results[3], "errors");
    if (errorsValue) errors.value = errorsValue.items ?? [];
    const trendValue = take(results[4], "trend");
    if (trendValue) trend.value = trendValue.items ?? [];
    const upstreamsValue = take(results[5], "upstreams");
    if (upstreamsValue) upstreams.value = upstreamsValue.items ?? [];
    const systemValue = take(results[6], "system");
    if (systemValue) system.value = systemValue;
    const globalValue = take(results[7], "global");
    if (globalValue) global.value = globalValue;
    const modulesValue = take(results[8], "modules");
    if (modulesValue) modules.value = modulesValue;
    const proxyValue = take(results[9], "proxy");
    if (proxyValue) proxyNodes.value = proxyValue;

    lastUpdatedAt.value = new Date();
    loading.value = false;
  }

  function changeRange(rangeId: WorkbenchRangeId) {
    selectedRangeId.value = rangeId;
    void refresh();
  }

  return {
    rangeOptions: WORKBENCH_RANGE_OPTIONS,
    selectedRangeId,
    selectedRange,
    loading,
    lastUpdatedAt,
    failedSections,
    summary,
    models,
    tenants,
    tenantIncluded,
    errors,
    trend,
    upstreams,
    system,
    global,
    modules,
    proxyNodes,
    refresh,
    changeRange
  };
}

export type AdminOverviewData = ReturnType<typeof useAdminOverviewData>;
