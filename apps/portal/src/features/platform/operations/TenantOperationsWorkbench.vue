<!--
  租户经营分析驾驶舱。

  页面不再把财务理解成几张孤立的金额卡，而是按租户每天真正要做的判断组织信息：
  1. 本期消耗与 D-AI 可识别收入分别是多少；
  2. 统一额度还能支撑多久，是否有透支或即将到期额度；
  3. 哪些用户需要补充余额或关注消费；
  4. 成本集中在哪些模型、来源和最近账务动作。

  所有数字都来自真实租户范围接口；没有接口支持的推断会明确标记为“估算”。
-->
<script setup lang="ts">
import { computed, type Component } from "vue";
import { useRouter } from "vue-router";
import {
  Activity,
  ArrowRight,
  ArrowUpRight,
  BadgeDollarSign,
  Banknote,
  CircleAlert,
  Clock3,
  CreditCard,
  Landmark,
  ReceiptText,
  RefreshCw,
  ShieldCheck,
  ShoppingBag,
  TrendingDown,
  TrendingUp,
  UsersRound,
  WalletCards
} from "lucide-vue-next";

import { PortalPagePanel } from "@/platform";
import { DsButton, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";
import { formatDisplayMicroUSD, formatDisplayUSD } from "@/shared/currency";
import type { TenantBalanceLedgerItem } from "@/api/types/tenant";

import TenantWorkbenchRangeTabs from "@/components/workbench/TenantWorkbenchRangeTabs.vue";
import { WORKBENCH_RANGE_OPTIONS, buildWorkbenchRangeWindow } from "@/components/workbench/workbenchRanges";
import { balanceTransactionText } from "@/features/platform/account-center/model";
import { useTenantOperationsDashboard } from "./composables/useTenantOperationsDashboard";

type TagTone = "neutral" | "accent" | "positive" | "warning" | "danger" | "info";

interface FinancialMetric {
  key: string;
  label: string;
  value: string;
  hint: string;
  badge: string;
  tone: "accent" | "positive" | "warning" | "danger";
  icon: Component;
}

interface FinanceUserRow {
  userId: string;
  username: string;
  balanceUsd: number | null;
  consumptionUsd: number;
  transactionCount: number;
  percentage: string;
  statusLabel: string;
  statusTone: TagTone;
}

interface FinanceSignal {
  key: string;
  title: string;
  detail: string;
  action: string;
  tone: TagTone;
  icon: Component;
  path: string;
}

interface FlowRow {
  key: string;
  label: string;
  detail: string;
  value: string;
  tone: "income" | "expense" | "neutral";
  icon: Component;
}

const router = useRouter();
const dashboard = useTenantOperationsDashboard();

const numberFormatter = new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 });
const decimalFormatter = new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 1 });

const summary = computed(() => dashboard.financialSummary.value);
const overview = computed(() => dashboard.overview.value);
const balance = computed(() => dashboard.serviceBalance.value);
const rangeLabel = computed(() => dashboard.selectedRangeLabel.value);

const userRevenue = computed(() => {
  if (summary.value) return Number(summary.value.total_user_charged_usd) || 0;
  return Number(overview.value.userDeductionUsd) || 0;
});

const platformCost = computed<number | null>(() => {
  if (!summary.value) return null;
  return Number(summary.value.total_tenant_payable_usd) || 0;
});

const topupIncome = computed(() => Number(overview.value.settlementIncomeMicroUsd) || 0);
const userBalancePool = computed(() => Number(overview.value.userTotalBalanceUsd) || 0);
const totalRequests = computed(() => Number(summary.value?.total_requests) || 0);
const successfulRequests = computed(() => Number(summary.value?.successful_requests) || 0);
const failedRequests = computed(() => Number(summary.value?.failed_requests) || 0);
const successRate = computed(() =>
  totalRequests.value > 0 ? (successfulRequests.value / totalRequests.value) * 100 : null
);
const totalTokens = computed(() => Number(summary.value?.total_tokens) || 0);

const periodWindow = computed(() =>
  buildWorkbenchRangeWindow(
    WORKBENCH_RANGE_OPTIONS.find((item) => item.label === rangeLabel.value) ?? WORKBENCH_RANGE_OPTIONS[0]
  )
);
const periodDays = computed(() =>
  Math.max(1, Math.ceil((periodWindow.value.endTime - periodWindow.value.startTime) / (24 * 60 * 60 * 1000)))
);
const averageDailyCost = computed(() =>
  platformCost.value === null ? null : platformCost.value / periodDays.value
);
const runwayDays = computed(() => {
  if (averageDailyCost.value === null || averageDailyCost.value <= 0) return null;
  return Math.max(0, balance.value.availableUsd / averageDailyCost.value);
});

const expiringLot = computed(() =>
  [...(balance.value.balanceLots ?? [])]
    .filter((lot) => Number(lot.remainingUsd) > 0 && lot.expiresAt)
    .sort(
      (left, right) =>
        new Date(left.expiresAt as string).getTime() - new Date(right.expiresAt as string).getTime()
    )[0] ?? null
);
const expiringDays = computed(() => {
  if (!expiringLot.value?.expiresAt) return null;
  return Math.ceil(
    (new Date(expiringLot.value.expiresAt).getTime() - Date.now()) / (24 * 60 * 60 * 1000)
  );
});

const balanceBase = computed(() => Math.max(Number(balance.value.totalUsd) || 0, 0));
const balanceRemainingPercent = computed(() => {
  if (balanceBase.value <= 0) return 0;
  return Math.min(100, Math.max(0, (Number(balance.value.remainingUsd) / balanceBase.value) * 100));
});

const consumptionByUser = computed(
  () => new Map(dashboard.consumptionRanking.value.map((item) => [String(item.userId), item]))
);

const lowBalanceCount = computed(
  () => dashboard.users.value.filter((user) => user.status === 1 && Number(user.balanceUsd) <= 1).length
);

const userHealthRows = computed<FinanceUserRow[]>(() => {
  const rows = dashboard.users.value.map((user) => {
    const consumption = consumptionByUser.value.get(String(user.userId));
    return buildUserRow({
      userId: String(user.userId),
      username: user.username || user.nickname || user.email || String(user.userId),
      balanceUsd: Number.isFinite(Number(user.balanceUsd)) ? Number(user.balanceUsd) : null,
      consumptionUsd: Number(consumption?.amountUsd) || 0,
      transactionCount: Number(consumption?.transactionCount) || 0,
      percentage: consumption?.percentage || "0"
    });
  });

  const knownIds = new Set(rows.map((row) => row.userId));
  for (const item of dashboard.consumptionRanking.value) {
    if (knownIds.has(String(item.userId))) continue;
    rows.push(
      buildUserRow({
        userId: String(item.userId),
        username: item.username || String(item.userId),
        balanceUsd: null,
        consumptionUsd: Number(item.amountUsd) || 0,
        transactionCount: Number(item.transactionCount) || 0,
        percentage: item.percentage || "0"
      })
    );
  }

  return rows
    .sort(
      (left, right) =>
        userStatusWeight(right) - userStatusWeight(left) || right.consumptionUsd - left.consumptionUsd
    )
    .slice(0, 8);
});

