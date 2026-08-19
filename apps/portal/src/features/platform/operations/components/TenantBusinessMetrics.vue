<!--
  财务概览指标区:可用余额/到账金额/用户消费/AI 服务成本四张指标卡 + 账户操作入口。
  颜色全部走 var(--ds-*) token;指标卡复用 TenantWorkbenchMetricCard(图标+色调语义,
  超出 DsMetricCard 的 label/value/hint 能力,故不换)。
-->
<script setup lang="ts">
import { computed } from "vue";
import {
  BadgeDollarSign,
  Landmark,
  ReceiptText,
  ShoppingBag,
  WalletCards
} from "lucide-vue-next";

import TenantWorkbenchMetricCard from "@/components/workbench/TenantWorkbenchMetricCard.vue";
import type { AccountBalance, TenantAnalyticsOverview } from "@/api/types/platformTenant";
import type { TenantAiDashboardSummary } from "@/api/types/aiTenant";
import { formatDisplayMicroUSD as formatMicroUSD, formatDisplayUSD as formatUSD } from "@/shared/currency";

const props = defineProps<{
  serviceBalance: AccountBalance;
  overview: TenantAnalyticsOverview;
  financialSummary: TenantAiDashboardSummary | null;
  rangeLabel: string;
  loading: boolean;
  serviceBalanceLoading: boolean;
}>();

const emit = defineEmits<{
  openSettlement: [];
  topUpServiceBalance: [];
}>();

const metrics = computed(() => [
  {
    key: "cash-balance",
    label: "可用余额",
    value: formatUSD(props.serviceBalance.availableUsd),
    hint: "统一额度账户，支持透支",
    icon: WalletCards,
    tone: "emerald" as const,
    loading: props.serviceBalanceLoading
  },
  {
    key: "settlement-income",
    label: `${props.rangeLabel}到账金额`,
    value: formatMicroUSD(props.overview.settlementIncomeMicroUsd),
    hint: "用户在线充值扣除手续费后的实际入账",
    icon: BadgeDollarSign,
    tone: "blue" as const,
    loading: props.loading
  },
  {
    key: "user-consumption",
    label: `${props.rangeLabel}用户消费`,
    value: formatUSD(props.overview.userDeductionUsd),
    hint: "按成功 AI 请求使用记录汇总",
    icon: ShoppingBag,
    tone: "amber" as const,
    loading: props.loading
  },
  {
    key: "ai-service-cost",
    label: `${props.rangeLabel} AI 服务成本`,
    value: props.financialSummary ? formatUSD(props.financialSummary.total_tenant_payable_usd) : "—",
    hint: props.financialSummary ? "平台按实际 AI 用量向租户结算" : "AI 结算数据暂不可用",
    icon: ReceiptText,
    tone: "indigo" as const,
    loading: props.loading
  }
]);

</script>

<template>
  <section class="business-summary" aria-labelledby="business-summary-title">
    <div class="business-summary__head">
      <div>
        <p class="business-summary__eyebrow">财务概览</p>
        <h2 id="business-summary-title" class="business-summary__title">资金与结算</h2>
      </div>
      <button class="business-summary__cash-link" type="button" @click="emit('openSettlement')">
        查看余额
      </button>
    </div>

    <div class="business-summary__metrics">
      <TenantWorkbenchMetricCard
        v-for="metric in metrics"
        :key="metric.key"
        :label="metric.label"
        :value="metric.value"
        :hint="metric.hint"
        :icon="metric.icon"
        :tone="metric.tone"
        :loading="metric.loading"
      />
    </div>

    <div class="service-balance" :aria-busy="serviceBalanceLoading">
      <div class="service-balance__icon" aria-hidden="true">
        <Landmark :size="18" :stroke-width="1.9" />
      </div>
      <div class="service-balance__copy">
        <span class="service-balance__label">统一额度账户</span>
        <span class="service-balance__hint">集中管理 AI 服务消费、账户充值与余额明细</span>
      </div>
      <div class="service-balance__actions">
        <button class="service-balance__action" type="button" @click="emit('openSettlement')">账户明细</button>
        <button class="service-balance__action" type="button" @click="emit('topUpServiceBalance')">充值</button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.business-summary {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.business-summary__head {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 16px;
}

.business-summary__eyebrow {
  margin: 0 0 4px;
  color: var(--ds-positive);
  font-size: 11px;
  font-weight: 750;
  letter-spacing: 0.12em;
}

.business-summary__title {
  margin: 0;
  color: var(--ds-ink);
  font-size: 18px;
  font-weight: 750;
  letter-spacing: 0;
}

.business-summary__cash-link,
.service-balance__action {
  border: 0;
  background: transparent;
  color: var(--ds-accent-hover);
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
}

.business-summary__cash-link:hover,
.service-balance__action:hover {
  text-decoration: underline;
}

.business-summary__metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

.service-balance {
  display: grid;
  grid-template-columns: 34px minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  border: 1px solid var(--ds-line);
  border-radius: 8px;
  background: var(--ds-panel-muted);
  padding: 11px 14px;
}

.service-balance__icon {
  display: grid;
  width: 34px;
  height: 34px;
  place-items: center;
  border-radius: 8px;
  background: var(--ds-panel-muted);
  color: var(--ds-muted);
}

.service-balance__copy {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
}

.service-balance__label {
  color: var(--ds-ink-soft);
  font-size: 12px;
  font-weight: 700;
}

.service-balance__hint {
  color: var(--ds-faint);
  font-size: 11px;
}

.service-balance__actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

@media (max-width: 1100px) {
  .business-summary__metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .business-summary__head {
    align-items: start;
  }

  .business-summary__metrics {
    grid-template-columns: minmax(0, 1fr);
  }

  .service-balance__actions {
    grid-column: 2 / -1;
    justify-content: flex-end;
  }
}
</style>
