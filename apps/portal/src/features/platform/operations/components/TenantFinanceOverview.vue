<script setup lang="ts">
import { useRouter } from "vue-router";
import { ArrowRight } from "lucide-vue-next";
import { DsButton, DsTag } from "@/shared/ui";
import type { TenantOperationsDashboard } from "../composables/useTenantOperationsDashboard";
import type { TenantFinanceSummary } from "../composables/useTenantFinanceSummary";
import { formatUSD, formatPercent, formatCount, formatCompactTokens } from "../financePresentation";

const props = defineProps<{
  dashboard: Pick<TenantOperationsDashboard, "summaryLoading">;
  finance: Pick<
    TenantFinanceSummary,
    "financeStatus"
    | "headline"
    | "headlineDetail"
    | "userRevenue"
    | "platformCost"
    | "totalRequests"
    | "totalTokens"
    | "errorRate"
    | "primaryAction"
    | "rangeLabel"
    | "financeMetrics"
  >;
}>();
const dashboard = props.dashboard;
const {
  financeStatus,
  headline,
  headlineDetail,
  userRevenue,
  platformCost,
  totalRequests,
  totalTokens,
  errorRate,
  primaryAction,
  rangeLabel,
  financeMetrics
} = props.finance;

const router = useRouter();

function goTo(path: string) {
  void router.push(path);
}
</script>

<template>
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
          <span>明确错误率</span>
          <strong>{{ dashboard.summaryLoading.value ? "—" : formatPercent(errorRate) }}</strong>
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
</template>

<style scoped>
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

.financial-metric__icon {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  flex: 0 0 34px;
  border-radius: var(--ds-radius-control);
  background: var(--ds-accent-soft);
  color: var(--ds-accent);
}

.financial-metric--positive .financial-metric__icon {
  background: var(--ds-positive-soft);
  color: var(--ds-positive);
}

.financial-metric--warning .financial-metric__icon {
  background: var(--ds-warning-soft);
  color: var(--ds-warning);
}

.financial-metric--danger .financial-metric__icon {
  background: var(--ds-danger-soft);
  color: var(--ds-danger);
}

.financial-metric__badge {
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

@media (max-width: 1240px) {
  .financial-metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
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
}

@media (max-width: 640px) {

  .finance-hero__copy {
    padding: 22px 20px 20px;
  }

  .finance-hero__title {
    font-size: 25px;
  }

  .financial-metrics {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
