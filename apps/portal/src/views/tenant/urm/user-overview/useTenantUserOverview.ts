import { computed, shallowRef, watch, type MaybeRefOrGetter, toValue } from "vue";

import { aiTenantApi } from "../../../api/aiTenant";
import {
  listTenantUsageRecords,
  type TenantUsageLog,
  type TenantUsageStats
} from "../../../features/ai/usage";
import { proxyTenantApi } from "../../../api/proxyTenant";
import { urmTenantApi } from "../../../api/urmTenant";
import type { TenantAiLimitPolicy, TenantAiUserGroup, TenantAiVisibleGroup } from "../../../types/aiTenant";
import type { ProxyAccessLog, ProxyConsumptionStats, ProxyRouteWithPermission } from "../../../types/proxyTenant";
import type { EndUserItem, RechargeRecordItem } from "../../../types/urmTenant";
import { findEndUserById } from "../../../utils/endUsers";
import { parseTimestamp } from "./formatters";
import type {
  UserOverviewAccessibleGroup,
  UserOverviewGroupSummary,
  UserOverviewRiskSignal,
  UserOverviewRoutePermissionSummary
} from "./model";

const RECENT_RECHARGE_LIMIT = 8;
const RECENT_ACTIVITY_LIMIT = 8;
const ACTIVITY_WINDOW_DAYS = 30;

export interface UserOverviewServiceAvailability {
  ai: boolean;
  proxy: boolean;
}

const EMPTY_ACCESS_STATS: ProxyConsumptionStats = {
  totalRequests: 0,
  successRequests: 0,
  failedRequests: 0,
  tenantTotalCost: 0,
  userTotalCost: 0,
  avgLatencyMs: 0
};

