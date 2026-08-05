<!--
  工作台：统一承载用量概览、最近对话和最近生图，作为用户端 AI 入口页。
  重构：迁移至 DsUI 一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       列表统一 DsTable、分组徽章统一 DsTag,图表颜色改由 --ds-* token 解析);
       业务逻辑/请求参数不变,最近生图仍复用 app-core 的 PortalImageJobTable。
-->
<script setup lang="ts">
import {
  PortalContentCard,
  PortalMetricGrid,
  PortalPagePanel
} from "@/platform";
import { DsTable, DsTag, type DsTableColumn } from "@/shared/ui";
import { formatMultiplier } from "@/platform/ai/utils";
import {
  PortalImageJobTable
} from "@/platform/ai/images";
import {
  UsageCostCell,
  UsageTag,
  UsageTokenCell,
  requestSourceOptions
} from "@/platform/ai/usage";
import { computed, nextTick, onMounted, onUnmounted, reactive, shallowRef, watch } from "vue";
import { Refresh } from "@element-plus/icons-vue";
import { LayoutDashboard } from "lucide-vue-next";
import { LineChart, PieChart } from "echarts/charts";
import { GridComponent, LegendComponent, TooltipComponent } from "echarts/components";
import { init, use, type EChartsType } from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";
import { useRouter } from "vue-router";

import { aiCustomerApi, formatCredits } from "@/api/aiCustomer";
import { urmCustomerApi } from "@/api/urmCustomer";
import {
  getCustomerUsageSummary,
  listCustomerUsageRecords,
  type CustomerUsageLog,
  type CustomerUsageSummary
} from "@/features/ai/usage";
import type { ChatSession, ConsoleImageJob, AiVisibleGroup } from "@/api/types/aiCustomer";

use([LineChart, PieChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer]);

const LOG_LIMIT = 100;

const sessionColumns: DsTableColumn[] = [
  { key: "title", title: "标题" },
  { key: "target", title: "目标" },
  { key: "provider_api_format", title: "API 格式", width: 140, mono: true },
  { key: "updated_at", title: "时间", width: 180 },
  { key: "actions", title: "操作", width: 90 }
];

const logColumns: DsTableColumn[] = [
  { key: "model", title: "模型 / 应用" },
  { key: "cost", title: "消耗积分", width: 110, align: "right" },
  { key: "tokens", title: "Token" },
  { key: "effort", title: "推理强度", width: 92 },
  { key: "status", title: "状态", width: 90 },
  { key: "source", title: "来源", width: 100 },
  { key: "request_id", title: "请求 ID", mono: true },
  { key: "created_at", title: "时间", width: 170 }
];

// 图表颜色只能吃具体色值,从 --ds-* token 解析,避免硬编码 hex。
function resolveTokenColor(token: string): string | undefined {
  if (typeof document === "undefined" || typeof getComputedStyle !== "function") return undefined;
  return getComputedStyle(document.documentElement).getPropertyValue(token).trim() || undefined;
}

const router = useRouter();
const loading = shallowRef(false);
const usageLogs = shallowRef<CustomerUsageLog[]>([]);
const summary = shallowRef<CustomerUsageSummary | null>(null);
const recentSessions = shallowRef<ChatSession[]>([]);
const recentJobs = shallowRef<ConsoleImageJob[]>([]);
const visibleGroups = shallowRef<AiVisibleGroup[]>([]);
const groupsLoading = shallowRef(false);
const groupsLoaded = shallowRef(false);
const groupsError = shallowRef("");
const requestSource = shallowRef("");

const balanceInfo = reactive({
  totalCredits: 0,
  frozenCredits: 0,
  availableCredits: 0
});

const chartModelRef = shallowRef<HTMLElement | null>(null);
const chartTimelineRef = shallowRef<HTMLElement | null>(null);
const chartTokenRef = shallowRef<HTMLElement | null>(null);
let chartModel: EChartsType | null = null;
let chartTimeline: EChartsType | null = null;
let chartToken: EChartsType | null = null;
let workspaceGeneration = 0;
let workspaceController: AbortController | undefined;

const last7DayKeys = () => {
  const keys: string[] = [];
  for (let i = 6; i >= 0; i -= 1) {
    const date = new Date();
    date.setHours(0, 0, 0, 0);
    date.setDate(date.getDate() - i);
    keys.push(date.toISOString().slice(0, 10));
  }
  return keys;
};

