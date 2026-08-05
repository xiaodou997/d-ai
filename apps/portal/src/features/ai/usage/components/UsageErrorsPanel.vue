<!--
  错误记录面板:固定只看失败请求,点击整行打开请求详情。
  DsTable 暂无行点击能力,行点击通过容器事件委托实现。
-->
<script setup lang="ts">
import { computed } from "vue";
import {
  UsageTag,
  formatMs
} from "@/platform/ai/usage";
import { DsPagination, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

import type { AdminUsageRow, UsageFilterChip, UsagePagination } from "../model";
import {
  formatTimestamp,
  resolveRequestTotalMs
} from "../format";

const props = defineProps<{
  filterChips: UsageFilterChip[];
  isPlatformAdmin: boolean;
  loading: boolean;
  pagination: UsagePagination;
  rows: AdminUsageRow[];
  total: number;
}>();

const emit = defineEmits<{
  pageChange: [page: number];
  pageSizeChange: [size: number];
  refresh: [];
  selectRecord: [row: AdminUsageRow];
  switchToRecords: [];
}>();

const columns = computed<DsTableColumn[]>(() => [
  { key: "created_at", title: "时间", width: 156, mono: true },
  ...(props.isPlatformAdmin
    ? [{ key: "tenant", title: "租户", width: 180 } as DsTableColumn]
    : []),
  { key: "user", title: "用户", width: 180 },
  { key: "chain", title: "模型链路", width: 220 },
  { key: "result", title: "结果", width: 168 },
  { key: "error", title: "错误摘要" },
  { key: "duration", title: "总耗时", width: 110, align: "right", mono: true }
]);

function statusText(row: AdminUsageRow) {
  return `HTTP ${row.http_status ?? "—"} / UP ${row.upstream_status ?? "—"}`;
}

function requestScopeText(row: AdminUsageRow) {
  return row.identity.user.label;
}

function requestScopeMeta(row: AdminUsageRow) {
  return row.identity.user.meta;
}

function tenantText(row: AdminUsageRow) {
  return row.identity.tenant.label;
}

// DsTable 行按 props.rows 顺序渲染,用事件委托把行索引映射回行数据
function handleTableClick(event: MouseEvent) {
  if (props.loading) return;
  const tr = (event.target as HTMLElement | null)?.closest("tr");
  if (!tr?.parentElement) return;
  const index = Array.prototype.indexOf.call(tr.parentElement.children, tr);
  const row = props.rows[index];
  if (row) emit("selectRecord", row);
}
</script>

<template>
  <section class="usage-errors-panel">
    <div class="usage-panel-toolbar">
      <div class="usage-panel-context">
        <p class="usage-panel-context__title">当前口径</p>
        <div class="usage-panel-context__chips">
          <DsTag tone="danger">状态 · failed</DsTag>
          <DsTag v-if="!filterChips.length" tone="neutral">未附加字段筛选</DsTag>
          <DsTag v-for="chip in filterChips" :key="chip.key" tone="accent">
            {{ chip.label }} · {{ chip.value }}
          </DsTag>
        </div>
      </div>

      <div class="usage-panel-actions">
        <el-button @click="emit('switchToRecords')">去请求记录筛选</el-button>
        <el-button type="primary" :loading="loading" @click="emit('refresh')">刷新</el-button>
      </div>
    </div>

    <div class="usage-errors-table" @click="handleTableClick">
      <DsTable
        :frame="false"
        :columns="columns"
        :rows="rows"
        row-key="request_id"
        :loading="loading"
        empty-title="暂无错误记录"
        empty-description="当前窗口没有失败请求"
      >
        <template #cell-created_at="{ row }">
          {{ formatTimestamp(row.created_at) }}
        </template>

        <template #cell-tenant="{ row }">
          <div class="stack-cell">
            <span class="stack-cell__main">{{ tenantText(row) }}</span>
            <span v-if="tenantText(row) !== row.tenant_id" class="stack-cell__sub">{{ row.tenant_id }}</span>
          </div>
        </template>

        <template #cell-user="{ row }">
          <div class="stack-cell">
            <span class="stack-cell__main">{{ requestScopeText(row) }}</span>
            <span class="stack-cell__sub">
              {{ row.request_source }}
              <template v-if="requestScopeMeta(row)"> · {{ requestScopeMeta(row) }}</template>
            </span>
          </div>
        </template>

        <template #cell-chain="{ row }">
          <div class="stack-cell">
            <span class="stack-cell__main">{{ row.model_code }}</span>
            <span class="stack-cell__sub">{{ [row.provider_code, row.upstream_model].filter(Boolean).join(" · ") || row.group_name_snapshot || "未命中路由摘要" }}</span>
          </div>
        </template>

        <template #cell-result="{ row }">
          <div class="stack-cell">
            <span class="stack-cell__main stack-cell__main--inline">
              <UsageTag kind="status" :value="row.request_status" />
            </span>
            <span class="stack-cell__sub mono">{{ statusText(row) }}</span>
          </div>
        </template>

        <template #cell-error="{ row }">
          <div class="stack-cell">
            <span class="stack-cell__main">{{ row.error_code || "请求失败" }}</span>
            <span class="stack-cell__sub stack-cell__sub--danger">
              {{ row.error_message || row.request_id }}
            </span>
          </div>
        </template>

        <template #cell-duration="{ row }">
          {{ formatMs(resolveRequestTotalMs(row) || null) }}
        </template>
      </DsTable>
    </div>

    <div class="usage-panel-pager">
      <DsPagination
        :page="pagination.page"
        :page-size="pagination.size"
        :total="total"
        :page-sizes="[10, 20, 50, 100]"
        @update:page="emit('pageChange', $event)"
        @update:page-size="emit('pageSizeChange', $event)"
      />
    </div>
  </section>
</template>

<style scoped>
.usage-errors-panel {
  display: grid;
  gap: 16px;
}

.usage-errors-table :deep(.ds-table__row) {
  cursor: pointer;
}

.usage-panel-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
}

.usage-panel-context {
  display: grid;
  gap: 8px;
}

.usage-panel-context__title {
  margin: 0;
  color: var(--ds-muted);
  font-size: 12px;
  font-weight: 700;
}

.usage-panel-context__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.usage-panel-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.usage-panel-pager {
  display: flex;
  justify-content: flex-end;
}

.stack-cell {
  display: inline-flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
}

.stack-cell__main {
  color: var(--ds-ink);
  font-size: 12px;
  font-weight: 650;
  line-height: 1.35;
}

.stack-cell__main--inline {
  display: inline-flex;
  align-items: center;
}

.stack-cell__sub {
  color: var(--ds-faint);
  font-size: 11px;
  line-height: 1.4;
}

.stack-cell__sub--danger {
  color: var(--ds-danger);
}

.mono {
  font-family: var(--ds-font-mono);
}
</style>
