<!--
  数据大盘「用户消费贡献榜」面板:按成功消费积分排序的横向条形榜。
  颜色全部走 var(--ds-*) token(含骨架渐变,与 DsTable .ds-table__skeleton 同组 token);
  数据与 props 不变。
-->
<script setup lang="ts">
import { computed } from "vue";
import { Trophy, UsersRound } from "lucide-vue-next";

import type { UserConsumptionItem } from "@/api/types/urmTenant";

const props = defineProps<{
  items: readonly UserConsumptionItem[];
  rangeLabel: string;
  loading: boolean;
}>();

const emit = defineEmits<{
  openDetails: [];
}>();

const numberFormatter = new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 2 });
const maxCredits = computed(() => Math.max(...props.items.map((item) => item.credits), 0));

function barWidth(item: UserConsumptionItem) {
  if (maxCredits.value <= 0) return "0%";
  return `${Math.max((item.credits / maxCredits.value) * 100, 4)}%`;
}
</script>

<template>
  <section class="ranking-panel" aria-labelledby="consumption-ranking-title">
    <header class="ranking-panel__head">
      <div class="ranking-panel__heading">
        <span class="ranking-panel__icon" aria-hidden="true"><Trophy :size="19" /></span>
        <div>
          <h2 id="consumption-ranking-title" class="ranking-panel__title">用户消费贡献榜</h2>
          <p class="ranking-panel__desc">{{ rangeLabel }}按成功消费积分排序</p>
        </div>
      </div>
      <button class="ranking-panel__action" type="button" @click="emit('openDetails')">查看消费明细</button>
    </header>

    <div v-if="loading" class="ranking-panel__loading" aria-label="正在加载用户消费排行">
      <span v-for="index in 5" :key="index" class="ranking-panel__skeleton"></span>
    </div>

    <div v-else-if="items.length === 0" class="ranking-panel__empty">
      <UsersRound :size="38" :stroke-width="1.5" />
      <p>当前时间范围内暂无用户消费</p>
    </div>

    <ol v-else class="ranking-list">
      <li v-for="(item, index) in items" :key="item.userId" class="ranking-row">
        <span class="ranking-row__rank" :class="{ 'is-leading': index < 3 }">{{ index + 1 }}</span>
        <div class="ranking-row__identity">
          <strong class="ranking-row__name">{{ item.username || "未知用户" }}</strong>
          <span class="ranking-row__meta">{{ numberFormatter.format(item.transactionCount) }} 笔消费</span>
        </div>
        <div class="ranking-row__bar" aria-hidden="true">
          <span class="ranking-row__bar-fill" :style="{ width: barWidth(item) }"></span>
        </div>
        <div class="ranking-row__result">
          <strong>{{ numberFormatter.format(item.credits) }}</strong>
          <span>{{ item.percentage || "0.0" }}%</span>
        </div>
      </li>
    </ol>
  </section>
</template>

<style scoped>
.ranking-panel {
  min-width: 0;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.ranking-panel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid var(--ds-line);
  padding: 18px 20px;
}

.ranking-panel__heading {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 11px;
}

.ranking-panel__icon {
  display: grid;
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
  place-items: center;
  border-radius: 8px;
  background: var(--ds-warning-soft);
  color: var(--ds-warning);
}

.ranking-panel__title {
  margin: 0;
  color: var(--ds-ink);
  font-size: 15px;
  font-weight: 750;
  letter-spacing: 0;
}

.ranking-panel__desc {
  margin: 3px 0 0;
  color: var(--ds-faint);
  font-size: 12px;
}

.ranking-panel__action {
  border: 0;
  background: transparent;
  color: var(--ds-accent-hover);
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
  cursor: pointer;
}

.ranking-panel__loading {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 22px 20px;
}

.ranking-panel__skeleton {
  height: 36px;
  border-radius: 6px;
  /* 骨架渐变与 DsTable .ds-table__skeleton 保持同一组 token */
  background: linear-gradient(
    90deg,
    var(--ds-panel-muted) 25%,
    var(--ds-line) 50%,
    var(--ds-panel-muted) 75%
  );
  background-size: 200% 100%;
  animation: ranking-loading 1.3s ease-in-out infinite;
}

.ranking-panel__empty {
  display: flex;
  min-height: 330px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--ds-faint);
}

.ranking-panel__empty p {
  margin: 10px 0 0;
  font-size: 13px;
}

.ranking-list {
  display: flex;
  min-height: 330px;
  flex-direction: column;
  justify-content: center;
  gap: 4px;
  margin: 0;
  padding: 12px 20px 18px;
  list-style: none;
}

.ranking-row {
  display: grid;
  grid-template-columns: 28px minmax(112px, 0.75fr) minmax(100px, 1.3fr) minmax(82px, auto);
  align-items: center;
  gap: 12px;
  min-height: 44px;
  border-bottom: 1px solid var(--ds-line);
}

.ranking-row:last-child {
  border-bottom: 0;
}

.ranking-row__rank {
  display: grid;
  width: 24px;
  height: 24px;
  place-items: center;
  border-radius: 50%;
  background: var(--ds-panel-muted);
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 800;
}

.ranking-row__rank.is-leading {
  background: var(--ds-warning-soft);
  color: var(--ds-warning);
}

.ranking-row__identity,
.ranking-row__result {
  display: flex;
  min-width: 0;
  flex-direction: column;
}

.ranking-row__name {
  overflow: hidden;
  color: var(--ds-ink-soft);
  font-size: 13px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ranking-row__meta,
.ranking-row__result span {
  color: var(--ds-faint);
  font-size: 11px;
}

.ranking-row__bar {
  height: 7px;
  overflow: hidden;
  border-radius: 4px;
  background: var(--ds-panel-muted);
}

.ranking-row__bar-fill {
  display: block;
  height: 100%;
  border-radius: 4px;
  background: var(--ds-positive);
}

.ranking-row__result {
  align-items: end;
}

.ranking-row__result strong {
  color: var(--ds-positive);
  font-size: 13px;
  font-weight: 800;
}

@keyframes ranking-loading {
  from { background-position: 200% 0; }
  to { background-position: -200% 0; }
}

@media (max-width: 640px) {
  .ranking-panel__head {
    align-items: start;
    padding: 16px;
  }

  .ranking-list {
    padding-inline: 16px;
  }

  .ranking-row {
    grid-template-columns: 26px minmax(0, 1fr) auto;
    grid-template-rows: auto 7px;
    gap: 9px;
    padding-block: 8px;
  }

  .ranking-row__rank {
    grid-column: 1;
    grid-row: 1;
  }

  .ranking-row__identity {
    grid-column: 2;
    grid-row: 1;
  }

  .ranking-row__result {
    grid-column: 3;
    grid-row: 1;
  }

  .ranking-row__bar {
    grid-column: 2 / 4;
    grid-row: 2;
  }
}
</style>
