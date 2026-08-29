<script setup lang="ts">
import { computed } from "vue";

import { DsMetricCard } from "@/shared/ui";
import type { TenantUsageStats } from "@/features/ai/usage";
import type { EndUserItem } from "@/api/types/platformTenant";
import { formatDisplayUSD } from "@/shared/currency";
import { formatDateTime, formatNumber } from "../formatters";
import type { UserOverviewGroupSummary, UserOverviewRiskSignal } from "../model";

interface MetricCard {
  key: string;
  label: string;
  value: string;
  meta: string;
}

const props = defineProps<{
  user: EndUserItem | null;
  rechargeTotal: number;
  latestRechargeTime: number | null;
  aiUsageStats: TenantUsageStats;
  groupSummary: UserOverviewGroupSummary;
  riskSignals: UserOverviewRiskSignal[];
  activityWindowLabel: string;
  aiAvailable: boolean;
}>();

const cards = computed<MetricCard[]>(() => {
  const activeRisks = props.riskSignals.filter((item) => item.tone === "warning" || item.tone === "danger");
  return [
    {
      key: "balance",
      label: "当前 USD 余额",
      value: formatDisplayUSD(props.user?.balanceUsd),
      meta: props.user?.status === 1 ? "账户可用余额" : "账号停用中"
    },
    {
      key: "recharge",
      label: "充值记录",
      value: formatNumber(props.rechargeTotal),
      meta: props.latestRechargeTime ? `最近充值 ${formatDateTime(props.latestRechargeTime)}` : "暂无充值记录"
    },
    {
      key: "ai",
      label: "AI 请求",
      value: props.aiAvailable ? formatNumber(props.aiUsageStats.total_requests) : "—",
      meta: props.aiAvailable
        ? `${props.activityWindowLabel}，失败 ${formatNumber(props.aiUsageStats.failed_count)}，消耗 $${Number(props.aiUsageStats.total_user_charged_usd ?? 0).toLocaleString("en-US", { maximumFractionDigits: 6 })}`
        : "当前租户未开通智能服务"
    },
    {
      key: "groups",
      label: "AI 可见分组",
      value: props.aiAvailable ? formatNumber(props.groupSummary.accessible) : "—",
      meta: props.aiAvailable
        ? `默认开放 ${formatNumber(props.groupSummary.defaultVisible)}，单独配置 ${formatNumber(props.groupSummary.customBindings)}`
        : "当前租户未开通智能服务"
    },
    {
      key: "risk",
      label: "风险信号",
      value: formatNumber(activeRisks.length),
      meta: activeRisks.length
        ? activeRisks.map((item) => `${item.title} ${item.value}`).slice(0, 2).join(" · ")
        : "当前未发现明显异常"
    }
  ];
});
</script>

<template>
  <section class="metrics-grid">
    <DsMetricCard v-for="card in cards" :key="card.key" :label="card.label" :value="card.value" :hint="card.meta" />
  </section>
</template>

<style scoped>
.metrics-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 14px;
}

@media (max-width: 1440px) {
  .metrics-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 768px) {
  .metrics-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .metrics-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