function buildUserRow(input: Omit<FinanceUserRow, "statusLabel" | "statusTone">): FinanceUserRow {
  if (input.balanceUsd === null) {
    return { ...input, statusLabel: "待同步余额", statusTone: "neutral" };
  }
  if (input.balanceUsd <= 0) {
    return { ...input, statusLabel: "余额不足", statusTone: "danger" };
  }
  if (input.balanceUsd <= 1) {
    return { ...input, statusLabel: "余额偏低", statusTone: "warning" };
  }
  return { ...input, statusLabel: "余额正常", statusTone: "positive" };
}

function userStatusWeight(row: FinanceUserRow) {
  if (row.statusTone === "danger") return 3;
  if (row.statusTone === "warning") return 2;
  if (row.statusTone === "neutral") return 1;
  return 0;
}

const modelTotalCost = computed(() =>
  dashboard.topModels.value.reduce(
    (total, model) => total + (Number(model.total_tenant_payable_usd) || 0),
    0
  )
);
const modelMaxCost = computed(() =>
  Math.max(...dashboard.topModels.value.map((model) => Number(model.total_tenant_payable_usd) || 0), 0)
);
const modelMaxRequests = computed(() =>
  Math.max(...dashboard.topModels.value.map((model) => Number(model.request_count) || 0), 0)
);

function modelBarWidth(model: { total_tenant_payable_usd: number; request_count: number }) {
  const value =
    modelMaxCost.value > 0
      ? (Number(model.total_tenant_payable_usd) || 0) / modelMaxCost.value
      : modelMaxRequests.value > 0
        ? (Number(model.request_count) || 0) / modelMaxRequests.value
        : 0;
  return `${Math.max(0, Math.min(100, value * 100))}%`;
}

const sourceTotal = computed(() =>
  dashboard.appConsumption.value.reduce((total, item) => total + (Number(item.amountUsd) || 0), 0)
);

function sourceBarWidth(item: { amountUsd: number; percentage: string }) {
  const percentage = Number.parseFloat(item.percentage);
  if (Number.isFinite(percentage)) return `${Math.max(0, Math.min(100, percentage))}%`;
  if (sourceTotal.value <= 0) return "0%";
  return `${Math.max(0, Math.min(100, ((Number(item.amountUsd) || 0) / sourceTotal.value) * 100))}%`;
}

function sourceLabel(value: string) {
  return (
    {
      api_key: "API 调用",
      web_chat: "网页对话",
      web_image: "网页生图",
      workspace: "工作台",
      "": "D-AI"
    } as Record<string, string>
  )[value] || value || "其他来源";
}

const recentUsageRows = computed(() =>
  dashboard.recentConsumption.value.slice(0, 6).map((record) => ({
    id: record.request_id,
    user: record.username || record.user_id || record.external_user_id || "未知用户",
    model: record.model_code || "未命名模型",
    source: sourceLabel(record.request_source),
    chargedUsd: Number(record.user_charged_usd) || 0,
    payableUsd: Number(record.tenant_payable_usd) || 0,
    createdAt: record.created_at
  }))
);

const financeMetrics = computed<FinancialMetric[]>(() => [
  {
    key: "user-revenue",
    label: "用户实收",
    value: dashboard.summaryLoading.value ? "—" : formatUSD(userRevenue.value),
    hint: "终端用户实际扣款；订阅覆盖流量可能为 0",
    badge: rangeLabel.value,
    tone: "positive",
    icon: BadgeDollarSign
  },
  {
    key: "platform-cost",
    label: "平台服务成本",
    value:
      dashboard.summaryLoading.value || platformCost.value === null
        ? "—"
        : formatUSD(platformCost.value),
    hint: "本租户按实际 AI 用量应付平台的结算金额",
    badge: rangeLabel.value,
    tone: "danger",
    icon: ReceiptText
  },
  {
    key: "topup-income",
    label: "用户充值入账",
    value: dashboard.summaryLoading.value ? "—" : formatMicroUSD(topupIncome.value),
    hint: "用户充值进入租户额度账户的入账金额",
    badge: rangeLabel.value,
    tone: "accent",
    icon: CreditCard
  },
  {
    key: "user-balance",
    label: "用户余额池",
    value: dashboard.summaryLoading.value ? "—" : formatUSD(userBalancePool.value),
    hint: "当前仍由终端用户持有的可用额度",
    badge: "当前存量",
    tone: "warning",
    icon: UsersRound
  }
]);

const flowRows = computed<FlowRow[]>(() => [
  {
    key: "topup",
    label: "用户充值入账",
    detail: "进入租户统一额度账户",
    value: dashboard.summaryLoading.value ? "—" : formatMicroUSD(topupIncome.value),
    tone: "income",
    icon: CreditCard
  },
  {
    key: "charged",
    label: "用户消费扣款",
    detail: "终端用户实际扣款",
    value: dashboard.summaryLoading.value ? "—" : formatUSD(userRevenue.value),
    tone: "income",
    icon: ShoppingBag
  },
  {
    key: "cost",
    label: "平台服务成本",
    detail: "从租户侧结算的 AI 服务成本",
    value:
      dashboard.summaryLoading.value || platformCost.value === null
        ? "—"
        : `-${formatUSD(platformCost.value)}`,
    tone: "expense",
    icon: TrendingDown
  },
]);

const financeStatus = computed<{ label: string; tone: TagTone }>(() => {
  if (dashboard.loading.value) return { label: "正在计算", tone: "neutral" };
  if (summary.value && totalRequests.value === 0 && userRevenue.value === 0 && platformCost.value === 0) {
    return { label: "暂无用量", tone: "neutral" };
  }
  if (summary.value && totalRequests.value > 0 && userRevenue.value === 0 && platformCost.value === 0) {
    return { label: "暂无实收", tone: "info" };
  }
  if (balance.value.outstandingDebtMicroUsd > 0) return { label: "存在透支", tone: "danger" };
  if (platformCost.value !== null && platformCost.value > 0 && userRevenue.value <= 0) return { label: "存在未归因消耗", tone: "warning" };
  if (userRevenue.value > 0) return { label: "收入已记录", tone: "positive" };
  return { label: "数据待完善", tone: "neutral" };
});

const headline = computed(() => {
  if (dashboard.loading.value) return "正在生成本期经营结论";
  if (summary.value && totalRequests.value === 0 && userRevenue.value === 0 && platformCost.value === 0) {
    return "本期暂无已结算用量";
  }
  if (platformCost.value !== null && platformCost.value > 0 && userRevenue.value <= 0) return "本期有平台消耗，但暂无 D-AI 可归因收入";
  if (userRevenue.value > 0) return "本期已记录 D-AI 可归因收入";
  return "本期消耗与收入数据待完善";
});

const headlineDetail = computed(() => {
  if (dashboard.loading.value) return "正在汇总账户、结算、用户余额和 AI 用量数据。";
  if (summary.value && totalRequests.value === 0 && userRevenue.value === 0 && platformCost.value === 0) {
    return `${rangeLabel.value}暂未产生已结算 AI 用量；可以先检查模型分组、API 密钥和用户引导是否已配置。`;
  }
  if (platformCost.value !== null && platformCost.value > 0 && userRevenue.value <= 0) {
    return `本期平台服务成本 ${formatUSD(platformCost.value)}，D-AI 可归因收入为 ${formatUSD(userRevenue.value)}；外部 API 或租户自有平台产生的收入不在本系统统计。`;
  }
  return `本期记录 D-AI 可归因收入 ${formatUSD(userRevenue.value)}、平台服务成本 ${formatUSD(platformCost.value)}；请结合外部平台账务判断整体经营结果。`;
});

