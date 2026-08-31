<script setup lang="ts">
import { computed } from "vue";

import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from "lucide-vue-next";

const props = withDefaults(
  defineProps<{
    page: number;
    pageSize: number;
    total: number;
    pageSizes?: number[];
  }>(),
  {
    pageSizes: () => [10, 20, 50, 100]
  }
);

const emit = defineEmits<{
  "update:page": [value: number];
  "update:pageSize": [value: number];
}>();

const totalPages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)));

const pages = computed(() => {
  const tp = totalPages.value;
  const current = props.page;
  const delta = 2;
  const range: (number | string)[] = [];

  for (let i = 1; i <= tp; i++) {
    if (i === 1 || i === tp || (i >= current - delta && i <= current + delta)) {
      range.push(i);
    } else if (range[range.length - 1] !== "…") {
      range.push("…");
    }
  }
  return range;
});

function goTo(page: number) {
  if (page >= 1 && page <= totalPages.value && page !== props.page) {
    emit("update:page", page);
  }
}

function changePageSize(size: number) {
  if (size !== props.pageSize) {
    emit("update:pageSize", size);
    emit("update:page", 1);
  }
}
</script>

<template>
  <div class="ds-pagination">
    <span class="ds-pagination__info" aria-live="polite">共 {{ total }} 条</span>

    <div class="ds-pagination__controls">
      <select
        class="ds-pagination__select"
        :value="pageSize"
        aria-label="每页条数"
        @change="changePageSize(Number(($event.target as HTMLSelectElement).value))"
      >
        <option v-for="size in pageSizes" :key="size" :value="size">{{ size }} 条/页</option>
      </select>

      <nav class="ds-pagination__nav" aria-label="分页">
        <button
          type="button"
          class="ds-pagination__btn ds-pagination__btn--icon"
          :disabled="page <= 1"
          aria-label="第一页"
          title="第一页"
          @click="goTo(1)"
        >
          <ChevronsLeft :size="14" />
        </button>
        <button
          type="button"
          class="ds-pagination__btn ds-pagination__btn--icon"
          :disabled="page <= 1"
          aria-label="上一页"
          title="上一页"
          @click="goTo(page - 1)"
        >
          <ChevronLeft :size="14" />
        </button>

        <template v-for="(p, idx) in pages" :key="idx">
          <span v-if="p === '…'" class="ds-pagination__ellipsis">…</span>
          <button
            v-else
            type="button"
            class="ds-pagination__btn"
            :class="{ 'ds-pagination__btn--active': p === page }"
            :aria-current="p === page ? 'page' : undefined"
            :aria-label="`第 ${p} 页`"
            @click="goTo(p as number)"
          >
            {{ p }}
          </button>
        </template>

        <button
          type="button"
          class="ds-pagination__btn ds-pagination__btn--icon"
          :disabled="page >= totalPages"
          aria-label="下一页"
          title="下一页"
          @click="goTo(page + 1)"
        >
          <ChevronRight :size="14" />
        </button>
        <button
          type="button"
          class="ds-pagination__btn ds-pagination__btn--icon"
          :disabled="page >= totalPages"
          aria-label="最后一页"
          title="最后一页"
          @click="goTo(totalPages)"
        >
          <ChevronsRight :size="14" />
        </button>
      </nav>
    </div>
  </div>
</template>

<style scoped>
.ds-pagination {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  width: 100%;
  min-width: 0;
  flex-wrap: wrap;
}

.ds-pagination__info {
  color: var(--ds-muted);
  font-size: 12.5px;
  font-weight: 500;
  white-space: nowrap;
}

.ds-pagination__controls {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  max-width: 100%;
  justify-content: flex-end;
  flex-wrap: wrap;
}

.ds-pagination__select {
  height: var(--ds-control-height-icon);
  border: 1px solid var(--ds-line-strong);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel);
  color: var(--ds-ink-soft);
  padding: 0 8px;
  font-size: 12.5px;
  cursor: pointer;
}

.ds-pagination__select:focus {
  outline: none;
  border-color: var(--ds-accent);
  box-shadow: var(--ds-shadow-focus);
}

.ds-pagination__select:focus-visible,
.ds-pagination__btn:focus-visible {
  outline: 2px solid var(--ds-accent);
  outline-offset: 2px;
}

.ds-pagination__nav {
  display: flex;
  align-items: center;
  gap: 2px;
  min-width: 0;
  max-width: 100%;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.ds-pagination__btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 30px;
  height: var(--ds-control-height-icon);
  border: 1px solid transparent;
  border-radius: var(--ds-radius-control);
  background: transparent;
  color: var(--ds-ink-soft);
  font-size: 12.5px;
  font-weight: 500;
  cursor: pointer;
  transition:
    background-color 120ms ease,
    color 120ms ease,
    border-color 120ms ease;
}

.ds-pagination__btn:hover:not(:disabled):not(.ds-pagination__btn--active) {
  background: var(--ds-panel-muted);
  color: var(--ds-ink);
}

.ds-pagination__btn--active {
  background: var(--ds-accent);
  color: var(--ds-accent-contrast);
  font-weight: 600;
}

.ds-pagination__btn--icon {
  padding: 0 4px;
}

.ds-pagination__btn:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}

.ds-pagination__ellipsis {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 30px;
  height: var(--ds-control-height-icon);
  color: var(--ds-muted);
  font-size: 12.5px;
  user-select: none;
}
</style>
