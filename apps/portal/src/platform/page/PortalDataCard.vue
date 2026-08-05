<script setup lang="ts">
import { computed, useSlots } from "vue";

import { DsTag } from "@/shared/ui";

import PortalContentCard from "./PortalContentCard.vue";

const slots = useSlots();

const props = withDefaults(
  defineProps<{
    title?: string;
    description?: string;
    total?: number;
    totalLabel?: string;
    meta?: string;
  }>(),
  {
    totalLabel: "条"
  }
);

const hasFilters = computed(() => Boolean(slots.filters));
const hasPagination = computed(() => Boolean(slots.pagination));
const metaText = computed(() => {
  if (props.meta) return props.meta;
  if (typeof props.total === "number") return `共 ${props.total} ${props.totalLabel}`;
  return "";
});
// 只有标题/描述/操作/自定义 meta 存在时才渲染卡头;仅有 total 统计不渲染头部条带
// (分页栏已展示"共 N 条",避免在表格上方多出一条空白带)
const hasHeaderContext = computed(
  () => Boolean(props.title) || Boolean(props.description) || Boolean(slots.actions) || Boolean(slots.meta)
);
const hasMeta = computed(() => hasHeaderContext.value && (Boolean(slots.meta) || Boolean(metaText.value)));
const hasActions = computed(() => Boolean(slots.actions));
</script>

<template>
  <PortalContentCard :title="title" :description="description" body-padding="none">
    <template v-if="hasMeta" #meta>
      <slot name="meta">
        <DsTag v-if="metaText" tone="neutral">{{ metaText }}</DsTag>
      </slot>
    </template>

    <template v-if="hasActions" #actions>
      <slot name="actions" />
    </template>

    <div v-if="hasFilters" class="portal-data-card__filters">
      <slot name="filters" />
    </div>

    <div class="portal-data-card__table">
      <slot />
    </div>

    <template v-if="hasPagination" #footer>
      <div class="portal-data-card__pagination">
        <slot name="pagination" />
      </div>
    </template>
  </PortalContentCard>
</template>

<style scoped>
.portal-data-card__filters {
  padding: 14px 20px;
  /* 整卡白底,只用分隔线区分区域,不用灰色块 */
  border-bottom: 1px solid var(--ds-line);
  background: var(--ds-panel);
}

.portal-data-card__table {
  min-width: 0;
}

.portal-data-card__pagination {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  padding: 12px 20px;
  background: var(--ds-panel);
}

@media (max-width: 768px) {
  .portal-data-card__filters,
  .portal-data-card__pagination {
    padding-inline: 16px;
  }

  .portal-data-card__pagination {
    justify-content: flex-start;
  }
}
</style>