const primaryAction = computed(() => {
  if (balance.value.outstandingDebtMicroUsd > 0 || (runwayDays.value !== null && runwayDays.value < 7)) {
    return {
      label: "补充服务额度",
      detail: "避免额度不足影响用户调用",
      path: "/tenant/account?action=buy"
    };
  }
  if (lowBalanceCount.value > 0) {
    return {
      label: "处理低余额用户",
      detail: `已有 ${numberFormatter.format(lowBalanceCount.value)} 名用户余额偏低`,
      path: "/tenant/users/directory"
    };
  }
  return {
    label: "检查模型定价",
    detail: "确认零售价覆盖最新平台结算成本",
    path: "/tenant/ai/models/prices"
  };
});

const signals = computed<FinanceSignal[]>(() => {
  const items: FinanceSignal[] = [];
  if (balance.value.outstandingDebtMicroUsd > 0) {
    items.push({
      key: "debt",
      title: "统一额度账户已透支",
      detail: `当前透支 ${formatMicroUSD(balance.value.outstandingDebtMicroUsd)}，建议尽快补充服务额度。`,
      action: "查看流水",
      tone: "danger",
      icon: CircleAlert,
      path: "/tenant/account?tab=ledger"
    });
  }
  if (expiringLot.value && expiringDays.value !== null && expiringDays.value <= 7) {
    items.push({
      key: "expiry",
      title: "有一笔限时额度即将到期",
      detail: `${formatUSD(expiringLot.value.remainingUsd)} 将在 ${Math.max(0, expiringDays.value)} 天内到期。`,
      action: "查看额度",
      tone: "warning",
      icon: Clock3,
      path: "/tenant/account?tab=ledger"
    });
  }
  if (runwayDays.value !== null && runwayDays.value < 7) {
    items.push({
      key: "runway",
      title: "服务额度安全垫偏薄",
      detail: `按${rangeLabel.value}日均成本估算，仅可支撑 ${runwayText.value}。`,
      action: "去充值",
      tone: "warning",
      icon: WalletCards,
      path: "/tenant/account?action=buy"
    });
  }
  if (lowBalanceCount.value > 0) {
    items.push({
      key: "users",
      title: "用户余额需要运营跟进",
      detail: `${numberFormatter.format(lowBalanceCount.value)} 名活跃用户余额不超过 $1。`,
      action: "查看用户",
      tone: "info",
      icon: UsersRound,
      path: "/tenant/users/directory"
    });
  }
  if (failedRequests.value > 0) {
    items.push({
      key: "failures",
      title: "本期有失败请求",
      detail: `${numberFormatter.format(failedRequests.value)} 次失败请求可能影响用户体验和收入转化。`,
      action: "查看用量",
      tone: "info",
      icon: Activity,
      path: "/tenant/ai/usage"
    });
  }
  if (items.length) return items.slice(0, 4);
  return [{
    key: "healthy",
    title: "目前没有需要立即处理的财务信号",
    detail: "继续关注服务额度、用户余额和本期毛利变化。",
    action: "查看完整分析",
    tone: "positive",
    icon: ShieldCheck,
    path: "/tenant/ai/usage"
  }];
});

const runwayText = computed(() => {
  if (runwayDays.value === null) return "暂无消耗";
  if (runwayDays.value >= 365) return "超过 365 天";
  return `${decimalFormatter.format(runwayDays.value)} 天`;
});

const runwayHint = computed(() => {
  if (averageDailyCost.value === null) return "暂缺平台成本数据";
  if (averageDailyCost.value <= 0) return `${rangeLabel.value}暂无平台服务成本`;
  return `按${rangeLabel.value}日均 ${formatUSD(averageDailyCost.value)} 估算`;
});

const expiryHint = computed(() => {
  if (!expiringLot.value?.expiresAt) return "当前没有限时额度";
  if (expiringDays.value !== null && expiringDays.value <= 7) return `${Math.max(0, expiringDays.value)} 天内到期`;
  return `${formatShortDate(expiringLot.value.expiresAt)} 到期`;
});

const balanceStatus = computed<{ label: string; tone: TagTone }>(() => {
  if (balance.value.outstandingDebtMicroUsd > 0) return { label: "已透支", tone: "danger" };
  if (runwayDays.value !== null && runwayDays.value < 7) return { label: "安全垫偏薄", tone: "warning" };
  if (balance.value.serviceState !== "active") return { label: "服务受限", tone: "danger" };
  return { label: "服务正常", tone: "positive" };
});

const netCashReference = computed<number | null>(() =>
  platformCost.value === null ? null : topupIncome.value / 1_000_000 - platformCost.value
);

const ledgerColumns: DsTableColumn[] = [
  { key: "txnType", title: "账务动作", width: 150 },
  { key: "amount", title: "变动金额", width: 150, align: "right" },
  { key: "balanceAfter", title: "变动后余额", width: 150, align: "right" },
  { key: "note", title: "备注", wrap: true },
  { key: "createdAt", title: "发生时间", width: 170 }
];

const usageColumns: DsTableColumn[] = [
  { key: "user", title: "用户", width: 150 },
  { key: "model", title: "模型" },
  { key: "source", title: "来源", width: 110 },
  { key: "charged", title: "用户扣款", width: 120, align: "right" },
  { key: "payable", title: "平台成本", width: 120, align: "right" },
  { key: "createdAt", title: "时间", width: 145 }
];

const userColumns: DsTableColumn[] = [
  { key: "user", title: "用户", width: 180 },
  { key: "status", title: "余额状态", width: 120 },
  { key: "balance", title: "当前余额", width: 120, align: "right" },
  { key: "consumption", title: "本期消费", width: 120, align: "right" },
  { key: "transactions", title: "消费次数", width: 100, align: "right" },
  { key: "percentage", title: "消费占比", width: 100, align: "right" }
];

function formatUSD(value: number | null | undefined) {
  return formatDisplayUSD(value);
}

function formatMicroUSD(value: number | null | undefined) {
  return formatDisplayMicroUSD(value);
}

function signedMoney(value: number | null | undefined) {
  const numeric = Number(value) || 0;
  return `${numeric < 0 ? "-" : "+"}${formatUSD(Math.abs(numeric))}`;
}

function formatPercent(value: number | null | undefined) {
  if (value === null || value === undefined || !Number.isFinite(Number(value))) return "—";
  return `${Number(value).toFixed(1).replace(/\.0$/, "")}%`;
}

function formatCount(value: number | null | undefined) {
  return numberFormatter.format(Number(value) || 0);
}

function formatCompactTokens(value: number | null | undefined) {
  const numeric = Number(value) || 0;
  if (Math.abs(numeric) < 1000) return formatCount(numeric);
  if (Math.abs(numeric) < 1_000_000) return `${(numeric / 1000).toFixed(1).replace(/\.0$/, "")}K`;
  return `${(numeric / 1_000_000).toFixed(1).replace(/\.0$/, "")}M`;
}

function formatTime(value?: number | string | null) {
  if (!value) return "—";
  return new Date(value).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false
  });
}

function formatShortDate(value?: number | string | null) {
  if (!value) return "—";
  return new Date(value).toLocaleDateString("zh-CN", { month: "2-digit", day: "2-digit" });
}

function ledgerAmountClass(value: number) {
  return value >= 0 ? "amount-positive" : "amount-negative";
}

