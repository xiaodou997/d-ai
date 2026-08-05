<!--
  数据大盘指标区:可用余额/到账金额/用户消费/活跃用户四张指标卡 + 可用积分条。
  颜色全部走 var(--ds-*) token;指标卡复用 TenantWorkbenchMetricCard(图标+色调语义,
  超出 DsMetricCard 的 label/value/hint 能力,故不换),数据与 props 不变。
-->
<script setup lang="ts">
import { computed } from "vue";
import {
  BadgeDollarSign,
  Landmark,
  ShoppingBag,
  UsersRound,
  WalletCards
} from "lucide-vue-next";

import TenantWorkbenchMetricCard from "../../../../components/workbench/TenantWorkbenchMetricCard.vue";
import type { TenantCashAccount } from "../../../../types/tenant";
import type { AccountBalance, TenantAnalyticsOverview } from "../../../../types/urmTenant";

const props = defineProps<{
  cashAccount: TenantCashAccount;
  serviceBalance: AccountBalance;
  overview: TenantAnalyticsOverview;
  rangeLabel: string;
  loading: boolean;
  serviceBalanceLoading: boolean;
}>();

const emit = defineEmits<{
  openSettlement: [];
  topUpServiceBalance: [];
}>();

const currencyFormatter = new Intl.NumberFormat("zh-CN", {
  style: "currency",
  currency: "CNY",
  minimumFractionDigits: 2,
  maximumFractionDigits: 2
});

const numberFormatter = new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 2 });

const metrics = computed(() => [
  {
    key: "cash-balance",
    label: "可用余额",
    value: formatCents(props.cashAccount.available),
    hint: `用户充值到账，提现中 ${formatCents(props.cashAccount.frozen)}`,
    icon: WalletCards,
    tone: "emerald" as const
  },
  {
    key: "settlement-income",
    label: `${props.rangeLabel}到账金额`,
    value: formatCents(props.overview.settlementIncomeCents),
    hint: "用户在线充值扣除手续费后的实际入账",
    icon: BadgeDollarSign,
    tone: "blue" as const
  },
  {
    key: "user-consumption",
    label: `${props.rangeLabel}用户消费积分`,
    value: numberFormatter.format(props.overview.userDeductionCredits ?? 0),
    hint: "按成功消费记录汇总，不等同于现金收入",
    icon: ShoppingBag,
    tone: "amber" as const
  },
  {
    key: "active-users",
    label: "活跃消费用户",
    value: numberFormatter.format(props.overview.activeUserCount ?? 0),
    hint: `${numberFormatter.format(props.overview.userConsumptionCount ?? 0)} 笔成功消费`,
    icon: UsersRound,
    tone: "indigo" as const
  }
]);

const serviceBalanceValue = computed(() =>
  props.serviceBalanceLoading ? "—" : numberFormatter.format(props.serviceBalance.availableCredits ?? 0)
);

function formatCents(cents: number) {
  return currencyFormatter.format(Number(cents ?? 0) / 100);
}
</script>

<template>
  <section class="business-summary" aria-labelledby="business-summary-title">
    <div class="business-summary__head">
      <div>
        <p class="business-summary__eyebrow">经营概览</p>
        <h2 id="business-summary-title" class="business-summary__title">余额与用户使用</h2>
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
        :loading="loading"
      />
    </div>

    <div class="service-balance" :aria-busy="serviceBalanceLoading">
      <div class="service-balance__icon" aria-hidden="true">
        <Landmark :size="18" :stroke-width="1.9" />
      </div>
      <div class="service-balance__copy">
        <span class="service-balance__label">可用积分</span>
        <span class="service-balance__hint">使用平台服务时会扣除</span>
      </div>
      <strong class="service-balance__value">{{ serviceBalanceValue }} 积分</strong>
      <button class="service-balance__action" type="button" @click="emit('topUpServiceBalance')">
        购买积分
      </button>
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
  grid-template-columns: 34px minmax(0, 1fr) auto auto;
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

.service-balance__value {
  color: var(--ds-ink-soft);
  font-size: 13px;
  font-weight: 750;
  letter-spacing: 0;
  white-space: nowrap;
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

  .service-balance {
    grid-template-columns: 34px minmax(0, 1fr) auto;
  }

  .service-balance__value {
    grid-column: 2 / 3;
    white-space: normal;
  }

  .service-balance__action {
    grid-column: 3;
    grid-row: 1 / span 2;
  }
}
</style>
