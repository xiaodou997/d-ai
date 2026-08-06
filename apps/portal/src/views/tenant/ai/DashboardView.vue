<!--
  智能服务工作台(AI 数据大盘):资产与授权面 + 核心调用信号 + 模型/入口版图 + 质量信号 + 最近工作记录。
  重构:迁移至新设计系统一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行,
       时间窗口/快捷入口/刷新收进 #actions,四个区块收进同卡 body、以 1px 分隔线分区,
       指标卡统一 DsMetricCard);来源分布等图表色值由硬编码 hex 改为运行时解析
       var(--ds-*) token(见 components/chartTokens.ts);业务逻辑与请求参数不变。
-->
<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useRouter } from "vue-router";
import { Refresh } from "@element-plus/icons-vue";
import { LayoutDashboard } from "lucide-vue-next";

import { PortalPagePanel } from "@/platform";

import TenantWorkbenchRangeTabs from "@/components/workbench/TenantWorkbenchRangeTabs.vue";
import AiWorkbenchMetricsSection from "./components/AiWorkbenchMetricsSection.vue";
import AiWorkbenchChartsSection from "./components/AiWorkbenchChartsSection.vue";
import AiWorkbenchQualitySection from "./components/AiWorkbenchQualitySection.vue";
import AiWorkbenchWorkspaceTrailSection from "./components/AiWorkbenchWorkspaceTrailSection.vue";
import {
  DEFAULT_WORKBENCH_RANGE_ID,
  WORKBENCH_RANGE_OPTIONS,
  buildWorkbenchRangeWindow,
  getWorkbenchRangeOption,
  isWorkbenchRangeId,
  type WorkbenchRangeId,
  type WorkbenchRangeOption
} from "@/components/workbench/workbenchRanges";
import { aiTenantApi, formatCredits } from "@/api/aiTenant";
import { listTenantUsageRecords, type TenantUsageLog } from "@/features/ai/usage";
import { tenantApi } from "@/api/tenant";
import type {
  ChatSession,
  ConsoleImageJob,
  TenantAiDashboardRecentError,
  TenantAiDashboardTopModel,
} from "@/api/types/aiTenant";
import type { TenantEndUserItem } from "@/api/types/tenant";

const router = useRouter();
const loading = ref(false);
const aiLoading = ref(false);
const signalLoading = ref(false);
const gatewayLoading = ref(false);
const analysisLoading = ref(false);

const topModels = ref<TenantAiDashboardTopModel[]>([]);
const recentErrors = ref<TenantAiDashboardRecentError[]>([]);
const workspaceSessions = ref<ChatSession[]>([]);
const workspaceJobs = ref<ConsoleImageJob[]>([]);
const usageInsightLogs = ref<TenantUsageLog[]>([]);
const usageInsightTotal = ref(0);
const users = ref<TenantEndUserItem[]>([]);
const selectedRangeId = ref<WorkbenchRangeId>(DEFAULT_WORKBENCH_RANGE_ID);

let latestRangeRequestEpoch = 0;

const aiStats = reactive({
  groupCount: 0,
  modelCount: 0,
  apiKeyCount: 0,
  runKeyCount: 0,
  totalCost: 0,
  totalRequests: 0,
  successRequests: 0,
  avgLatency: 0
});

const selectedRange = computed(() => getWorkbenchRangeOption(selectedRangeId.value));
const selectedRangeLabel = computed(() => selectedRange.value.label);

const successRate = computed(() => {
  const total = Number(aiStats.totalRequests) || 0;
  if (!total) return "0%";
  return `${((Number(aiStats.successRequests) || 0) * 100) / total}`.slice(0, 5).replace(/\.?0+$/, "") + "%";
});

const formatTime = (ts?: number | null) => {
  if (!ts) return "—";
  return new Date(ts).toLocaleString("zh-CN");
};

const formatMetricNumber = (value?: number | null) => Number(value ?? 0).toLocaleString("zh-CN");

const requestSourceLabel = (value?: string | null) =>
  ({
    api_key: "API",
    run_key: "Run Key",
    web_chat: "网页对话",
    web_image: "网页生图",
    app_preview: "应用试运行"
  } as Record<string, string>)[value || ""] || value || "未知来源";

// 图表分类色不再硬编码 hex,只记录 DsUI token,渲染时对主题子树内元素解析(components/chartTokens.ts)
const sourceDefinitions = [
  { key: "api_key", label: "API", colorToken: "--ds-info" },
  { key: "run_key", label: "Run Key", colorToken: "--ds-accent" },
  { key: "web_chat", label: "网页对话", colorToken: "--ds-positive" },
  { key: "web_image", label: "网页生图", colorToken: "--ds-warning" },
  { key: "app_preview", label: "应用试运行", colorToken: "--ds-danger" }
] as const;