function ledgerAmount(value: number) {
  const numeric = Number(value) || 0;
  return `${numeric >= 0 ? "+" : "-"}${formatMicroUSD(Math.abs(numeric))}`;
}

function ledgerType(item: TenantBalanceLedgerItem) {
  return balanceTransactionText(item.txnType);
}

function ledgerNote(item: TenantBalanceLedgerItem) {
  return item.note || item.refId || "—";
}

function goTo(path: string) {
  void router.push(path);
}

function handleRangeChange(rangeId: string) {
  void dashboard.selectRange(rangeId);
}
</script>

<template>
  <div class="page-container finance-workbench">
    <PortalPagePanel
      fill
      :icon="Banknote"
      :breadcrumbs="[{ label: '概览' }, { label: '经营分析' }]"
      description="把收入、平台成本、用户余额与账务风险放在同一个租户经营视图里。"
    >
      <template #actions>
        <TenantWorkbenchRangeTabs
          :model-value="dashboard.selectedRangeId.value"
          :options="WORKBENCH_RANGE_OPTIONS"
          :loading="dashboard.loading.value"
          aria-label="经营分析时间范围"
          @update:model-value="handleRangeChange"
        />
        <DsButton variant="ghost" size="sm" @click="goTo('/tenant/account?tab=ledger')">
          <template #icon><ArrowUpRight :size="14" /></template>
          账户流水
        </DsButton>
        <DsButton variant="primary" size="sm" :loading="dashboard.loading.value" @click="dashboard.refresh">
          <template #icon><RefreshCw :size="14" /></template>
          刷新
        </DsButton>
      </template>

      <div class="finance-body" :aria-busy="dashboard.loading.value">
        <section class="finance-hero" aria-labelledby="finance-hero-title">
          <div class="finance-hero__copy">
            <div class="finance-hero__eyebrow">
              <span class="finance-hero__eyebrow-dot" aria-hidden="true" />
              <span>本期经营结论</span>
              <DsTag :tone="financeStatus.tone">{{ financeStatus.label }}</DsTag>
            </div>
            <h2 id="finance-hero-title" class="finance-hero__title">{{ headline }}</h2>
            <p class="finance-hero__detail">{{ headlineDetail }}</p>
            <div class="finance-hero__numbers">
              <div class="hero-number">
                <span>D-AI 可归因收入</span>
                <strong class="amount-positive">{{ dashboard.summaryLoading.value ? "—" : formatUSD(userRevenue) }}</strong>
              </div>
              <div class="hero-number">
                <span>平台服务成本</span>
                <strong class="amount-negative">{{ dashboard.summaryLoading.value || platformCost === null ? "—" : formatUSD(platformCost) }}</strong>
              </div>
              <div class="hero-number">
                <span>本期请求</span>
                <strong>{{ dashboard.summaryLoading.value ? "—" : formatCount(totalRequests) }}</strong>
              </div>
              <div class="hero-number">
                <span>本期 Token</span>
                <strong>{{ dashboard.summaryLoading.value ? "—" : formatCompactTokens(totalTokens) }}</strong>
              </div>
              <div class="hero-number">
                <span>成功率</span>
                <strong>{{ dashboard.summaryLoading.value ? "—" : formatPercent(successRate) }}</strong>
              </div>
            </div>
          </div>
          <div class="finance-hero__action">
            <span class="finance-hero__action-label">建议现在做</span>
            <strong>{{ primaryAction.label }}</strong>
            <p>{{ primaryAction.detail }}</p>
            <DsButton variant="secondary" size="sm" @click="goTo(primaryAction.path)">
              去处理
              <ArrowRight :size="14" />
            </DsButton>
          </div>
        </section>

        <section class="finance-section" aria-labelledby="financial-metrics-title">
          <div class="section-heading">
            <div>
              <span class="section-heading__eyebrow">MONEY IN MOTION</span>
              <h2 id="financial-metrics-title">本期消耗与收入</h2>
            </div>
            <span class="section-heading__note">{{ rangeLabel }} · 已结算口径</span>
          </div>

          <div class="financial-metrics">
            <article
              v-for="metric in financeMetrics"
              :key="metric.key"
              class="financial-metric"
              :class="`financial-metric--${metric.tone}`"
            >
              <div class="financial-metric__topline">
                <span class="financial-metric__icon"><component :is="metric.icon" :size="17" /></span>
                <span class="financial-metric__badge">{{ metric.badge }}</span>
              </div>
              <span class="financial-metric__label">{{ metric.label }}</span>
              <strong class="financial-metric__value">{{ metric.value }}</strong>
              <span class="financial-metric__hint">{{ metric.hint }}</span>
            </article>
          </div>
        </section>

        <section class="finance-grid finance-grid--primary" aria-label="额度与资金流">
          <article class="finance-card cash-position">
            <header class="finance-card__head">
              <div class="finance-card__title-wrap">
                <span class="finance-card__icon finance-card__icon--accent"><WalletCards :size="18" /></span>
                <div>
                  <h2>服务额度安全垫</h2>
                  <p>统一额度账户决定 AI 服务还能稳定运行多久。</p>
                </div>
              </div>
              <DsTag :tone="balanceStatus.tone">{{ balanceStatus.label }}</DsTag>
            </header>

            <div class="cash-position__main">
              <div>
                <span class="cash-position__label">当前可用额度</span>
                <strong class="cash-position__value">{{ dashboard.serviceBalanceLoading.value ? "—" : formatUSD(balance.availableUsd) }}</strong>
              </div>
              <div class="cash-position__runway">
                <span>预计可支撑</span>
                <strong>{{ dashboard.serviceBalanceLoading.value ? "—" : runwayText }}</strong>
                <small>{{ runwayHint }}</small>
              </div>
            </div>

            <div
              class="balance-meter"
              role="progressbar"
              aria-label="剩余额度比例"
              aria-valuemin="0"
              aria-valuemax="100"
              :aria-valuenow="balanceRemainingPercent"
            >
              <span class="balance-meter__fill" :style="{ width: `${balanceRemainingPercent}%` }" />
            </div>
            <div class="balance-meter__meta">
              <span>剩余 {{ formatUSD(balance.remainingUsd) }}</span>
              <span>已用 {{ formatUSD(balance.usedUsd) }} / 总额 {{ formatUSD(balance.totalUsd) }}</span>
            </div>

            <div class="cash-position__stats">
              <div>
                <span>当前透支</span>
                <strong :class="balance.outstandingDebtMicroUsd > 0 ? 'amount-negative' : ''">{{ formatMicroUSD(balance.outstandingDebtMicroUsd) }}</strong>
                <small>{{ balance.outstandingDebtMicroUsd > 0 ? "会持续压缩可用额度" : "账户没有透支" }}</small>
              </div>
              <div>
                <span>限时额度</span>
                <strong>{{ formatUSD(balance.timedUsd) }}</strong>
                <small>{{ expiryHint }}</small>
              </div>
              <div>
                <span>长期有效</span>
                <strong>{{ formatUSD(balance.permanentUsd) }}</strong>
                <small>不受有效期影响</small>
              </div>
            </div>

            <footer class="finance-card__footer-actions">
              <DsButton variant="primary" size="sm" @click="goTo('/tenant/account?action=buy')">
                <template #icon><CreditCard :size="14" /></template>
                充值服务额度
              </DsButton>
              <DsButton variant="ghost" size="sm" @click="goTo('/tenant/account?tab=ledger')">
                查看完整流水
                <ArrowRight :size="14" />
              </DsButton>
            </footer>
          </article>

          <article class="finance-card flow-card">
            <header class="finance-card__head">
              <div class="finance-card__title-wrap">
                <span class="finance-card__icon finance-card__icon--positive"><TrendingUp :size="18" /></span>
                <div>
                  <h2>本期资金流向</h2>
                  <p>把用户资金、消费和平台结算放在同一条链路里看。</p>
                </div>
              </div>
              <span class="finance-card__period">{{ rangeLabel }}</span>
            </header>

            <div class="flow-list">
              <div v-for="(flow, index) in flowRows" :key="flow.key" class="flow-row">
                <div class="flow-row__line" aria-hidden="true">
                  <span class="flow-row__node" :class="`flow-row__node--${flow.tone}`"><component :is="flow.icon" :size="14" /></span>
                  <span v-if="index < flowRows.length - 1" class="flow-row__connector" />
                </div>
                <div class="flow-row__copy">
                  <strong>{{ flow.label }}</strong>
                  <span>{{ flow.detail }}</span>
                </div>
                <strong class="flow-row__amount" :class="`flow-row__amount--${flow.tone}`">{{ flow.value }}</strong>
              </div>
            </div>

            <div class="flow-card__summary">
              <div>
                <span>参考资金变化</span>
                <strong :class="netCashReference === null || netCashReference >= 0 ? 'amount-positive' : 'amount-negative'">
                  {{ dashboard.summaryLoading.value || netCashReference === null ? "—" : signedMoney(netCashReference) }}
                </strong>
              </div>
              <small>仅按 D-AI 账户充值入账减平台服务成本计算，不代表租户在其他平台的真实收入或利润。</small>
            </div>
          </article>
        </section>

        <section class="finance-grid finance-grid--secondary" aria-label="用户资金健康与经营信号">
          <article class="finance-card user-health-card">
            <header class="finance-card__head">
              <div class="finance-card__title-wrap">
                <span class="finance-card__icon finance-card__icon--warning"><UsersRound :size="18" /></span>
                <div>
                  <h2>用户资金健康</h2>
                  <p>优先展示低余额用户和本期高消耗用户，提前处理续费与流失风险。</p>
                </div>
              </div>
              <div class="finance-card__head-meta">
                <DsTag tone="warning">低余额 {{ formatCount(lowBalanceCount) }}</DsTag>
                <button type="button" class="text-action" @click="goTo('/tenant/users/directory')">用户管理 <ArrowUpRight :size="13" /></button>
              </div>
            </header>
            <div class="finance-table-wrap">
              <DsTable
                :columns="userColumns"
                :rows="userHealthRows"
                row-key="userId"
                :loading="dashboard.usersLoading.value || dashboard.rankingLoading.value"
                empty-title="暂无用户资金数据"
                empty-description="创建终端用户并产生充值或消费后，这里会显示资金健康状态。"
                aria-label="用户资金健康"
              >
                <template #cell-user="{ row }">
                  <div class="table-primary">
                    <strong>{{ row.username }}</strong>
                    <small>{{ row.userId }}</small>
                  </div>
                </template>
                <template #cell-status="{ row }"><DsTag :tone="row.statusTone">{{ row.statusLabel }}</DsTag></template>
                <template #cell-balance="{ row }"><strong>{{ row.balanceUsd === null ? "—" : formatUSD(row.balanceUsd) }}</strong></template>
                <template #cell-consumption="{ row }"><strong class="amount-positive">{{ formatUSD(row.consumptionUsd) }}</strong></template>
                <template #cell-transactions="{ row }">{{ formatCount(row.transactionCount) }}</template>
                <template #cell-percentage="{ row }">{{ row.percentage }}%</template>
              </DsTable>
            </div>
          </article>

          <article class="finance-card signals-card">
            <header class="finance-card__head">
              <div class="finance-card__title-wrap">
                <span class="finance-card__icon finance-card__icon--info"><CircleAlert :size="18" /></span>
                <div>
                  <h2>需要关注</h2>
                  <p>把财务数据直接翻译成下一步动作。</p>
                </div>
              </div>
            </header>
            <div class="signal-list">
              <article v-for="signal in signals" :key="signal.key" class="signal-item" :class="`signal-item--${signal.tone}`">
                <span class="signal-item__icon"><component :is="signal.icon" :size="15" /></span>
                <div class="signal-item__copy">
                  <strong>{{ signal.title }}</strong>
                  <p>{{ signal.detail }}</p>
                </div>
                <button type="button" class="signal-item__action" @click="goTo(signal.path)">{{ signal.action }} <ArrowRight :size="13" /></button>
              </article>
            </div>
            <div class="signals-card__mini-links">
              <button type="button" @click="goTo('/tenant/ai/usage')"><Activity :size="14" />查看用量分析</button>
              <button type="button" @click="goTo('/tenant/ai/models/prices')"><ReceiptText :size="14" />检查模型定价</button>
            </div>
          </article>
        </section>

        <section class="finance-section" aria-labelledby="structure-title">
          <div class="section-heading">
            <div>
              <span class="section-heading__eyebrow">COST STRUCTURE</span>
              <h2 id="structure-title">成本结构与最近动作</h2>
            </div>
            <button type="button" class="text-action" @click="goTo('/tenant/ai/usage')">进入用量分析 <ArrowUpRight :size="13" /></button>
          </div>

          <div class="finance-grid finance-grid--structure">
            <article class="finance-card structure-card">
              <header class="finance-card__head">
                <div class="finance-card__title-wrap">
                  <span class="finance-card__icon finance-card__icon--accent"><ReceiptText :size="18" /></span>
                  <div>
                    <h2>模型成本结构</h2>
                    <p>按请求量展示，并标出平台服务成本，方便优先复核高金额模型。</p>
                  </div>
                </div>
                <span class="finance-card__period">Top 6 · {{ formatUSD(modelTotalCost) }}</span>
              </header>
              <div v-if="!dashboard.structureLoading.value && dashboard.topModels.value.length" class="model-list">
                <div v-for="(model, index) in dashboard.topModels.value" :key="model.model_code" class="model-row">
                  <span class="model-row__rank">{{ String(index + 1).padStart(2, "0") }}</span>
                  <div class="model-row__copy">
                    <strong>{{ model.model_code }}</strong>
                    <span>{{ formatCount(model.request_count) }} 次请求 · {{ formatCompactTokens(model.total_tokens) }} Token</span>
                  </div>
                  <div class="model-row__bar"><span :style="{ width: modelBarWidth(model) }" /></div>
                  <strong class="model-row__amount">{{ formatUSD(model.total_tenant_payable_usd) }}</strong>
                </div>
              </div>
              <div v-else-if="dashboard.structureLoading.value" class="structure-empty">正在加载模型成本结构…</div>
              <div v-else class="structure-empty"><ReceiptText :size="24" />当前时间范围内暂无模型成本数据</div>
            </article>

            <article class="finance-card structure-card">
              <header class="finance-card__head">
                <div class="finance-card__title-wrap">
                  <span class="finance-card__icon finance-card__icon--positive"><Activity :size="18" /></span>
                  <div>
                    <h2>消费来源结构</h2>
                    <p>确认用户消费主要来自 API、网页对话还是其他入口。</p>
                  </div>
                </div>
                <span class="finance-card__period">{{ formatUSD(sourceTotal) }}</span>
              </header>
              <div v-if="!dashboard.structureLoading.value && dashboard.appConsumption.value.length" class="source-list">
                <div v-for="item in dashboard.appConsumption.value" :key="item.clientId || item.clientName" class="source-row">
                  <div class="source-row__head"><strong>{{ sourceLabel(item.clientId || item.clientName) }}</strong><span>{{ item.percentage || "0" }}%</span></div>
                  <div class="source-row__bar"><span :style="{ width: sourceBarWidth(item) }" /></div>
                  <div class="source-row__meta"><span>{{ formatUSD(item.amountUsd) }} 用户实收</span><span>{{ item.clientId || "D-AI" }}</span></div>
                </div>
              </div>
              <div v-else-if="dashboard.structureLoading.value" class="structure-empty">正在加载消费来源结构…</div>
              <div v-else class="structure-empty"><Activity :size="24" />当前时间范围内暂无消费来源数据</div>
            </article>
          </div>
        </section>

        <section class="finance-grid finance-grid--activity" aria-label="近期用户消费与账务流水">
          <article class="finance-card activity-card">
            <header class="finance-card__head">
              <div class="finance-card__title-wrap">
                <span class="finance-card__icon finance-card__icon--warning"><ShoppingBag :size="18" /></span>
                <div>
                  <h2>最近用户消费</h2>
                  <p>{{ rangeLabel }}最新成功 AI 请求，方便快速核对计费结果。</p>
                </div>
              </div>
              <button type="button" class="text-action" @click="goTo('/tenant/ai/usage')">查看全部 <ArrowUpRight :size="13" /></button>
            </header>
            <div class="finance-table-wrap">
              <DsTable
                :columns="usageColumns"
                :rows="recentUsageRows"
                row-key="id"
                :loading="dashboard.recentLoading.value"
                empty-title="暂无成功消费记录"
                empty-description="当前时间范围内没有可核对的用户 AI 消费。"
                aria-label="最近用户消费"
              >
                <template #cell-user="{ row }"><strong>{{ row.user }}</strong></template>
                <template #cell-source="{ row }"><DsTag tone="neutral">{{ row.source }}</DsTag></template>
                <template #cell-charged="{ row }"><strong class="amount-positive">{{ formatUSD(row.chargedUsd) }}</strong></template>
                <template #cell-payable="{ row }"><strong class="amount-negative">{{ formatUSD(row.payableUsd) }}</strong></template>
                <template #cell-createdAt="{ row }">{{ formatTime(row.createdAt) }}</template>
              </DsTable>
            </div>
          </article>

          <article class="finance-card activity-card">
            <header class="finance-card__head">
              <div class="finance-card__title-wrap">
                <span class="finance-card__icon finance-card__icon--info"><Landmark :size="18" /></span>
                <div>
                  <h2>最近账务动作</h2>
                  <p>充值、服务消费、余额调整和提现都在这里留痕。</p>
                </div>
              </div>
              <button type="button" class="text-action" @click="goTo('/tenant/account?tab=ledger')">完整流水 <ArrowUpRight :size="13" /></button>
            </header>
            <div class="finance-table-wrap">
              <DsTable
                :columns="ledgerColumns"
                :rows="dashboard.balanceLedger.value"
                row-key="txnId"
                :loading="dashboard.ledgerLoading.value"
                empty-title="暂无账务动作"
                empty-description="充值或产生服务消费后，账务动作会记录在这里。"
                aria-label="最近账务动作"
              >
                <template #cell-txnType="{ row }"><strong>{{ ledgerType(row) }}</strong></template>
                <template #cell-amount="{ row }"><strong :class="ledgerAmountClass(row.amountMicroUsd)">{{ ledgerAmount(row.amountMicroUsd) }}</strong></template>
                <template #cell-balanceAfter="{ row }">{{ formatMicroUSD(row.balanceAfterMicroUsd) }}</template>
                <template #cell-note="{ row }"><span class="table-note">{{ ledgerNote(row) }}</span></template>
                <template #cell-createdAt="{ row }">{{ formatTime(row.createdAt) }}</template>
              </DsTable>
            </div>
          </article>
        </section>

        <footer class="finance-footnote">
          <span class="finance-footnote__icon"><CircleAlert :size="14" /></span>
          <span>口径说明：D-AI 可归因收入来自系统内实际扣款，平台服务成本来自租户结算；外部 API 和租户自有平台的收入不在本页统计。</span>
          <button type="button" @click="goTo('/tenant/account?tab=ledger')">查看原始流水 <ArrowRight :size="13" /></button>
        </footer>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.finance-workbench {
  display: flex;
  flex: 1;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
}

