<script setup lang="ts">
import { useRouter } from "vue-router";
import { Activity, ArrowRight, CircleAlert, ReceiptText } from "lucide-vue-next";
import type { TenantFinanceSummary } from "../composables/useTenantFinanceSummary";

const props = defineProps<{
  finance: Pick<TenantFinanceSummary, "signals">;
}>();
const { signals } = props.finance;

const router = useRouter();

function goTo(path: string) {
  void router.push(path);
}
</script>

<template>
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
</template>

<style scoped>
.signal-item__action,
.signals-card__mini-links button {
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

.signal-item__action:hover,
.signals-card__mini-links button:hover {
  color: var(--ds-accent);
  text-decoration: underline;
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

@media (max-width: 640px) {

  .signal-list {
    padding-inline: 16px;
  }

  .signals-card__mini-links {
    margin-inline: 16px;
    padding-inline: 0;
  }

  .signals-card__mini-links {
    padding-bottom: 16px;
  }
}
</style>