const modelDistribution = computed(() => {
  const grouped = new Map<string, number>();
  for (const row of usageLogs.value) {
    const modelCode = row.model_code || (row.app_name ? `应用 · ${row.app_name}` : "unknown");
    const cost = Number(row.user_charged_credits) || 0;
    grouped.set(modelCode, (grouped.get(modelCode) || 0) + cost);
  }
  return Array.from(grouped.entries())
    .map(([name, value]) => ({ name, value }))
    .sort((a, b) => b.value - a.value)
    .slice(0, 8);
});

const timelineDayKeys = computed(() => last7DayKeys());

const timelineLabels = computed(() =>
  timelineDayKeys.value.map((dayKey) =>
    new Date(`${dayKey}T00:00:00`).toLocaleDateString("zh-CN", { month: "short", day: "numeric" })
  )
);

const groupByDay = (pick: (row: CustomerUsageLog) => number) => {
  const grouped = new Map<string, number>();
  for (const row of usageLogs.value) {
    if (!row.created_at) continue;
    const createdAt = new Date(row.created_at);
    if (Number.isNaN(createdAt.getTime())) continue;
    const dayKey = createdAt.toISOString().slice(0, 10);
    grouped.set(dayKey, (grouped.get(dayKey) || 0) + (pick(row) || 0));
  }
  return timelineDayKeys.value.map((dayKey) => grouped.get(dayKey) || 0);
};

const timelineValues = computed(() => groupByDay((row) => Number(row.user_charged_credits)));
const timelinePromptTokens = computed(() => groupByDay((row) => Number(row.prompt_tokens)));
const timelineCompletionTokens = computed(() => groupByDay((row) => Number(row.completion_tokens)));

const recentSessionRows = computed(() => recentSessions.value.slice(0, 6));
const recentJobRows = computed(() => recentJobs.value.slice(0, 6));
const previewGroups = computed(() => visibleGroups.value.slice(0, 6));
const activeGroupCount = computed(() => visibleGroups.value.filter((group) => group.status === "active").length);
const showInitialGroupLoading = computed(() => groupsLoading.value && !groupsLoaded.value);
const showGroupErrorState = computed(() => Boolean(groupsError.value) && visibleGroups.value.length === 0);
const showGroupErrorBanner = computed(() => Boolean(groupsError.value));
const groupErrorDetail = computed(() => {
  if (!groupsError.value) return "";
  if (visibleGroups.value.length > 0) {
    return `刷新失败：${groupsError.value} 当前展示的是上一次成功同步的分组结果。`;
  }
  return groupsError.value;
});

const sessionTargetLabel = (session: ChatSession) => {
  return session.model_code || "-";
};

const groupMetricValue = (value: number) => {
  if (showInitialGroupLoading.value) return "...";
  if (showGroupErrorState.value) return "--";
  return String(value);
};

const fetchBalance = async () => {
  try {
    const data = await urmCustomerApi.getBalance(false);
    if (data) {
      balanceInfo.totalCredits = data.totalCredits ?? 0;
      balanceInfo.frozenCredits = data.frozenCredits ?? 0;
      balanceInfo.availableCredits = data.availableCredits ?? 0;
    }
  } catch (e) {
    console.error("获取余额失败:", e);
  }
};

const fetchWorkspaceData = async () => {
  workspaceController?.abort();
  workspaceController = new AbortController();
  const requestController = workspaceController;
  const requestGeneration = ++workspaceGeneration;
  loading.value = true;
  try {
    const [usageResponse, summaryResponse, sessionResponse, imageResponse] = await Promise.all([
      listCustomerUsageRecords({
        limit: LOG_LIMIT,
        request_source: requestSource.value || undefined
      }, requestController.signal),
      getCustomerUsageSummary(requestSource.value || undefined, requestController.signal),
      aiCustomerApi.listWorkspaceChatSessions({ limit: 6 }, requestController.signal),
      aiCustomerApi.listWorkspaceImageJobs({ limit: 6 }, requestController.signal)
    ]);
    if (requestController.signal.aborted || requestGeneration !== workspaceGeneration) return;
    usageLogs.value = usageResponse.items ?? [];
    summary.value = summaryResponse;
    recentSessions.value = sessionResponse.items ?? [];
    recentJobs.value = imageResponse.items ?? [];
  } catch (error) {
    if (!isAbortError(error)) throw error;
  } finally {
    if (!requestController.signal.aborted && requestGeneration === workspaceGeneration) loading.value = false;
  }
};

const fetchVisibleGroups = async () => {
  groupsLoading.value = true;
  groupsError.value = "";
  try {
    const res = await aiCustomerApi.listMyGroups();
    visibleGroups.value = res?.items ?? [];
    groupsLoaded.value = true;
  } catch (e) {
    if (!groupsLoaded.value) {
      visibleGroups.value = [];
    }
    groupsError.value = e instanceof Error ? e.message : "分组信息加载失败，请稍后重试。";
    console.error("获取可用分组失败:", e);
  } finally {
    groupsLoading.value = false;
  }
};