.finance-body {
  display: flex;
  min-width: 0;
  flex: 1;
  min-height: 0;
  flex-direction: column;
  gap: 26px;
  padding: 24px;
  background: var(--ds-paper);
}

.finance-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 260px;
  min-width: 0;
  overflow: hidden;
  border: 1px solid color-mix(in srgb, var(--ds-accent) 20%, var(--ds-line));
  border-radius: var(--ds-radius-xl);
  background: linear-gradient(115deg, color-mix(in srgb, var(--ds-accent) 11%, var(--ds-panel)) 0%, var(--ds-panel) 66%);
  box-shadow: var(--ds-shadow-panel);
}

.finance-hero__copy {
  min-width: 0;
  padding: 28px 30px 24px;
}

.finance-hero__eyebrow {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--ds-accent-hover);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.1em;
}

.finance-hero__eyebrow-dot {
  width: 7px;
  height: 7px;
  border-radius: var(--ds-radius-circle);
  background: var(--ds-accent);
  box-shadow: var(--ds-shadow-focus-wide);
}

.finance-hero__title {
  max-width: 760px;
  margin: 15px 0 8px;
  color: var(--ds-ink);
  font-size: clamp(24px, 3vw, 34px);
  font-weight: 750;
  letter-spacing: -0.04em;
  line-height: 1.18;
}

