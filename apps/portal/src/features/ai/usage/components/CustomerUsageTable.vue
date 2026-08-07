<!--
  用户端使用记录明细表:DsTable 高密度摘要 + DsPagination。
  重构:el-table/el-table-column 迁移为 DsTable(columns + #cell-{key} 插槽,frame=false 嵌入面板),
       请求 ID 列 mono,Token/积分/延迟列右对齐,空态走 DsTable empty;
       接口无分页(本地切片),DsPagination 始终渲染展示「共 N 条」。
       组件 props/emits 与"详情打开抽屉"交互不变。
-->
<script setup lang="ts">
import {
  UsageCostCell,
  UsageLatencyCell,
  UsageTag,
  UsageTokenCell,
  formatUsageTimestamp
} from "@/platform/ai/usage";
import { DsPagination, DsTable, type DsTableColumn } from "@/shared/ui";

import type { CustomerUsageLog } from "../model";

defineProps<{
  loading: boolean;
  page: number;
  pageSize: number;
  rows: CustomerUsageLog[];
  total: number;
}>();
defineEmits<{
  pageChange: [page: number];
  pageSizeChange: [pageSize: number];
  select: [row: CustomerUsageLog];
}>();

const columns: DsTableColumn[] = [
  { key: "created_at", title: "时间", width: 150 },
  { key: "target", title: "模型", width: 190 },
  { key: "group", title: "分组", width: 160 },
  { key: "status", title: "状态", width: 90 },
  { key: "source", title: "来源", width: 100 },
  { key: "billing", title: "计费来源", width: 100 },
  { key: "token", title: "Token", width: 170, align: "right" },
  { key: "credits", title: "积分", width: 110, align: "right" },
  { key: "latency", title: "延迟", width: 120, align: "right" },
  { key: "request_id", title: "请求 ID", width: 190, mono: true },
  { key: "actions", title: "操作", width: 70 }
];

function targetLabel(row: CustomerUsageLog) {
  return row.model_code || "-";
}

function groupLabel(row: CustomerUsageLog) {
  return row.billing_group_label_snapshot || row.group_name_snapshot || row.group_id || "-";
}

function billingSourceLabel(row: CustomerUsageLog) {
  return row.billing_source === "subscription" ? "订阅内" : "按量";
}
</script>

<template>
  <div class="customer-usage-table">
    <DsTable
      :frame="false"
      :columns="columns"
      :rows="rows"
      row-key="request_id"
      :loading="loading"
      empty-title="暂无使用记录"
      empty-description="调整筛选条件或记录范围后重试"
    >
      <template #cell-created_at="{ row }">
        <span class="mono text-xs">{{ formatUsageTimestamp(row.created_at) }}</span>
      </template>

      <template #cell-target="{ row }">
        <span class="target-cell">
          <span class="target-chip">{{ targetLabel(row) }}</span>
          <UsageTag v-if="row.stream" kind="stream" :value="row.stream" />
          <UsageTag v-if="row.reasoning_effort" kind="effort" :value="row.reasoning_effort" />
        </span>
      </template>

      <template #cell-group="{ row }">
        <span class="ellipsis">{{ groupLabel(row) }}</span>
      </template>

      <template #cell-status="{ row }">
        <UsageTag kind="status" :value="row.request_status" />
      </template>

      <template #cell-source="{ row }">
        <UsageTag kind="source" :value="row.request_source" />
      </template>

      <template #cell-billing="{ row }">
        <span class="billing-source" :class="{ 'is-subscription': row.billing_source === 'subscription' }">{{ billingSourceLabel(row) }}</span>
      </template>

      <template #cell-token="{ row }">
        <UsageTokenCell :prompt="row.prompt_tokens" :completion="row.completion_tokens" :cache-read="row.cache_read_tokens" :cache-write="row.cache_write_tokens" :reasoning="row.reasoning_tokens" />
      </template>

      <template #cell-credits="{ row }">
        <UsageCostCell :credits="row.user_charged_credits" />
      </template>

      <template #cell-latency="{ row }">
        <UsageLatencyCell :latency-ms="row.latency_ms" :first-token-ms="row.first_token_latency_ms" />
      </template>

      <template #cell-request_id="{ row }">
        <span class="ellipsis">{{ row.request_id }}</span>
      </template>

      <template #cell-actions="{ row }">
        <el-button link type="primary" @click="$emit('select', row)">详情</el-button>
      </template>
    </DsTable>

    <div class="customer-usage-table__pager">
      <DsPagination
        :page="page"
        :page-size="pageSize"
        :total="total"
        :page-sizes="[10, 20, 50]"
        @update:page="$emit('pageChange', $event)"
        @update:page-size="$emit('pageSizeChange', $event)"
      />
    </div>
  </div>
</template>

<style scoped>
/* 接 fill 链:表格区撑满面板 body 剩余高度,分页脚沉底 */
.customer-usage-table {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
}

.customer-usage-table :deep(.ds-table) {
  flex: 1;
  min-height: 0;
}

.customer-usage-table__pager {
  display: flex;
  justify-content: flex-end;
  padding: 12px 24px;
  border-top: 1px solid var(--ds-line);
}

.target-cell { display: inline-flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.target-chip { color: var(--ds-ink); font-weight: 650; }
.billing-source { display: inline-flex; padding: 1px 8px; border: 1px solid var(--ds-line); border-radius: 999px; color: var(--ds-muted); font-size: 12px; font-weight: 600; }
.billing-source.is-subscription { border-color: color-mix(in srgb, var(--ds-accent) 45%, transparent); background: color-mix(in srgb, var(--ds-accent) 12%, transparent); color: var(--ds-accent-hover); }
.ellipsis { display: block; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.mono { font-family: var(--ds-font-mono); }
.text-xs { font-size: 12px; }
</style>
