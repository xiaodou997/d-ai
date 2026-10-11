<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import { Activity, ArrowUpRight, ReceiptText } from "lucide-vue-next";
import type { TenantOperationsDashboard } from "../composables/useTenantOperationsDashboard";
import { sourceLabel, formatUSD, formatCount, formatCompactTokens } from "../financePresentation";

const props = defineProps<{
  dashboard: Pick<TenantOperationsDashboard, "appConsumption" | "structureLoading" | "topModels">;
}>();
const dashboard = props.dashboard;

const router = useRouter();

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

function goTo(path: string) {
  void router.push(path);
}
</script>

<template>
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
</template>

<style scoped>
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

@media (max-width: 640px) {

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
}
</style>