.finance-hero__detail {
  max-width: 760px;
  margin: 0;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.75;
}

.finance-hero__numbers {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 30px;
  margin-top: 24px;
}

.hero-number {
  display: flex;
  min-width: 90px;
  flex-direction: column;
  gap: 4px;
}

.hero-number span {
  color: var(--ds-muted);
  font-size: 11px;
}

.hero-number strong {
  color: var(--ds-ink);
  font-size: 17px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
}

.finance-hero__action {
  display: flex;
  align-items: flex-start;
  flex-direction: column;
  justify-content: center;
  gap: 7px;
  border-left: 1px solid var(--ds-line);
  background: color-mix(in srgb, var(--ds-panel-muted) 72%, var(--ds-panel));
  padding: 24px;
}

.finance-hero__action-label {
  color: var(--ds-faint);
  font-size: 11px;
  font-weight: 750;
  letter-spacing: 0.08em;
}

.finance-hero__action strong {
  color: var(--ds-ink);
  font-size: 17px;
  font-weight: 750;
}

.finance-hero__action p {
  margin: 0 0 8px;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.6;
}

.finance-section {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 14px;
}

.section-heading {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 16px;
}

.section-heading__eyebrow {
  display: block;
  margin-bottom: 4px;
  color: var(--ds-accent);
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.13em;
}

.section-heading h2 {
  margin: 0;
  color: var(--ds-ink);
  font-size: 18px;
  font-weight: 750;
  letter-spacing: -0.02em;
}

.section-heading__note {
  color: var(--ds-ink-soft);
  font-size: 12px;
}

