<!--
  工作台指标卡:label + 大数值 + hint,带 lucide 图标徽章与顶部色调条。
  色调(emerald/indigo/amber/sky/rose 等)全部映射到 var(--ds-*) 语义 token;
  消费方:租户运营数据大盘。
-->
<script setup lang="ts">
import { computed, type Component } from "vue";

const props = withDefaults(
  defineProps<{
    label: string;
    value: string | number;
    hint?: string;
    loading?: boolean;
    icon?: Component;
    tone?:
      | "primary"
      | "emerald"
      | "indigo"
      | "amber"
      | "sky"
      | "rose"
      | "blue"
      | "green"
      | "purple"
      | "orange";
  }>(),
  {
    hint: "",
    loading: false,
    tone: "primary"
  }
);

const displayValue = computed(() => (props.loading ? "—" : String(props.value)));
const toneClass = computed(() => `metric-card--${props.tone}`);
</script>

<template>
  <article class="metric-card" :class="toneClass" :aria-busy="loading">
    <div class="metric-card__head">
      <div v-if="icon" class="metric-card__icon" aria-hidden="true">
        <component :is="icon" :size="19" :stroke-width="1.9" />
      </div>
      <span class="metric-card__label">{{ label }}</span>
    </div>
    <strong class="metric-card__value">{{ displayValue }}</strong>
    <p v-if="hint" class="metric-card__hint">{{ hint }}</p>
  </article>
</template>

<style scoped>
.metric-card {
  --metric-accent: var(--ds-accent);
  --metric-soft: var(--ds-accent-soft);
  min-width: 0;
  min-height: 148px;
  border: 1px solid var(--ds-line);
  border-top: 3px solid var(--metric-accent);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  padding: 18px;
  box-shadow: var(--ds-shadow-sm);
}

.metric-card__head {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.metric-card__icon {
  display: grid;
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
  place-items: center;
  border-radius: 8px;
  background: var(--metric-soft);
  color: var(--metric-accent);
}

.metric-card__label {
  min-width: 0;
  color: var(--ds-muted);
  font-size: 13px;
  font-weight: 650;
  line-height: 1.35;
}

.metric-card__value {
  display: block;
  overflow-wrap: anywhere;
  margin-top: 16px;
  color: var(--ds-ink);
  font-size: 26px;
  font-weight: 750;
  letter-spacing: 0;
  line-height: 1.05;
}

.metric-card__hint {
  margin: 8px 0 0;
  color: var(--ds-faint);
  font-size: 12px;
  font-weight: 500;
  line-height: 1.45;
}

/* 色调只映射到 ds 语义 token,不引入 token 之外的色值 */
.metric-card--emerald,
.metric-card--green {
  --metric-accent: var(--ds-positive);
  --metric-soft: var(--ds-positive-soft);
}

.metric-card--indigo,
.metric-card--purple {
  --metric-accent: var(--ds-accent);
  --metric-soft: var(--ds-accent-soft);
}

.metric-card--amber,
.metric-card--orange {
  --metric-accent: var(--ds-warning);
  --metric-soft: var(--ds-warning-soft);
}

.metric-card--sky,
.metric-card--blue {
  --metric-accent: var(--ds-info);
  --metric-soft: var(--ds-info-soft);
}

.metric-card--rose {
  --metric-accent: var(--ds-danger);
  --metric-soft: var(--ds-danger-soft);
}

@media (max-width: 640px) {
  .metric-card {
    min-height: 132px;
  }

  .metric-card__value {
    font-size: 23px;
  }
}
</style>
