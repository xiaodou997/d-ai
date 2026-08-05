<!--
  租户端使用记录明细表:DsTable 高密度摘要,列与排序口径不变,
  「详情」操作打开调用详情抽屉;分页 DsPagination 始终渲染。
  被使用记录工作台与用户管理-使用记录面板(showUser=false 隐藏用户列)复用。
-->
<script setup lang="ts">
import { computed } from "vue";
import { PortalIdentityCell } from "@/platform/ai/identity";
import {
  UsageCostCell,
  UsageLatencyCell,
  UsageTag,
  UsageTokenCell,
  formatCredits,
  formatUsageTimestamp
} from "@/platform/ai/usage";
import { DsPagination, DsTable, type DsTableColumn } from "@/shared/ui";

import type { TenantUsageRow } from "../model";

const props = defineProps<{
  loading: boolean;
  page: number;
  pageSize: number;
  rows: TenantUsageRow[];
  showUser?: boolean;
  total: number;
}>();

defineEmits<{
  pageChange: [page: number];
  pageSizeChange: [pageSize: number];
  select: [row: TenantUsageRow];
}>();

// 用户列仅在租户全量记录场景展示(用户管理-使用记录面板传 showUser=false)
const columns = computed<DsTableColumn[]>(() => [
  { key: "created_at", title: "时间", width: 140 },
  ...(props.showUser !== false ? [{ key: "user", title: "用户", width: 130 }] : []),
  { key: "model", title: "模型" },
  { key: "group", title: "分组/倍率", width: 160 },
  { key: "status", title: "状态", width: 90 },
  { key: "source", title: "来源", width: 100 },
  { key: "token", title: "Token", width: 150, align: "right" },
  { key: "cost", title: "积分", width: 130, align: "right" },
  { key: "latency", title: "延迟", width: 110, align: "right" },
  { key: "actions", title: "操作", width: 70 }
]);

function targetLabel(row: TenantUsageRow) {
  return row.model_code || (row.app_name ? `应用 · ${row.app_name}` : "-");
}

function groupLabel(row: TenantUsageRow) {
  return row.billing_group_label_snapshot || row.group_name_snapshot || row.group_id || "-";
}
</script>

<template>
  <div class="tenant-usage-table">
    <DsTable
      :frame="false"
      :columns="columns"
      :rows="rows"
      row-key="request_id"
      :loading="loading"
      empty-title="暂无使用记录"
      empty-description="调整筛选条件或时间范围后重试"
    >
      <template #cell-created_at="{ row }">
        <span class="time-cell">{{ formatUsageTimestamp(row.created_at) }}</span>
      </template>

      <template #cell-user="{ row }">
        <PortalIdentityCell :label="row.userLabel" />
      </template>

      <template #cell-model="{ row }">
        <span class="model-cell">
          <span class="model-chip">{{ targetLabel(row) }}</span>
          <UsageTag v-if="row.stream" kind="stream" :value="row.stream" />
          <UsageTag v-if="row.reasoning_effort" kind="effort" :value="row.reasoning_effort" />
        </span>
      </template>

      <template #cell-group="{ row }">
        <span class="group-chip">{{ groupLabel(row) }}</span>
      </template>

      <template #cell-status="{ row }">
        <UsageTag kind="status" :value="row.request_status" />
      </template>

      <template #cell-source="{ row }">
        <UsageTag kind="source" :value="row.request_source" />
      </template>

      <template #cell-token="{ row }">
        <UsageTokenCell
          :prompt="row.prompt_tokens"
          :completion="row.completion_tokens"
          :cache-read="row.cache_read_tokens"
          :cache-write="row.cache_write_tokens"
          :reasoning="row.reasoning_tokens"
        />
      </template>

      <template #cell-cost="{ row }">
        <UsageCostCell
          :credits="row.user_charged_credits"
          :secondary="[
            { label: '租户成本', value: formatCredits(row.tenant_payable_credits) },
            { label: '零售应收', value: formatCredits(row.user_payable_credits) }
          ]"
        />
      </template>

      <template #cell-latency="{ row }">
        <UsageLatencyCell :latency-ms="row.latency_ms" :first-token-ms="row.first_token_latency_ms" />
      </template>

      <template #cell-actions="{ row }">
        <el-button link type="primary" @click="$emit('select', row)">详情</el-button>
      </template>
    </DsTable>

    <div class="tenant-usage-table__pager">
      <DsPagination
        :page="page"
        :page-size="pageSize"
        :total="total"
        :page-sizes="[10, 20, 50, 100]"
        @update:page="$emit('pageChange', $event)"
        @update:page-size="$emit('pageSizeChange', $event)"
      />
    </div>
  </div>
</template>

<style scoped>
.tenant-usage-table {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
  flex: 1;
  min-height: 0;
}

/* DsTable 撑满剩余高度并内部滚动,空态纵向居中 */
.tenant-usage-table :deep(.ds-table) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.tenant-usage-table :deep(.ds-table__empty) {
  flex: 1;
  justify-content: center;
}

.tenant-usage-table__pager {
  display: flex;
  justify-content: flex-end;
  flex-shrink: 0;
}

.time-cell {
  font-family: var(--ds-font-mono);
  font-size: 12px;
  color: var(--ds-ink);
  white-space: nowrap;
}

.model-cell {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.model-chip,
.group-chip {
  display: inline-block;
  border-radius: var(--ds-radius-control);
  padding: 2px 8px;
  font-size: 12px;
  font-weight: 600;
}

.model-chip {
  background: var(--ds-panel-muted);
  color: var(--ds-ink-soft);
  font-family: var(--ds-font-mono);
}

.group-chip {
  background: var(--ds-positive-soft);
  color: var(--ds-positive);
  font-weight: 700;
}
</style>