.financial-metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.financial-metric {
  display: flex;
  min-width: 0;
  min-height: 156px;
  flex-direction: column;
  gap: 7px;
  border: 1px solid var(--ds-line);
  border-top: 3px solid var(--ds-accent);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  padding: 16px 18px;
  box-shadow: var(--ds-shadow-sm);
}

.financial-metric--positive {
  border-top-color: var(--ds-positive);
}

.financial-metric--warning {
  border-top-color: var(--ds-warning);
}

.financial-metric--danger {
  border-top-color: var(--ds-danger);
}

.financial-metric__topline {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.financial-metric__icon,
.finance-card__icon {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  flex: 0 0 34px;
  border-radius: var(--ds-radius-control);
  background: var(--ds-accent-soft);
  color: var(--ds-accent);
}

.financial-metric--positive .financial-metric__icon,
.finance-card__icon--positive {
  background: var(--ds-positive-soft);
  color: var(--ds-positive);
}

.financial-metric--warning .financial-metric__icon,
.finance-card__icon--warning {
  background: var(--ds-warning-soft);
  color: var(--ds-warning);
}

.financial-metric--danger .financial-metric__icon {
  background: var(--ds-danger-soft);
  color: var(--ds-danger);
}

.finance-card__icon--info {
  background: var(--ds-info-soft);
  color: var(--ds-info);
}

.financial-metric__badge,
.finance-card__period {
  color: var(--ds-faint);
  font-size: 11px;
  white-space: nowrap;
}

.financial-metric__label {
  margin-top: 7px;
  color: var(--ds-muted);
  font-size: 12px;
  font-weight: 650;
}

.financial-metric__value {
  color: var(--ds-ink);
  font-size: 27px;
  font-weight: 800;
  letter-spacing: -0.03em;
  line-height: 1.15;
  font-variant-numeric: tabular-nums;
}

.financial-metric__hint {
  margin-top: auto;
  color: var(--ds-faint);
  font-size: 11px;
  line-height: 1.45;
}

.finance-grid {
  display: grid;
  min-width: 0;
  gap: 16px;
}

.finance-grid--primary {
  grid-template-columns: minmax(0, 1.1fr) minmax(420px, 0.9fr);
}

.finance-grid--secondary {
  grid-template-columns: minmax(0, 1.15fr) minmax(340px, 0.85fr);
}

.finance-grid--structure,
.finance-grid--activity {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.finance-card {
  min-width: 0;
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.finance-card__head {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
  border-bottom: 1px solid var(--ds-line);
  padding: 17px 20px;
}

.finance-card__title-wrap {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 11px;
}

.finance-card__title-wrap > div {
  min-width: 0;
}

.finance-card__title-wrap h2 {
  margin: 0;
  color: var(--ds-ink);
  font-size: 15px;
  font-weight: 750;
}

.finance-card__title-wrap p {
  margin: 4px 0 0;
  color: var(--ds-muted);
  font-size: 11.5px;
  line-height: 1.5;
}

.finance-card__footer-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  border-top: 1px solid var(--ds-line);
  padding: 14px 20px;
}

.cash-position__main {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 20px;
  padding: 22px 20px 16px;
}

.cash-position__label,
.cash-position__runway span,
.cash-position__stats span,
.flow-card__summary span {
  display: block;
  color: var(--ds-muted);
  font-size: 11.5px;
}

.cash-position__value {
  display: block;
  margin-top: 5px;
  color: var(--ds-ink);
  font-size: 34px;
  font-weight: 850;
  letter-spacing: -0.05em;
  line-height: 1.1;
  font-variant-numeric: tabular-nums;
}

.cash-position__runway {
  min-width: 142px;
  text-align: right;
}

.cash-position__runway strong {
  display: block;
  margin-top: 4px;
  color: var(--ds-accent-hover);
  font-size: 20px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
}

.cash-position__runway small,
.cash-position__stats small {
  display: block;
  margin-top: 4px;
  color: var(--ds-faint);
  font-size: 10.5px;
  line-height: 1.45;
}

.balance-meter {
  height: 9px;
  overflow: hidden;
  margin: 0 20px;
  border-radius: var(--ds-radius-pill);
  background: var(--ds-panel-muted);
}

.balance-meter__fill {
  display: block;
  height: 100%;
  border-radius: var(--ds-radius-pill);
  background: var(--ds-accent);
  transition: width 180ms ease;
}

.balance-meter__meta {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  padding: 8px 20px 18px;
  color: var(--ds-faint);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}

.cash-position__stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 1px;
  margin: 0 20px 20px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-line);
}

.cash-position__stats > div {
  min-width: 0;
  background: var(--ds-panel-muted);
  padding: 12px;
}

.cash-position__stats > div:first-child {
  border-radius: var(--ds-radius-control) 0 0 var(--ds-radius-control);
}

.cash-position__stats > div:last-child {
  border-radius: var(--ds-radius-none) var(--ds-radius-control) var(--ds-radius-control) var(--ds-radius-none);
}

.cash-position__stats strong {
  display: block;
  margin-top: 5px;
  color: var(--ds-ink);
  font-size: 14px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
}

.flow-list {
  padding: 18px 20px 6px;
}

.flow-row {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr) auto;
  align-items: center;
  gap: 11px;
  min-height: 54px;
}

.flow-row__line {
  position: relative;
  display: flex;
  height: 54px;
  align-items: flex-start;
  justify-content: center;
}

.flow-row__node {
  position: relative;
  z-index: 1;
  display: grid;
  width: 27px;
  height: 27px;
  place-items: center;
  border-radius: var(--ds-radius-circle);
  background: var(--ds-panel-muted);
  color: var(--ds-muted);
}

.flow-row__node--income {
  background: var(--ds-positive-soft);
  color: var(--ds-positive);
}

.flow-row__node--expense {
  background: var(--ds-danger-soft);
  color: var(--ds-danger);
}

.flow-row__connector {
  position: absolute;
  top: 27px;
  bottom: -1px;
  width: 1px;
  background: var(--ds-line-strong);
}

.flow-row__copy {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
}

