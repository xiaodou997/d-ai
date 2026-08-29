<script setup lang="ts">
import { computed, ref, watch, watchEffect } from "vue";

import { ChevronRight, Inbox } from "lucide-vue-next";

import type { DsTableColumn } from "./types";

type Row = Record<string, unknown>;

const props = withDefaults(
  defineProps<{
    columns: DsTableColumn[];
    // 开放为 any[]：cell 插槽的 row 需要可直接取字段，避免每个消费页面重复断言
    rows: any[];
    rowKey: string;
    loading?: boolean;
    emptyTitle?: string;
    emptyDescription?: string;
    selectable?: boolean;
    expandable?: boolean;
    selection?: unknown[];
    /** false 时去掉自身边框/圆角/背景，便于嵌入卡片内 */
    frame?: boolean;
  }>(),
  {
    loading: false,
    emptyTitle: "暂无数据",
    emptyDescription: undefined,
    selectable: false,
    expandable: false,
    selection: () => [],
    frame: true
  }
);

const emit = defineEmits<{
  "update:selection": [value: unknown[]];
}>();

const skeletonRows = Array.from({ length: 6 });

const totalColumns = computed(
  () => props.columns.length + (props.selectable ? 1 : 0) + (props.expandable ? 1 : 0)
);

function asRow(row: unknown): Row {
  return (row ?? {}) as Row;
}

function cellValue(row: unknown, key: string): unknown {
  return asRow(row)[key];
}

function displayValue(value: unknown): string {
  return value === null || value === undefined ? "" : String(value);
}

function rowKeyValue(row: unknown): string {
  return String(asRow(row)[props.rowKey]);
}

function columnWidth(width: string | number): string {
  return typeof width === "number" ? `${width}px` : width;
}

// --- selection ---

function isSelected(row: unknown): boolean {
  const key = rowKeyValue(row);
  return props.selection.some((item) => rowKeyValue(item) === key);
}

const allSelected = computed(
  () => props.rows.length > 0 && props.rows.every((row) => isSelected(row))
);

const someSelected = computed(() => props.rows.some((row) => isSelected(row)));

const selectAllRef = ref<HTMLInputElement | null>(null);

watchEffect(() => {
  if (selectAllRef.value) {
    selectAllRef.value.indeterminate = someSelected.value && !allSelected.value;
  }
});

function toggleRow(row: unknown) {
  if (isSelected(row)) {
    const key = rowKeyValue(row);
    emit(
      "update:selection",
      props.selection.filter((item) => rowKeyValue(item) !== key)
    );
  } else {
    emit("update:selection", [...props.selection, row]);
  }
}

function toggleAll() {
  if (allSelected.value) {
    const pageKeys = new Set(props.rows.map((row) => rowKeyValue(row)));
    emit(
      "update:selection",
      props.selection.filter((item) => !pageKeys.has(rowKeyValue(item)))
    );
  } else {
    const selectedKeys = new Set(props.selection.map((item) => rowKeyValue(item)));
    const missing = props.rows.filter((row) => !selectedKeys.has(rowKeyValue(row)));
    emit("update:selection", [...props.selection, ...missing]);
  }
}

// --- expand ---

const expandedKeys = ref(new Set<string>());

watch(
  () => props.rows,
  () => {
    expandedKeys.value = new Set();
  }
);

function isExpanded(row: unknown): boolean {
  return expandedKeys.value.has(rowKeyValue(row));
}

function toggleExpand(row: unknown) {
  const key = rowKeyValue(row);
  const next = new Set(expandedKeys.value);
  if (next.has(key)) {
    next.delete(key);
  } else {
    next.add(key);
  }
  expandedKeys.value = next;
}
</script>