const fetchAllData = async () => {
  await Promise.all([fetchWorkspaceData(), fetchBalance(), fetchVisibleGroups()]);
};

const openChatWorkspace = () => {
  router.push("/workspace/chat");
};

const openImageWorkspace = () => {
  router.push("/workspace/images");
};

const openMyGroups = () => {
  router.push("/workspace/groups");
};

const openChatSession = (session: ChatSession) => {
  router.push({
    path: "/workspace/chat",
    query: { session: session.id }
  });
};

const workspaceRequestSourceOptions = [{ label: "全部来源", value: "" }, ...requestSourceOptions];

const renderCharts = async () => {
  await nextTick();

  if (chartModelRef.value) {
    if (chartModel) chartModel.dispose();
    chartModel = init(chartModelRef.value);
    chartModel.setOption({
      tooltip: { trigger: "item", formatter: "{b}: {c} 积分 ({d}%)" },
      legend: { orient: "vertical", right: 10, top: "center" },
      series: [
        {
          type: "pie",
          radius: ["40%", "70%"],
          avoidLabelOverlap: false,
          itemStyle: { borderRadius: 10, borderColor: resolveTokenColor("--ds-panel"), borderWidth: 2 },
          label: { show: false },
          emphasis: { label: { show: true, fontSize: 14, fontWeight: "bold" } },
          labelLine: { show: false },
          data: modelDistribution.value
        }
      ]
    });
  }

  if (chartTimelineRef.value) {
    if (chartTimeline) chartTimeline.dispose();
    chartTimeline = init(chartTimelineRef.value);
    chartTimeline.setOption({
      tooltip: { trigger: "axis" },
      grid: { left: "3%", right: "4%", bottom: "3%", containLabel: true },
      xAxis: { type: "category", boundaryGap: false, data: timelineLabels.value },
      yAxis: { type: "value", name: "积分" },
      series: [
        {
          name: "消耗",
          type: "line",
          smooth: true,
          areaStyle: { opacity: 0.3 },
          lineStyle: { width: 3 },
          emphasis: { focus: "series" },
          data: timelineValues.value
        }
      ]
    });
  }

  if (chartTokenRef.value) {
    if (chartToken) chartToken.dispose();
    chartToken = init(chartTokenRef.value);
    chartToken.setOption({
      tooltip: { trigger: "axis" },
      legend: { data: ["输入 Token", "输出 Token"], bottom: 0 },
      grid: { left: "3%", right: "4%", bottom: "12%", top: "8%", containLabel: true },
      xAxis: { type: "category", boundaryGap: false, data: timelineLabels.value },
      yAxis: { type: "value", name: "Token" },
      series: [
        {
          name: "输入 Token",
          type: "line",
          smooth: true,
          stack: "tokens",
          areaStyle: { opacity: 0.3 },
          lineStyle: { width: 2 },
          itemStyle: { color: resolveTokenColor("--ds-info") },
          emphasis: { focus: "series" },
          data: timelinePromptTokens.value
        },
        {
          name: "输出 Token",
          type: "line",
          smooth: true,
          stack: "tokens",
          areaStyle: { opacity: 0.3 },
          lineStyle: { width: 2 },
          itemStyle: { color: resolveTokenColor("--ds-warning") },
          emphasis: { focus: "series" },
          data: timelineCompletionTokens.value
        }
      ]
    });
  }
};

const formatDate = (value?: number | null) => {
  if (!value) return "-";
  return new Date(value).toLocaleString();
};

const handleResize = () => {
  chartModel?.resize();
  chartTimeline?.resize();
  chartToken?.resize();
};

watch(usageLogs, () => {
  renderCharts();
});

onMounted(() => {
  fetchAllData();
  window.addEventListener("resize", handleResize);
});

onUnmounted(() => {
  workspaceGeneration += 1;
  workspaceController?.abort();
  window.removeEventListener("resize", handleResize);
  chartModel?.dispose();
  chartTimeline?.dispose();
  chartToken?.dispose();
});

function isAbortError(error: unknown) {
  return error instanceof DOMException
    ? error.name === "AbortError"
    : Boolean(error && typeof error === "object" && "name" in error && error.name === "AbortError");
}
</script>