.flow-row__copy strong {
  overflow: hidden;
  color: var(--ds-ink-soft);
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.flow-row__copy span {
  overflow: hidden;
  color: var(--ds-faint);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.flow-row__amount {
  color: var(--ds-ink);
  font-size: 14px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
}

.flow-row__amount--income {
  color: var(--ds-positive);
}

.flow-row__amount--expense {
  color: var(--ds-danger);
}

.flow-card__summary {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: end;
  gap: 12px 20px;
  margin: 8px 20px 20px;
  border-top: 1px solid var(--ds-line);
  padding-top: 15px;
}

.flow-card__summary strong {
  display: block;
  margin-top: 4px;
  font-size: 16px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
}

.flow-card__summary small {
  color: var(--ds-faint);
  font-size: 10.5px;
  line-height: 1.55;
}

.finance-card__head-meta {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.text-action,
.signal-item__action,
.signals-card__mini-links button,
.finance-footnote button {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 0;
  background: var(--ds-panel);
  color: var(--ds-accent-hover);
  font-size: 11.5px;
  font-weight: 750;
  white-space: nowrap;
  cursor: pointer;
}

.text-action:hover,
.signal-item__action:hover,
.signals-card__mini-links button:hover,
.finance-footnote button:hover {
  color: var(--ds-accent);
  text-decoration: underline;
}

.finance-table-wrap {
  min-width: 0;
  overflow-x: auto;
  padding: 6px 10px 10px;
}

.table-primary {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
}

.table-primary strong {
  overflow: hidden;
  color: var(--ds-ink-soft);
  font-size: 12.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.table-primary small,
.table-note {
  overflow: hidden;
  color: var(--ds-ink-soft);
  font-size: 10.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:deep(.ds-tag--neutral) {
  color: var(--ds-ink-soft);
  background: var(--ds-panel-muted);
}

:deep(.ds-tag--positive) {
  color: var(--ds-positive);
  background: var(--ds-positive-soft);
}

.signals-card {
  min-height: 0;
}

.signal-list {
  display: flex;
  flex-direction: column;
  padding: 10px 20px 6px;
}

.signal-item {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr) auto;
  align-items: start;
  gap: 10px;
  padding: 12px 0;
  border-bottom: 1px solid var(--ds-line);
}

.signal-item:last-child {
  border-bottom: 0;
}

.signal-item__icon {
  display: grid;
  width: 27px;
  height: 27px;
  place-items: center;
  border-radius: var(--ds-radius-circle);
  background: var(--ds-info-soft);
  color: var(--ds-info);
}

.signal-item--danger .signal-item__icon {
  background: var(--ds-danger-soft);
  color: var(--ds-danger);
}

.signal-item--warning .signal-item__icon {
  background: var(--ds-warning-soft);
  color: var(--ds-warning);
}

.signal-item--positive .signal-item__icon {
  background: var(--ds-positive-soft);
  color: var(--ds-positive);
}

.signal-item__copy {
  min-width: 0;
}

.signal-item__copy strong {
  display: block;
  color: var(--ds-ink-soft);
  font-size: 12.5px;
  line-height: 1.4;
}

.signal-item__copy p {
  margin: 4px 0 0;
  color: var(--ds-muted);
  font-size: 11px;
  line-height: 1.55;
}

.signal-item__action {
  padding-top: 3px;
  color: var(--ds-muted);
  font-size: 10.5px;
}

.signals-card__mini-links {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 16px;
  border-top: 1px solid var(--ds-line);
  padding: 13px 20px 16px;
}

.signals-card__mini-links button {
  padding: 0;
  color: var(--ds-muted);
  font-size: 11px;
}

.model-list,
.source-list {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 15px 20px 18px;
}

.model-row {
  display: grid;
  grid-template-columns: 25px minmax(120px, 0.8fr) minmax(90px, 1fr) auto;
  align-items: center;
  gap: 10px;
  min-height: 43px;
  border-bottom: 1px solid var(--ds-line);
}

.model-row:last-child {
  border-bottom: 0;
}

.model-row__rank {
  color: var(--ds-faint);
  font-size: 11px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
}

.model-row__copy {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
}

.model-row__copy strong {
  overflow: hidden;
  color: var(--ds-ink-soft);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.model-row__copy span {
  color: var(--ds-faint);
  font-size: 10px;
}

.model-row__bar,
.source-row__bar {
  height: 7px;
  overflow: hidden;
  border-radius: var(--ds-radius-pill);
  background: var(--ds-panel-muted);
}

.model-row__bar span,
.source-row__bar span {
  display: block;
  height: 100%;
  border-radius: var(--ds-radius-pill);
  background: var(--ds-accent);
}

.model-row__amount {
  min-width: 66px;
  color: var(--ds-danger);
  font-size: 12px;
  font-weight: 800;
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.source-list {
  gap: 17px;
}

.source-row {
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.source-row__head,
.source-row__meta {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}

.source-row__head strong {
  color: var(--ds-ink-soft);
  font-size: 12px;
}

.source-row__head span {
  color: var(--ds-accent-hover);
  font-size: 11px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
}

.source-row__bar span {
  background: var(--ds-positive);
}

.source-row__meta {
  color: var(--ds-faint);
  font-size: 10.5px;
}

.structure-empty {
  display: flex;
  min-height: 218px;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: var(--ds-faint);
  font-size: 12px;
}

.finance-footnote {
  display: flex;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
  padding: 11px 14px;
  color: var(--ds-faint);
  font-size: 10.5px;
  line-height: 1.55;
}

.finance-footnote__icon {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  color: var(--ds-muted);
}

.finance-footnote button {
  margin-left: auto;
  flex: 0 0 auto;
  padding: 0;
  color: var(--ds-muted);
  font-size: 10.5px;
}

.amount-positive {
  color: var(--ds-positive);
}

.amount-negative {
  color: var(--ds-danger);
}

@media (max-width: 1240px) {
  .financial-metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .finance-grid--primary,
  .finance-grid--secondary {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 900px) {
  .finance-hero {
    grid-template-columns: minmax(0, 1fr);
  }

  .finance-hero__action {
    align-items: flex-start;
    border-top: 1px solid var(--ds-line);
    border-left: 0;
  }

  .finance-grid--structure,
  .finance-grid--activity {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 640px) {
  .finance-body {
    gap: 20px;
    padding: 16px;
  }

  .finance-hero__copy {
    padding: 22px 20px 20px;
  }

  .finance-hero__title {
    font-size: 25px;
  }

  .financial-metrics {
    grid-template-columns: minmax(0, 1fr);
  }

  .section-heading {
    align-items: flex-start;
    flex-direction: column;
    gap: 6px;
  }

  .finance-card__head {
    padding-inline: 16px;
  }

  .cash-position__main {
    align-items: flex-start;
    flex-direction: column;
    gap: 16px;
    padding-inline: 16px;
  }

  .cash-position__runway {
    text-align: left;
  }

  .balance-meter {
    margin-inline: 16px;
  }

  .balance-meter__meta {
    flex-direction: column;
    padding-inline: 16px;
  }

  .cash-position__stats {
    grid-template-columns: minmax(0, 1fr);
    margin-inline: 16px;
  }

  .cash-position__stats > div:first-child,
  .cash-position__stats > div:last-child {
    border-radius: var(--ds-radius-none);
  }

  .cash-position__stats > div:first-child {
    border-radius: var(--ds-radius-control) var(--ds-radius-control) var(--ds-radius-none) var(--ds-radius-none);
  }

  .cash-position__stats > div:last-child {
    border-radius: var(--ds-radius-none) var(--ds-radius-none) var(--ds-radius-control) var(--ds-radius-control);
  }

  .finance-card__footer-actions {
    align-items: flex-start;
    flex-direction: column;
    padding-inline: 16px;
  }

  .flow-list,
  .signal-list {
    padding-inline: 16px;
  }

  .flow-card__summary,
  .signals-card__mini-links {
    margin-inline: 16px;
    padding-inline: 0;
  }

  .flow-card__summary {
    grid-template-columns: minmax(0, 1fr);
  }

  .signals-card__mini-links {
    padding-bottom: 16px;
  }

  .model-list,
  .source-list {
    padding-inline: 16px;
  }

  .model-row {
    grid-template-columns: 25px minmax(0, 1fr) auto;
    grid-template-rows: auto 7px;
    gap: 7px 9px;
    padding-block: 6px;
  }

  .model-row__rank,
  .model-row__copy {
    grid-row: 1;
  }

  .model-row__bar {
    grid-column: 2 / 4;
    grid-row: 2;
  }

  .finance-footnote {
    align-items: flex-start;
    flex-wrap: wrap;
  }

  .finance-footnote button {
    margin-left: 22px;
  }
}
</style>
