<script setup lang="ts">
import { computed } from "vue";
import { PortalContentCard } from "@dai/app-core";

import type { TenantAiUpstreamResource } from "../../../../types/aiTenant";
import { buildPricingCards } from "../presentation";

const props = defineProps<{
  resource: TenantAiUpstreamResource | null;
  creditsPerUsd: number;
  loading: boolean;
}>();

const cards = computed(() => buildPricingCards(props.resource, props.creditsPerUsd));
const panelDescription = computed(() => {
  if (!props.resource) return "选择上游资源后查看对应模型价格";
  if (!props.resource.price_book_name) return "当前资源尚未绑定结算价格表";
  return `结算价格表：${props.resource.price_book_name}`;
});
</script>

<template>
  <PortalContentCard class="pricing-card">
    <template #header>
      <div class="panel-copy">
        <span class="panel-title">可用模型与生效价格{{ props.resource ? ` · ${props.resource.name}` : "" }}</span>
        <span class="panel-description">{{ panelDescription }}</span>
      </div>
    </template>
    <template #actions>
      <span class="model-count">{{ cards.length }} 个模型</span>
      <span v-if="props.creditsPerUsd" class="rate-chip">
        汇率 1 USD = {{ props.creditsPerUsd }} 积分
      </span>
    </template>

    <div v-loading="props.loading" class="pricing-surface">
      <div v-if="cards.length" class="model-grid">
        <article
          v-for="card in cards"
          :key="card.key"
          class="model-card"
          :class="`model-card--${card.theme}`"
        >
          <header class="model-card__header">
            <h3 class="model-card__title">{{ card.modelCode }}</h3>
            <span class="capability-badge" :class="`capability-badge--${card.theme}`">
              {{ card.capabilityLabel }}
            </span>
          </header>

          <div class="model-card__body">
            <section v-for="section in card.sections" :key="`${card.key}-${section.key}`" class="pricing-panel">
              <span class="pricing-panel__title">{{ section.title }}</span>
              <div class="pricing-panel__lines">
                <div v-for="line in section.lines" :key="`${section.key}-${line.label}`" class="metric-row">
                  <span class="metric-row__label">{{ line.label }}</span>
                  <span class="metric-row__value-block">
                    <strong class="metric-row__value" :class="`metric-row__value--${line.tone}`">{{ line.usd }}</strong>
                    <span v-if="line.credits" class="metric-row__credits">{{ line.credits }}</span>
                  </span>
                </div>
              </div>
            </section>
          </div>
        </article>
      </div>

      <div v-else class="pricing-empty">
        <span>{{ props.loading ? "正在加载，请稍候..." : props.resource ? "该资源暂无已配置价格的可用模型" : "当前租户暂无可选上游资源" }}</span>
      </div>
    </div>
  </PortalContentCard>
</template>

<style scoped>
.panel-copy {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.panel-title {
  color: var(--ds-ink);
  font-weight: 700;
}

.panel-description {
  color: var(--ds-faint);
  font-size: 12px;
}

.model-count,
.rate-chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 26px;
  padding: 4px 9px;
  border-radius: var(--ds-radius-pill);
  background: var(--ds-accent-soft);
  font-size: 11px;
  font-weight: 700;
  white-space: nowrap;
}

.model-count {
  color: var(--ds-ink);
}

.rate-chip {
  border: 1px solid color-mix(in srgb, var(--ds-accent) 24%, transparent);
  color: var(--ds-accent);
  font-variant-numeric: tabular-nums;
}

/* 卡片随 grid 行拉伸,内部 flex 列让定价网格区吃掉剩余高度 */
.pricing-card {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.pricing-card :deep(.portal-content-card__body) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.pricing-surface {
  flex: 1;
  min-height: 120px;
  display: flex;
  flex-direction: column;
}

.model-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.model-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
  padding: 12px;
  border: 1px solid color-mix(in srgb, var(--ds-info) 20%, transparent);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.model-card--image {
  border-color: color-mix(in srgb, var(--ds-warning) 30%, transparent);
}

.model-card--video {
  border-color: color-mix(in srgb, var(--ds-danger) 30%, transparent);
}

.model-card--audio {
  border-color: color-mix(in srgb, var(--ds-accent) 30%, transparent);
}

.model-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.model-card__title {
  min-width: 0;
  margin: 0;
  overflow-wrap: anywhere;
  color: var(--ds-ink);
  font-size: 14px;
  font-weight: 700;
  line-height: 1.3;
}

.capability-badge {
  flex: 0 0 auto;
  padding: 4px 8px;
  border-radius: var(--ds-radius-pill);
  font-size: 10px;
  font-weight: 800;
  white-space: nowrap;
}

.capability-badge--token {
  color: var(--ds-info);
  background: var(--ds-info-soft);
}

.capability-badge--image {
  color: var(--ds-warning);
  background: var(--ds-warning-soft);
}

.capability-badge--video {
  color: var(--ds-danger);
  background: var(--ds-danger-soft);
}

.capability-badge--audio {
  color: var(--ds-accent);
  background: var(--ds-accent-soft);
}

.model-card__body {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 8px;
}

.pricing-panel {
  display: flex;
  flex-direction: column;
  gap: 7px;
  min-width: 0;
  padding: 9px 10px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
}

.pricing-panel__title {
  color: var(--ds-ink-soft);
  font-size: 10px;
  font-weight: 800;
}

.pricing-panel__lines {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.metric-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
}

.metric-row__label {
  color: var(--ds-muted);
  font-size: 11px;
  white-space: nowrap;
}

.metric-row__value {
  min-width: 0;
  color: var(--ds-ink);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
  text-align: right;
  overflow-wrap: anywhere;
}

.metric-row__value-block {
  display: inline-flex;
  min-width: 0;
  align-items: baseline;
  justify-content: flex-end;
  gap: 6px;
  text-align: right;
}

.metric-row__credits {
  color: var(--ds-faint);
  font-size: 10px;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.metric-row__value--input {
  color: var(--ds-info);
}

.metric-row__value--output {
  color: var(--ds-warning);
}

.metric-row__value--cache,
.metric-row__value--default {
  color: var(--ds-positive);
}

.metric-row__value--resolution {
  color: var(--ds-accent);
}

.metric-row__value--audio {
  color: var(--ds-accent);
}

.pricing-empty {
  display: flex;
  flex: 1;
  min-height: 160px;
  align-items: center;
  justify-content: center;
  border: 1px dashed var(--ds-line-strong);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel-muted);
  color: var(--ds-faint);
  font-size: 12px;
}

@media (max-width: 1360px) {
  .model-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .model-grid {
    grid-template-columns: 1fr;
  }
}
</style>