const EMPTY_AI_USAGE_STATS: TenantUsageStats = {
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

function buildActivityWindow(days: number) {
  const end = new Date();
  const start = new Date();
  start.setDate(start.getDate() - (days - 1));
  start.setHours(0, 0, 0, 0);
  end.setHours(23, 59, 59, 999);
  return {
    startMs: start.getTime(),
    endMs: end.getTime(),
    startIso: start.toISOString(),
    endIso: end.toISOString()
  };
}

export function useTenantUserOverview(
  userIdSource: MaybeRefOrGetter<string>,
  serviceAvailabilitySource: MaybeRefOrGetter<UserOverviewServiceAvailability> = { ai: true, proxy: true }
) {
  const loading = shallowRef(false);
  const warnings = shallowRef<string[]>([]);

  const user = shallowRef<EndUserItem | null>(null);
  const rechargeRecords = shallowRef<RechargeRecordItem[]>([]);
  const rechargeTotal = shallowRef(0);
  const accessStats = shallowRef<ProxyConsumptionStats>(EMPTY_ACCESS_STATS);
  const accessLogs = shallowRef<ProxyAccessLog[]>([]);
  const aiUsageStats = shallowRef<TenantUsageStats>(EMPTY_AI_USAGE_STATS);
  const aiUsageLogs = shallowRef<TenantUsageLog[]>([]);
  const aiPolicy = shallowRef<TenantAiLimitPolicy | null>(null);
  const visibleGroups = shallowRef<TenantAiVisibleGroup[]>([]);
  const userGroups = shallowRef<TenantAiUserGroup[]>([]);
  const routePermissions = shallowRef<ProxyRouteWithPermission[]>([]);
  const serviceAvailability = computed(() => toValue(serviceAvailabilitySource));

  const activityWindow = buildActivityWindow(ACTIVITY_WINDOW_DAYS);
  const activityWindowLabel = `近 ${ACTIVITY_WINDOW_DAYS} 天`;

  const groupSummary = computed<UserOverviewGroupSummary>(() => {
    const defaultVisible = visibleGroups.value.filter((group) => group.user_default_visible).length;
    const accessible = visibleGroups.value.filter((group) => group.user_default_visible || userGroups.value.some((item) => item.group_id === group.id)).length;
    return {
      totalAvailable: visibleGroups.value.length,
      accessible,
      defaultVisible,
      customBindings: userGroups.value.length
    };
  });

  const accessibleGroups = computed<UserOverviewAccessibleGroup[]>(() => {
    const bindingMap = new Map(userGroups.value.map((item) => [item.group_id, item]));
    return visibleGroups.value
      .filter((group) => group.user_default_visible || bindingMap.has(group.id))
      .map((group) => {
        const binding = bindingMap.get(group.id);
        return {
          id: group.id,
          name: group.name,
          source: binding ? "custom" : "default",
          effectiveUserMultiplier: binding?.multiplier_override ?? group.default_user_multiplier,
          defaultMultiplier: group.default_user_multiplier,
          overrideMultiplier: binding?.multiplier_override ?? null,
          defaultVisible: group.user_default_visible
        };
      });
  });

  const routePermissionSummary = computed<UserOverviewRoutePermissionSummary>(() => ({
    total: routePermissions.value.length,
    allowed: routePermissions.value.filter((item) => item.isAllowed).length,
    denied: routePermissions.value.filter((item) => !item.isAllowed).length
  }));

  const lastActivityTime = computed<number | null>(() => {
    const candidates = [
      user.value?.lastLoginTime ?? null,
      parseTimestamp(accessLogs.value[0]?.createdAt),
      aiUsageLogs.value[0]?.created_at ?? null,
      rechargeRecords.value[0]?.createdTime ?? null
    ].filter((item): item is number => item != null);
    if (!candidates.length) return null;
    return Math.max(...candidates);
  });

  const riskSignals = computed<UserOverviewRiskSignal[]>(() => {
    const signals: UserOverviewRiskSignal[] = [];

    if (user.value?.status === 2) {
      signals.push({
        id: "account-disabled",
        tone: "danger",
        title: "账号状态",
        value: "已停用",
        description: "该终端用户当前无法正常登录或继续调用服务。"
      });
    }

    if (accessStats.value.failedRequests > 0) {
      signals.push({
        id: "proxy-failed",
        tone: accessStats.value.failedRequests >= 10 ? "danger" : "warning",
        title: "Proxy 调用失败",
        value: `${accessStats.value.failedRequests} 次`,
        description: `${activityWindowLabel} 内代理调用失败，需结合最近访问日志判断是上游故障还是权限问题。`
      });
    }

    if (aiUsageStats.value.failed_count > 0) {
      signals.push({
        id: "ai-failed",
        tone: aiUsageStats.value.failed_count >= 10 ? "danger" : "warning",
        title: "AI 请求失败",
        value: `${aiUsageStats.value.failed_count} 次`,
        description: `${activityWindowLabel} 内智能服务调用存在失败请求，可排查模型、限流或上游异常。`
      });
    }

    if (aiPolicy.value?.status === "disabled") {
      signals.push({
        id: "policy-disabled",
        tone: "info",
        title: "AI 限流策略",
        value: "已停用",
        description: "用户专属限流策略存在，但当前处于停用状态。"
      });
    }

    if (!signals.length) {
      signals.push({
        id: "healthy",
        tone: "success",
        title: "风险状态",
        value: "稳定",
        description: `${activityWindowLabel} 内未发现停用、失败或权限限制异常。`
      });
    }

    return signals;
  });

  const riskCount = computed(() => riskSignals.value.filter((item) => item.tone === "warning" || item.tone === "danger").length);

  let requestToken = 0;

  async function loadOverview(userId: string) {
    const token = ++requestToken;
    loading.value = true;
    warnings.value = [];

    user.value = null;
    rechargeRecords.value = [];
    rechargeTotal.value = 0;
    accessStats.value = { ...EMPTY_ACCESS_STATS };
    accessLogs.value = [];
    aiUsageStats.value = { ...EMPTY_AI_USAGE_STATS };
    aiUsageLogs.value = [];
    aiPolicy.value = null;
    visibleGroups.value = [];
    userGroups.value = [];
    routePermissions.value = [];

    const issues: string[] = [];
    const services = serviceAvailability.value;

    const [
      userResult,
      accessStatsResult,
      accessLogsResult,
      routePermissionsResult,
      aiUsageResult,
      visibleGroupsResult,
      userGroupsResult,
      aiPolicyResult
    ] = await Promise.allSettled([
      findEndUserById<EndUserItem>((params) => urmTenantApi.getUsers(params), userId),
      services.proxy
        ? proxyTenantApi.getConsumptionStats({
            userId,
            startTime: activityWindow.startMs,
            endTime: activityWindow.endMs
          })
        : Promise.resolve(null),
      services.proxy
        ? proxyTenantApi.listLogs({
            userId,
            page: 1,
            size: RECENT_ACTIVITY_LIMIT,
            startTime: activityWindow.startMs,
            endTime: activityWindow.endMs
          })
        : Promise.resolve(null),
      services.proxy ? proxyTenantApi.listUserRoutePermissions(userId) : Promise.resolve(null),
      services.ai
        ? listTenantUsageRecords({
            user_id: userId,
            limit: RECENT_ACTIVITY_LIMIT,
            offset: 0,
            date_from: activityWindow.startIso,
            date_to: activityWindow.endIso
          })
        : Promise.resolve(null),
      services.ai ? aiTenantApi.listMyGroups() : Promise.resolve(null),
      services.ai ? aiTenantApi.listUserGroups(userId) : Promise.resolve(null),
      services.ai ? aiTenantApi.listUserLimitPolicies(userId) : Promise.resolve(null)
    ]);

    if (token !== requestToken) return;

    if (userResult.status === "fulfilled") {
      user.value = userResult.value;
      if (!user.value) {
        issues.push("未在当前租户用户目录里定位到该用户，基础资料可能不完整。");
      }
    } else {
      issues.push("基础资料加载失败。");
    }

    if (accessStatsResult.status === "fulfilled") {
      accessStats.value = accessStatsResult.value ?? { ...EMPTY_ACCESS_STATS };
    } else if (services.proxy) {
      issues.push("访问统计加载失败。");
    }

    if (accessLogsResult.status === "fulfilled") {
      accessLogs.value = accessLogsResult.value?.items ?? [];
    } else if (services.proxy) {
      issues.push("最近访问日志加载失败。");
    }

    if (routePermissionsResult.status === "fulfilled") {
      routePermissions.value = routePermissionsResult.value?.items ?? [];
    } else if (services.proxy) {
      issues.push("接口权限摘要加载失败。");
    }

    if (aiUsageResult.status === "fulfilled") {
      aiUsageStats.value = aiUsageResult.value?.stats ?? { ...EMPTY_AI_USAGE_STATS };
      aiUsageLogs.value = aiUsageResult.value?.records ?? [];
    } else if (services.ai) {
      issues.push("AI 用量摘要加载失败。");
    }

    if (visibleGroupsResult.status === "fulfilled") {
      visibleGroups.value = visibleGroupsResult.value?.items ?? [];
    } else if (services.ai) {
      issues.push("AI 分组可见性加载失败。");
    }

    if (userGroupsResult.status === "fulfilled") {
      userGroups.value = userGroupsResult.value?.items ?? [];
    } else if (services.ai) {
      issues.push("AI 用户例外配置加载失败。");
    }

    if (aiPolicyResult.status === "fulfilled") {
      aiPolicy.value = aiPolicyResult.value?.items?.[0] ?? null;
    } else if (services.ai) {
      issues.push("AI 用户限流策略加载失败。");
    }

    if (user.value?.username) {
      const rechargeResult = await Promise.allSettled([
        urmTenantApi.getUserRechargeRecords({
          page: 1,
          size: RECENT_RECHARGE_LIMIT,
          username: user.value.username
        })
      ]);

      if (token !== requestToken) return;

      const [result] = rechargeResult;
      if (result.status === "fulfilled") {
        rechargeRecords.value = result.value.items ?? [];
        rechargeTotal.value = result.value.total ?? 0;
      } else {
        issues.push("充值记录加载失败。");
      }
    } else {
      issues.push("缺少用户名，无法精确关联充值记录。");
    }

    warnings.value = issues;
    loading.value = false;
  }

  watch(
    () => toValue(userIdSource).trim(),
    async (userId) => {
      if (!userId) return;
      await loadOverview(userId);
    },
    { immediate: true }
  );

  return {
    loading,
    warnings,
    activityWindowLabel,
    user,
    rechargeRecords,
    rechargeTotal,
    accessStats,
    accessLogs,
    aiUsageStats,
    aiUsageLogs,
    aiPolicy,
    accessibleGroups,
    groupSummary,
    routePermissionSummary,
    riskSignals,
    riskCount,
    serviceAvailability,
    lastActivityTime,
    refresh: async () => loadOverview(toValue(userIdSource).trim())
  };
}