const userMap = computed(() => {
  const map = new Map<string, TenantEndUserItem>();
  for (const user of users.value) {
    map.set(String(user.userId), user);
  }
  return map;
});

const accessMetrics = computed(() => [
  {
    key: "group-count",
    label: "自有分组数",
    value: formatMetricNumber(aiStats.groupCount),
    hint: "当前租户创建的零售分组",
    loading: aiLoading.value
  },
  {
    key: "model-count",
    label: "可用模型数",
    value: formatMetricNumber(aiStats.modelCount),
    hint: "分组上游与零售价共同覆盖的模型",
    loading: aiLoading.value
  },
  {
    key: "api-key-count",
    label: "API 密钥数",
    value: formatMetricNumber(aiStats.apiKeyCount),
    hint: "租户侧已创建的 API 密钥",
    loading: aiLoading.value
  },
  {
    key: "run-key-count",
    label: "应用运行密钥数",
    value: formatMetricNumber(aiStats.runKeyCount),
    hint: "应用运行层独立入口",
    loading: aiLoading.value
  }
]);

const signalMetrics = computed(() => [
  {
    key: "total-cost",
    label: "平台结算扣费",
    value: formatCredits(aiStats.totalCost),
    hint: `${selectedRangeLabel.value}租户成本积分`,
    loading: signalLoading.value
  },
  {
    key: "total-requests",
    label: "请求总数",
    value: formatMetricNumber(aiStats.totalRequests),
    hint: `${selectedRangeLabel.value}调用`,
    loading: signalLoading.value
  },
  {
    key: "success-rate",
    label: "成功率",
    value: successRate.value,
    hint: `${formatMetricNumber(aiStats.successRequests)} 次成功`,
    loading: signalLoading.value
  },
  {
    key: "avg-latency",
    label: "平均延迟",
    value: `${formatMetricNumber(aiStats.avgLatency)} ms`,
    hint: `${selectedRangeLabel.value}端到端响应`,
    loading: signalLoading.value
  }
]);

const usageInsightSummary = computed(() => {
  const sampleCount = usageInsightLogs.value.length;
  const total = usageInsightTotal.value;
  if (!sampleCount && !total) return `${selectedRangeLabel.value}暂无调用数据`;
  if (!total) return `${selectedRangeLabel.value}最新 ${formatMetricNumber(sampleCount)} 条调用样本`;
  return `${selectedRangeLabel.value}最新 ${formatMetricNumber(sampleCount)} 条调用样本 / 共 ${formatMetricNumber(total)} 条`;
});

const sourceInsights = computed(() => {
  const buckets = new Map<
    string,
    { key: string; label: string; colorToken: string; requestCount: number; successCount: number; totalCredits: number; totalTokens: number }
  >(
    sourceDefinitions.map((item) => [
      item.key,
      {
        key: item.key,
        label: item.label,
        colorToken: item.colorToken,
        requestCount: 0,
        successCount: 0,
        totalCredits: 0,
        totalTokens: 0
      }
    ])
  );

  for (const row of usageInsightLogs.value) {
    const sourceKey = sourceDefinitions.some((item) => item.key === row.request_source) ? row.request_source : "unknown";
    if (!buckets.has(sourceKey)) {
      buckets.set(sourceKey, {
        key: sourceKey,
        label: requestSourceLabel(sourceKey),
        colorToken: "--ds-faint",
        requestCount: 0,
        successCount: 0,
        totalCredits: 0,
        totalTokens: 0
      });
    }
    const bucket = buckets.get(sourceKey);
    if (!bucket) continue;
    bucket.requestCount += 1;
    if (row.request_status === "success") bucket.successCount += 1;
    bucket.totalCredits += Number(row.user_charged_credits || 0);
    bucket.totalTokens += Number(row.total_tokens || 0);
  }

  const sampleTotal = usageInsightLogs.value.length;
  return Array.from(buckets.values()).map((bucket) => {
    const successRateValue = bucket.requestCount ? (bucket.successCount / bucket.requestCount) * 100 : 0;
    return {
      ...bucket,
      shareText: sampleTotal ? `${((bucket.requestCount / sampleTotal) * 100).toFixed(0)}%` : "0%",
      successRateText: `${successRateValue.toFixed(0)}%`,
      creditsText: formatCredits(bucket.totalCredits),
      tokensText: formatMetricNumber(bucket.totalTokens)
    };
  });
});

