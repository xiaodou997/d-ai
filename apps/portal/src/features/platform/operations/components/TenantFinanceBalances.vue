<script setup lang="ts">
import { useRouter } from "vue-router";
import { ArrowRight, CreditCard, TrendingUp, WalletCards } from "lucide-vue-next";
import { DsButton, DsTag } from "@/shared/ui";
import type { TenantOperationsDashboard } from "../composables/useTenantOperationsDashboard";
import type { TenantFinanceSummary } from "../composables/useTenantFinanceSummary";
import { formatUSD, formatMicroUSD, signedMoney } from "../financePresentation";

const props = defineProps<{
  dashboard: Pick<TenantOperationsDashboard, "serviceBalanceLoading" | "summaryLoading">;
  finance: Pick<
    TenantFinanceSummary,
    "balanceStatus"
    | "balance"
    | "runwayText"
    | "runwayHint"
    | "balanceRemainingPercent"
    | "expiryHint"
    | "rangeLabel"
    | "flowRows"
    | "netCashReference"
  >;
}>();
const dashboard = props.dashboard;
const { balanceStatus, balance, runwayText, runwayHint, balanceRemainingPercent, expiryHint, rangeLabel, flowRows, netCashReference } = props.finance;

const router = useRouter();

function goTo(path: string) {
  void router.push(path);
}
</script>

<template>
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
</template>

<style scoped>
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

@media (max-width: 640px) {

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

  .flow-list {
    padding-inline: 16px;
  }

  .flow-card__summary {
    margin-inline: 16px;
    padding-inline: 0;
  }

  .flow-card__summary {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