<template>
  <div class="page-container workspace-page">
    <PortalPagePanel
      :icon="LayoutDashboard"
      :breadcrumbs="[{ label: '智能服务' }, { label: '工作台' }]"
      description="账户余额、AI 调用概览和最近工作记录，统一收口到一个入口"
    >
      <template #actions>
        <el-button @click="openChatWorkspace">进入对话</el-button>
        <el-button @click="openImageWorkspace">进入生图</el-button>
        <el-select v-model="requestSource" placeholder="全部来源" class="workspace-source-select" @change="fetchWorkspaceData">
          <el-option
            v-for="item in workspaceRequestSourceOptions"
            :key="item.value"
            :label="item.label"
            :value="item.value"
          />
        </el-select>
        <el-button type="primary" :icon="Refresh" :loading="loading" @click="fetchAllData">刷新</el-button>
      </template>

      <!-- 工作台主体：面板 body 无内边距，用 24px 容器承载各分区 -->
      <div class="workspace-body">
      <PortalContentCard class="access-routing-card">
      <template #header>
        <div class="access-routing-copy">
          <p class="access-routing-eyebrow">Access Routing</p>
          <h2 class="access-routing-title">先看模型定价，再配置调用入口</h2>
          <p class="access-routing-desc">
            分组会影响当前账号可见的模型范围、API Key 唯一绑定的路由，以及最终计费倍率。模型定价页会清楚展示每个分组下的实际价格。
          </p>
        </div>
      </template>
      <template #actions>
        <el-button @click="openMyGroups">查看模型定价</el-button>
        <el-button type="primary" plain @click="openChatWorkspace">直接开始对话</el-button>
      </template>

      <PortalMetricGrid
        :metrics="[
          { label: '可用分组', value: groupMetricValue(visibleGroups.length), hint: '当前账号可用分组' },
          { label: '启用分组', value: groupMetricValue(activeGroupCount), hint: '当前仍然启用' }
        ]"
      />

      <el-alert v-if="showGroupErrorBanner" type="danger" :closable="false" show-icon class="access-routing-error-alert">
        <template #title>
          <div class="access-routing-error-content">
            <div>
              <strong>分组信息暂时不可用</strong>
              <p class="access-routing-error-desc">{{ groupErrorDetail }}</p>
            </div>
            <el-button plain @click="fetchVisibleGroups">重试</el-button>
          </div>
        </template>
      </el-alert>

      <div class="access-routing-preview">
        <template v-if="showInitialGroupLoading">
          <span class="access-routing-hint">正在同步当前账号的分组权限...</span>
        </template>
        <template v-else-if="previewGroups.length">
          <DsTag v-for="group in previewGroups" :key="group.id" tone="neutral">
            {{ group.name }} x{{ formatMultiplier(group.effective_user_multiplier) }}
          </DsTag>
          <span v-if="visibleGroups.length > previewGroups.length" class="access-routing-hint">
            还有 {{ visibleGroups.length - previewGroups.length }} 个分组
          </span>
        </template>
        <span v-else-if="showGroupErrorState" class="access-routing-hint">点击上方重试，重新加载分组信息。</span>
        <span v-else class="access-routing-hint">当前账号还没有开放分组，请联系管理员开通。</span>
      </div>
    </PortalContentCard>

    <!-- Balance & Usage Stats -->
    <PortalMetricGrid
      :metrics="[
        { label: '总积分', value: balanceInfo.totalCredits.toLocaleString() },
        { label: '冻结积分', value: balanceInfo.frozenCredits.toLocaleString() },
        { label: '可用积分', value: balanceInfo.availableCredits.toLocaleString() },
        { label: '总消耗积分', value: formatCredits(summary?.total_user_charged_credits || 0) },
        { label: '输入 Token', value: formatCredits(summary?.total_prompt_tokens || 0) },
        { label: '输出 Token', value: formatCredits(summary?.total_completion_tokens || 0) },
        { label: '总请求次数', value: String(summary?.request_count || 0) }
      ]"
    />

    <!-- Charts -->
    <div class="grid grid-cols-3 gap-4">
      <PortalContentCard title="模型消耗分布">
        <div ref="chartModelRef" style="height: 280px; width: 100%"></div>
      </PortalContentCard>
      <PortalContentCard title="近 7 天消耗趋势">
        <div ref="chartTimelineRef" style="height: 280px; width: 100%"></div>
      </PortalContentCard>
      <PortalContentCard title="近 7 天 Token 趋势">
        <div ref="chartTokenRef" style="height: 280px; width: 100%"></div>
      </PortalContentCard>
    </div>

    <!-- Recent Workspace Records -->
    <div class="grid grid-cols-1 xl:grid-cols-2 gap-4">
      <PortalContentCard title="最近对话">
        <template #actions>
          <el-button link type="primary" @click="openChatWorkspace">查看全部</el-button>
        </template>
        <DsTable
          :frame="false"
          :columns="sessionColumns"
          :rows="recentSessionRows"
          row-key="id"
          :loading="loading"
          empty-title="暂无对话"
        >
          <template #cell-title="{ row }">
            <div class="session-cell">
              <p class="session-cell__title">{{ row.title || "新对话" }}</p>
              <p class="session-cell__id">{{ row.id }}</p>
            </div>
          </template>
          <template #cell-target="{ row }">{{ sessionTargetLabel(row) }}</template>
          <template #cell-provider_api_format="{ row }">{{ row.provider_api_format || "-" }}</template>
          <template #cell-updated_at="{ row }">{{ formatDate(row.updated_at) }}</template>
          <template #cell-actions="{ row }">
            <el-button link type="primary" @click="openChatSession(row)">打开</el-button>
          </template>
        </DsTable>
      </PortalContentCard>

      <PortalContentCard title="最近生图">
        <template #actions>
          <el-button link type="primary" @click="openImageWorkspace">查看全部</el-button>
        </template>
        <PortalImageJobTable :jobs="recentJobRows" :format-credits="formatCredits" />
      </PortalContentCard>
    </div>

    <!-- Logs Table -->
    <PortalContentCard title="调用记录" body-padding="none">
      <template #actions>
        <span class="workspace-log-count">共 {{ usageLogs.length }} 条</span>
      </template>
      <DsTable
        :frame="false"
        :columns="logColumns"
        :rows="usageLogs"
        row-key="request_id"
        :loading="loading"
        empty-title="暂无调用记录"
      >
        <template #cell-model="{ row }">
          {{ row.model_code || (row.app_name ? `应用 · ${row.app_name}` : "-") }}
        </template>
        <template #cell-cost="{ row }">
          <UsageCostCell :credits="row.user_charged_credits" />
        </template>
        <template #cell-tokens="{ row }">
          <UsageTokenCell
            dense
            :prompt="row.prompt_tokens"
            :completion="row.completion_tokens"
            :cache-read="row.cache_read_tokens"
            :cache-write="row.cache_write_tokens"
            :reasoning="row.reasoning_tokens"
          />
        </template>
        <template #cell-effort="{ row }">
          <UsageTag kind="effort" :value="row.reasoning_effort" />
        </template>
        <template #cell-status="{ row }">
          <UsageTag kind="status" :value="row.request_status" />
        </template>
        <template #cell-source="{ row }">
          <UsageTag kind="source" :value="row.request_source" />
        </template>
        <template #cell-created_at="{ row }">{{ formatDate(row.created_at) }}</template>
      </DsTable>
    </PortalContentCard>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.workspace-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 工作台主体：24px 容器承载各分区（面板 body 本身无内边距） */