const topUserInsights = computed(() => {
  const buckets = new Map<
    string,
    {
      key: string;
      userLabel: string;
      requestCount: number;
      successCount: number;
      totalCredits: number;
      lastActive: number;
    }
  >();

  for (const row of usageInsightLogs.value) {
    const key = row.user_id ? `user:${row.user_id}` : row.external_user_id ? `external:${row.external_user_id}` : "anonymous";
    const fallbackLabel = row.user_id
      ? userMap.value.get(String(row.user_id))?.username || userMap.value.get(String(row.user_id))?.email || row.user_id
      : row.external_user_id || "匿名访客";
    const bucket = buckets.get(key) || {
      key,
      userLabel: fallbackLabel,
      requestCount: 0,
      successCount: 0,
      totalCredits: 0,
      lastActive: 0
    };
    bucket.requestCount += 1;
    if (row.request_status === "success") bucket.successCount += 1;
    bucket.totalCredits += Number(row.user_charged_credits || 0);
    bucket.lastActive = Math.max(bucket.lastActive, Number(row.created_at || 0));
    buckets.set(key, bucket);
  }

  return Array.from(buckets.values())
    .sort((a, b) => b.totalCredits - a.totalCredits || b.requestCount - a.requestCount || b.lastActive - a.lastActive)
    .slice(0, 6)
    .map((bucket) => ({
      ...bucket,
      creditsText: formatCredits(bucket.totalCredits),
      successRateText: bucket.requestCount ? `${((bucket.successCount / bucket.requestCount) * 100).toFixed(0)}%` : "0%",
      lastActiveText: formatTime(bucket.lastActive)
    }));
});

const buildUsageRange = (range: WorkbenchRangeOption) => {
  const window = buildWorkbenchRangeWindow(range);
  return {
    date_from: window.date_from,
    date_to: window.date_to
  };
};

const fetchAccessOverview = async () => {
  aiLoading.value = true;
  try {
    const [groupsRes, modelsRes, keysRes, runKeysRes, sessionsRes, jobsRes] = await Promise.all([
      aiTenantApi.listMyGroups().catch(() => ({ items: [], total: 0 })),
      aiTenantApi.listAvailableModels().catch(() => ({ items: [], total: 0 })),
      aiTenantApi.listApiKeys().catch(() => ({ items: [], total: 0 })),
      aiTenantApi.listRunKeys().catch(() => ({ items: [], total: 0 })),
      aiTenantApi.listWorkspaceChatSessions({ limit: 6 }).catch(() => ({ items: [], total: 0 })),
      aiTenantApi.listWorkspaceImageJobs({ limit: 6 }).catch(() => ({ items: [], total: 0 }))
    ]);
    aiStats.groupCount = groupsRes.total || groupsRes.items?.length || 0;
    aiStats.modelCount = modelsRes.items?.length || 0;
    aiStats.apiKeyCount = keysRes.items?.length || 0;
    aiStats.runKeyCount = runKeysRes.total || runKeysRes.items?.length || 0;
    workspaceSessions.value = sessionsRes.items || [];
    workspaceJobs.value = jobsRes.items || [];
  } catch (e) {
    console.error("获取AI统计失败:", e);
  } finally {
    aiLoading.value = false;
  }
};

const fetchUsers = async () => {
  try {
    const usersRes = await tenantApi.listEndUsers({ page: 1, size: 200 }).catch(() => ({ items: [], total: 0, page: 1, size: 200 }));
    users.value = usersRes.items || [];
  } catch (e) {
    console.error("获取终端用户列表失败:", e);
    users.value = [];
  }
};