<template>
  <div class="ds-table" :class="{ 'ds-table--frameless': !frame }">
    <table class="ds-table__table">
      <thead>
        <tr>
          <th v-if="selectable" class="ds-table__th ds-table__th--selection">
            <input
              ref="selectAllRef"
              type="checkbox"
              class="ds-table__checkbox"
              :checked="allSelected"
              title="全选当前页"
              @change="toggleAll"
            />
          </th>
          <th v-if="expandable" class="ds-table__th ds-table__th--expand" />
          <th
            v-for="col in columns"
            :key="col.key"
            class="ds-table__th"
            :class="`ds-table__cell--${col.align ?? 'left'}`"
            :style="col.width ? { width: columnWidth(col.width) } : undefined"
          >
            {{ col.title }}
          </th>
        </tr>
      </thead>

      <tbody v-if="loading">
        <tr v-for="(_, rowIndex) in skeletonRows" :key="rowIndex" class="ds-table__row ds-table__row--skeleton">
          <td v-if="selectable" class="ds-table__td"><span class="ds-table__skeleton ds-table__skeleton--box" /></td>
          <td v-if="expandable" class="ds-table__td"><span class="ds-table__skeleton ds-table__skeleton--box" /></td>
          <td v-for="col in columns" :key="col.key" class="ds-table__td">
            <span class="ds-table__skeleton" />
          </td>
        </tr>
      </tbody>

      <tbody v-else>
        <template v-for="(row, index) in rows" :key="rowKeyValue(row)">
          <tr class="ds-table__row" :class="{ 'ds-table__row--expanded': expandable && isExpanded(row), 'ds-table__row--selected': selectable && isSelected(row) }">
            <td v-if="selectable" class="ds-table__td ds-table__td--selection">
              <input
                type="checkbox"
                class="ds-table__checkbox"
                :checked="isSelected(row)"
                @change="toggleRow(row)"
              />
            </td>
            <td v-if="expandable" class="ds-table__td ds-table__td--expand">
              <button
                type="button"
                class="ds-table__expand-toggle"
                :class="{ 'ds-table__expand-toggle--open': isExpanded(row) }"
                :title="isExpanded(row) ? '收起' : '展开'"
                @click="toggleExpand(row)"
              >
                <ChevronRight :size="14" />
              </button>
            </td>
            <td
              v-for="col in columns"
              :key="col.key"
              class="ds-table__td"
              :class="[
                `ds-table__cell--${col.align ?? 'left'}`,
                { 'ds-table__cell--mono': col.mono, 'ds-table__cell--wrap': col.wrap }
              ]"
              :style="col.width ? { width: columnWidth(col.width) } : undefined"
            >
              <slot :name="`cell-${col.key}`" :row="row" :value="cellValue(row, col.key)" :index="index">
                {{ displayValue(cellValue(row, col.key)) }}
              </slot>
            </td>
          </tr>
          <tr v-if="expandable && isExpanded(row)" class="ds-table__expand-row">
            <td class="ds-table__expand-cell" :colspan="totalColumns">
              <slot name="expand" :row="row" />
            </td>
          </tr>
        </template>
      </tbody>
    </table>

    <div
      v-if="!loading && rows.length === 0"
      class="ds-table__empty"
      :class="{ 'ds-table__empty--custom': !!$slots.empty }"
    >
      <slot name="empty">
        <Inbox :size="30" class="ds-table__empty-icon" />
        <p class="ds-table__empty-title">{{ emptyTitle }}</p>
        <p v-if="emptyDescription" class="ds-table__empty-description">{{ emptyDescription }}</p>
      </slot>
    </div>
  </div>
</template>

<style scoped>
.ds-table {
  /* 作为 flex 子项时允许自身收缩，才能把超宽 table 留在这里滚动；
     否则内容的 min-content 宽度会把整个上层面板撑宽，最终被面板的 overflow 裁掉。 */
  min-width: 0;
  overflow-x: auto;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
}

.ds-table--frameless {
  border: none;
  border-radius: var(--ds-radius-none);
  background: transparent;
}

.ds-table__table {
  width: max-content;
  min-width: 100%;
  border-collapse: collapse;
}

.ds-table__th {
  position: sticky;
  top: 0;
  z-index: 1;
  background: var(--ds-panel);
  padding: 10px 16px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--ds-muted);
  text-align: left;
  white-space: nowrap;
  border-bottom: 1px solid var(--ds-line);
}

.ds-table__th--selection,
.ds-table__th--expand {
  width: 44px;
}