.workspace-body {
  display: grid;
  gap: 20px;
  padding: 24px;
}

/* el-select 在页头操作区需要固定宽度，避免被压窄 */
.workspace-source-select {
  width: 140px;
}

.workspace-log-count {
  color: var(--ds-muted);
  font-size: 12.5px;
  white-space: nowrap;
}

.session-cell {
  min-width: 0;
}

.session-cell__title {
  margin: 0;
  overflow: hidden;
  color: var(--ds-ink);
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.session-cell__id {
  margin: 0;
  overflow: hidden;
  color: var(--ds-faint);
  font-family: var(--ds-font-mono);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.access-routing-copy {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-width: 720px;
}

.access-routing-eyebrow {
  margin: 0;
  color: var(--ds-accent-hover);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.22em;
  text-transform: uppercase;
}

.access-routing-title {
  margin: 0;
  color: var(--ds-ink);
  font-size: 18px;
  font-weight: 700;
}

.access-routing-desc {
  margin: 0;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.6;
}

.access-routing-error-alert {
  margin-top: 16px;
  border-radius: var(--ds-radius-panel);
  border: 1px solid color-mix(in srgb, var(--ds-danger) 22%, transparent);
  background: var(--ds-danger-soft);
  padding: 14px 16px;
}

:deep(.access-routing-error-alert .el-alert__icon) {
  color: var(--ds-danger);
}

.access-routing-error-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.access-routing-error-content strong {
  color: var(--ds-danger);
}

.access-routing-error-desc {
  margin: 4px 0 0;
  color: var(--ds-muted);
  font-size: 13px;
}

.access-routing-preview {
  margin-top: 16px;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.access-routing-hint {
  color: var(--ds-muted);
  font-size: 13px;
}
</style>
