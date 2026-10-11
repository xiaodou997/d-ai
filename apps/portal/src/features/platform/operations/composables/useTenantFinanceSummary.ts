import { computed } from "vue";
import { Activity, BadgeDollarSign, CircleAlert, Clock3, CreditCard, ReceiptText, ShieldCheck, ShoppingBag, TrendingDown, UsersRound, WalletCards } from "lucide-vue-next";
import { WORKBENCH_RANGE_OPTIONS, buildWorkbenchRangeWindow } from "@/components/workbench/workbenchRanges";
import { type TagTone, type FinancialMetric, type FinanceSignal, type FlowRow, numberFormatter, decimalFormatter, formatUSD, formatMicroUSD, formatShortDate } from "../financePresentation";
import type { TenantOperationsDashboard } from "./useTenantOperationsDashboard";

export function useTenantFinanceSummary(dashboard: TenantOperationsDashboard) {
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

  const errorRate = computed(() =>
    totalRequests.value > 0 ? ((totalRequests.value - successfulRequests.value) / totalRequests.value) * 100 : null
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

  const lowBalanceCount = computed(
    () => overview.value.lowBalanceUserCount
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
        detail: `${numberFormatter.format(lowBalanceCount.value)} 名启用用户余额不超过 $1。`,
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

  return {
    balance,
    rangeLabel,
    userRevenue,
    platformCost,
    totalRequests,
    errorRate,
    totalTokens,
    balanceRemainingPercent,
    lowBalanceCount,
    financeMetrics,
    flowRows,
    financeStatus,
    headline,
    headlineDetail,
    primaryAction,
    signals,
    runwayText,
    runwayHint,
    expiryHint,
    balanceStatus,
    netCashReference
  };
}

export type TenantFinanceSummary = ReturnType<typeof useTenantFinanceSummary>;