const fetchRangeBoundData = async (range: WorkbenchRangeOption, requestEpoch: number) => {
  signalLoading.value = true;
  gatewayLoading.value = true;
  analysisLoading.value = true;
  try {
    const rangeQuery = buildUsageRange(range);
    const [summaryRes, modelsRes, errorsRes, usageRes] = await Promise.all([
      aiTenantApi.getDashboardSummary(rangeQuery).catch(() => null),
      aiTenantApi.getDashboardTopModels({ ...rangeQuery, limit: 8 }).catch(() => ({ items: [], total: 0 })),
      aiTenantApi.listDashboardRecentErrors({ ...rangeQuery, limit: 5 }).catch(() => ({ items: [], total: 0 })),
      listTenantUsageRecords({
          limit: range.sampleLimit,
          offset: 0,
          ...rangeQuery
        }).catch(() => ({
          total: 0,
          stats: {
            total_requests: 0,
            success_count: 0,
            failed_count: 0,
            total_tokens: 0,
            total_user_charged_credits: 0,
            avg_latency_ms: 0
          },
          records: []
        }))
    ]);

    if (requestEpoch !== latestRangeRequestEpoch) return;

    aiStats.totalCost = Number(summaryRes?.total_tenant_payable_credits ?? 0);
    aiStats.totalRequests = Number(summaryRes?.total_requests ?? usageRes.stats?.total_requests ?? 0);
    aiStats.successRequests = Number(summaryRes?.successful_requests ?? usageRes.stats?.success_count ?? 0);
    aiStats.avgLatency = Number(summaryRes?.avg_latency_ms ?? usageRes.stats?.avg_latency_ms ?? 0);
    topModels.value = modelsRes.items || [];
    recentErrors.value = errorsRes.items || [];
    usageInsightTotal.value = usageRes.total ?? 0;
    usageInsightLogs.value = usageRes.records ?? [];
  } catch (e) {
    console.error("获取调用结构分析失败:", e);
    if (requestEpoch !== latestRangeRequestEpoch) return;
    aiStats.totalCost = 0;
    aiStats.totalRequests = 0;
    aiStats.successRequests = 0;
    aiStats.avgLatency = 0;
    topModels.value = [];
    recentErrors.value = [];
    usageInsightTotal.value = 0;
    usageInsightLogs.value = [];
  } finally {
    if (requestEpoch === latestRangeRequestEpoch) {
      signalLoading.value = false;
      gatewayLoading.value = false;
      analysisLoading.value = false;
    }
  }
};

const nextRangeRequestEpoch = () => {
  latestRangeRequestEpoch += 1;
  return latestRangeRequestEpoch;
};

const refreshRangeData = async () => {
  const requestEpoch = nextRangeRequestEpoch();
  loading.value = true;
  try {
    await fetchRangeBoundData(selectedRange.value, requestEpoch);
  } finally {
    if (requestEpoch === latestRangeRequestEpoch) {
      loading.value = false;
    }
  }
};

const fetchData = async () => {
  const requestEpoch = nextRangeRequestEpoch();
  loading.value = true;
  try {
    await Promise.all([fetchAccessOverview(), fetchUsers(), fetchRangeBoundData(selectedRange.value, requestEpoch)]);
  } finally {
    if (requestEpoch === latestRangeRequestEpoch) {
      loading.value = false;
    }
  }
};

const handleRangeChange = (rangeId: string) => {
  if (!isWorkbenchRangeId(rangeId)) return;
  if (selectedRangeId.value === rangeId) return;
  selectedRangeId.value = rangeId;
  void refreshRangeData();
};

onMounted(() => {
  void fetchData();
});
</script>

<template>
  <div class="ai-dashboard-page">
    <PortalPagePanel
      fill
      :icon="LayoutDashboard"
      :breadcrumbs="[{ label: '智能服务' }, { label: '概览' }, { label: '工作台' }]"
      description="把资产、消耗、入口与质量信号压在一屏。"
    >
      <template #actions>
        <TenantWorkbenchRangeTabs
          :model-value="selectedRangeId"
          :options="WORKBENCH_RANGE_OPTIONS"
          :loading="loading"
          aria-label="AI 工作台分析窗口"
          @update:model-value="handleRangeChange"
        />
        <el-button @click="router.push('/tenant/ai/models/prices')">价格表</el-button>
        <el-button @click="router.push('/tenant/developer/keys?tab=application')">应用密钥</el-button>
        <el-button type="primary" :loading="loading" @click="fetchData">
          <template #icon><el-icon><Refresh /></el-icon></template>
          刷新工作台
        </el-button>
      </template>

      <div class="ai-dashboard-body">
        <AiWorkbenchMetricsSection
          :access-metrics="accessMetrics"
          :signal-metrics="signalMetrics"
          :range-label="selectedRangeLabel"
        />

        <AiWorkbenchChartsSection
          :top-models="topModels"
          :model-loading="gatewayLoading"
          :source-items="sourceInsights"
          :source-loading="analysisLoading"
          :source-summary="usageInsightSummary"
          :range-label="selectedRangeLabel"
        />

        <AiWorkbenchQualitySection
          :recent-errors="recentErrors"
          :errors-loading="gatewayLoading"
          :user-insights="topUserInsights"
          :users-loading="analysisLoading"
          :range-label="selectedRangeLabel"
        />

        <AiWorkbenchWorkspaceTrailSection
          :sessions="workspaceSessions"
          :jobs="workspaceJobs"
          :loading="aiLoading"
        />
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.ai-dashboard-page {
  display: flex;
  flex: 1;
  min-height: 0;
  min-width: 0;
  flex-direction: column;
}

/* 面板 body 无内边距,四个区块在同卡内以 1px 分隔线分区(见 AiWorkbenchSection) */
.ai-dashboard-body {
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}
</style>