.ds-table__td {
  padding: 13px 16px;
  font-size: 13.5px;
  color: var(--ds-ink-soft);
  border-top: 1px solid var(--ds-line);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.ds-table__cell--wrap {
  min-width: 180px;
  max-width: 420px;
  white-space: normal;
  overflow-wrap: anywhere;
}

.ds-table__td--selection,
.ds-table__td--expand {
  width: 44px;
}

.ds-table__row > .ds-table__td {
  transition: background-color 120ms ease;
}

/* 行级联淡入:首屏/翻页时逐行浮现,前 12 行 24ms 步进 */
@keyframes ds-table-row-in {
  from {
    opacity: 0;
    transform: translateY(4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.ds-table__row {
  animation: ds-table-row-in 260ms ease both;
}

.ds-table__row:nth-child(2) { animation-delay: 24ms; }
.ds-table__row:nth-child(3) { animation-delay: 48ms; }
.ds-table__row:nth-child(4) { animation-delay: 72ms; }
.ds-table__row:nth-child(5) { animation-delay: 96ms; }
.ds-table__row:nth-child(6) { animation-delay: 120ms; }
.ds-table__row:nth-child(7) { animation-delay: 144ms; }
.ds-table__row:nth-child(8) { animation-delay: 168ms; }
.ds-table__row:nth-child(9) { animation-delay: 192ms; }
.ds-table__row:nth-child(10) { animation-delay: 216ms; }
.ds-table__row:nth-child(11) { animation-delay: 240ms; }
.ds-table__row:nth-child(n + 12) { animation-delay: 264ms; }

@media (prefers-reduced-motion: reduce) {
  .ds-table__row {
    animation: none;
  }
}

.ds-table__row:hover > .ds-table__td {
  background: color-mix(in srgb, var(--ds-accent-soft) 45%, transparent);
}

/* 选中行:accent-soft 底 + 首格 2px 主题色指示条,优先级高于 hover */
.ds-table__row--selected > .ds-table__td,
.ds-table__row--selected:hover > .ds-table__td {
  background: var(--ds-accent-soft);
}

.ds-table__row--selected > .ds-table__td:first-child {
  box-shadow: var(--ds-shadow-inset-accent);
}

.ds-table__cell--left {
  text-align: left;
}

.ds-table__cell--center {
  text-align: center;
}

.ds-table__cell--right {
  text-align: right;
}

.ds-table__cell--mono {
  font-family: var(--ds-font-mono);
  font-size: 12.5px;
}

.ds-table__checkbox {
  width: 14px;
  height: 14px;
  margin: 0;
  accent-color: var(--ds-accent);
  cursor: pointer;
  vertical-align: middle;
}

.ds-table__expand-toggle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border: none;
  border-radius: var(--ds-radius-sm);
  background: transparent;
  color: var(--ds-muted);
  cursor: pointer;
  transition:
    background-color 120ms ease,
    color 120ms ease,
    transform 160ms ease;
}

.ds-table__expand-toggle:hover {
  background: var(--ds-panel-muted);
  color: var(--ds-ink);
}

.ds-table__expand-toggle--open {
  transform: rotate(90deg);
}

.ds-table__expand-cell {
  padding: 0;
  border-top: 1px solid var(--ds-line);
  background: var(--ds-panel-muted);
}

.ds-table__skeleton {
  display: block;
  height: 12px;
  border-radius: var(--ds-radius-sm);
  background: linear-gradient(
    90deg,
    var(--ds-panel-muted) 25%,
    var(--ds-line) 50%,
    var(--ds-panel-muted) 75%
  );
  background-size: 200% 100%;
  animation: ds-table-shimmer 1.2s ease-in-out infinite;
}

.ds-table__skeleton--box {
  width: 14px;
  height: 14px;
}

@keyframes ds-table-shimmer {
  0% {
    background-position: 200% 0;
  }
  100% {
    background-position: -200% 0;
  }
}

.ds-table__empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 48px 16px;
  border-top: 1px solid var(--ds-line);
  text-align: center;
}

/* 消费方通过 empty 插槽传入 DsEmpty 时,padding 交由 DsEmpty 控制,避免叠加 */
.ds-table__empty--custom {
  gap: 0;
  padding: 0;
}

.ds-table__empty-icon {
  color: var(--ds-faint);
}

.ds-table__empty-title {
  margin: 0;
  font-size: 13.5px;
  font-weight: 600;
  color: var(--ds-ink-soft);
}

.ds-table__empty-description {
  margin: 0;
  font-size: 12.5px;
  color: var(--ds-muted);
}
</style>
