<!--
  用户端使用记录明细表:DsTable 高密度摘要 + DsPagination。
  重构:el-table/el-table-column 迁移为 DsTable(columns + #cell-{key} 插槽,frame=false 嵌入面板),
       Token/费用/耗时列右对齐,空态走 DsTable empty;
       接口无分页(本地切片),DsPagination 始终渲染展示「共 N 条」。
       组件 props/emits 与"详情打开抽屉"交互不变。
-->
<script setup lang="ts">
import {
  UsageTag,
  UsageTokenCell,
  formatMs,
  formatUSD,
  formatUsageTimestamp
} from "@/platform/ai/usage";
import { formatMultiplier } from "@/platform/ai/utils";
import { DsPagination, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

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
  { key: "created_at", title: "时间", width: 140 },
  { key: "target", title: "模型", width: 190 },
  { key: "group", title: "分组/倍率", width: 180 },
  { key: "status", title: "状态", width: 90 },
  { key: "source", title: "来源", width: 100 },
  { key: "billing", title: "计费来源", width: 100 },
  { key: "token", title: "Token", width: 150, align: "right" },
  { key: "credits", title: "费用（USD）", width: 130, align: "right" },
  { key: "latency", title: "耗时", width: 120, align: "right" },
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

function groupMultiplier(row: CustomerUsageLog) {
  return row.effective_user_multiplier_snapshot == null
    ? ""
    : `×${formatMultiplier(row.effective_user_multiplier_snapshot)}`;
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
        <span class="time-cell">{{ formatUsageTimestamp(row.created_at) }}</span>
      </template>

      <template #cell-target="{ row }">
        <span class="target-cell">
          <span class="model-chip">{{ targetLabel(row) }}</span>
          <UsageTag v-if="row.stream" kind="stream" :value="row.stream" />
          <UsageTag v-if="row.reasoning_effort" kind="effort" :value="row.reasoning_effort" />
        </span>
      </template>

      <template #cell-group="{ row }">
        <div class="group-cell">
          <DsTag class="group-tag" :tone="groupLabel(row) === '-' ? 'neutral' : 'accent'" :title="groupLabel(row)">
            <span class="group-name">{{ groupLabel(row) }}</span>
          </DsTag>
          <span v-if="groupMultiplier(row)" class="group-multiplier mono">{{ groupMultiplier(row) }}</span>
        </div>
      </template>

      <template #cell-status="{ row }">
        <UsageTag kind="status" :value="row.request_status" />
      </template>

      <template #cell-source="{ row }">
        <UsageTag kind="source" :value="row.request_source" />
      </template>

      <template #cell-billing="{ row }">
        <DsTag :tone="row.billing_source === 'subscription' ? 'accent' : 'neutral'">{{ billingSourceLabel(row) }}</DsTag>
      </template>

      <template #cell-token="{ row }">
        <UsageTokenCell
          :prompt="row.prompt_tokens"
          :completion="row.completion_tokens"
          :cache-read="row.cache_read_tokens"
          :cache-write="row.cache_write_tokens"
        />
      </template>

      <template #cell-credits="{ row }">
        <span class="cost-value mono">{{ formatUSD(row.user_charged_usd) }}</span>
      </template>

      <template #cell-latency="{ row }">
        <div class="usage-metric usage-metric--timing">
          <span class="usage-metric__top mono">首 Token {{ formatMs(row.first_token_latency_ms) }}</span>
          <span class="usage-metric__bottom mono">请求耗时 {{ formatMs(row.latency_ms) }}</span>
        </div>
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
  min-width: 0;
  flex: 1;
  flex-direction: column;
  gap: 16px;
}

.customer-usage-table :deep(.ds-table) {
  flex: 1;
  min-height: 0;
  min-width: 0;
}

.customer-usage-table__pager {
  display: flex;
  justify-content: flex-end;
  min-width: 0;
  flex-shrink: 0;
}

.target-cell { display: inline-flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.model-chip { display: inline-block; border-radius: var(--ds-radius-control); background: var(--ds-panel-muted); padding: 2px 8px; color: var(--ds-ink-soft); font-family: var(--ds-font-mono); font-size: 12px; font-weight: 600; }
.time-cell { color: var(--ds-ink); font-family: var(--ds-font-mono); font-size: 12px; white-space: nowrap; }
.group-cell { display: inline-flex; min-width: 0; align-items: center; gap: 6px; }
.group-tag { min-width: 0; max-width: 138px; }
.group-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.group-multiplier { color: var(--ds-muted); font-size: 11px; font-weight: 700; white-space: nowrap; }
.cost-value { color: var(--ds-accent); font-size: 12px; font-weight: 700; white-space: nowrap; }
.usage-metric { display: inline-grid; justify-items: end; gap: 3px; }
.usage-metric__top, .usage-metric__bottom { display: inline-flex; align-items: center; white-space: nowrap; }
.usage-metric__top { color: var(--ds-info); font-size: 11px; font-weight: 700; }
.usage-metric__bottom { color: var(--ds-ink); font-size: 12px; font-weight: 700; }
.mono { font-family: var(--ds-font-mono); }
</style>
