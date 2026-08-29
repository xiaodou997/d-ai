<!--
  用户排行面板:按用户计费金额降序,点击整行回到使用记录并锁定该用户。
  排行接口按 limit 返回 Top N、无分页参数,故不渲染分页器。
  DsTable 暂无行点击能力,行点击通过容器事件委托实现。
-->
<script setup lang="ts">
import { computed } from "vue";
import { DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

import type { AdminUsageRankingRow, UsageFilterChip } from "../model";
import {
  formatNumber,
  formatTimestamp,
  formatUSD
} from "../format";

const props = defineProps<{
  filterChips: UsageFilterChip[];
  isPlatformAdmin: boolean;
  limit: number;
  loading: boolean;
  rows: AdminUsageRankingRow[];
  total: number;
}>();

const emit = defineEmits<{
  refresh: [];
  selectUser: [userId: string];
  switchToRecords: [];
  updateLimit: [limit: number];
}>();

const limitModel = computed({
  get: () => props.limit,
  set: (value: number) => emit("updateLimit", value)
});

const columns = computed<DsTableColumn[]>(() => [
  { key: "rank", title: "#", width: 68, align: "center" },
  { key: "user", title: "用户", width: 220 },
  ...(props.isPlatformAdmin
    ? [{ key: "tenant", title: "租户", width: 180 } as DsTableColumn]
    : []),
  { key: "request_count", title: "请求数", width: 110, align: "right", mono: true },
  { key: "success_rate", title: "成功率", width: 110, align: "right", mono: true },
  { key: "total_tokens", title: "Token", width: 120, align: "right", mono: true },
  { key: "charged", title: "用户实际扣款", width: 128, align: "right", mono: true },
  { key: "outcome", title: "成功 / 失败", width: 148, align: "right", mono: true }
]);

function successRate(row: AdminUsageRankingRow) {
  if (!row.request_count) return "0%";
  return `${((row.success_count / row.request_count) * 100).toFixed(1)}%`;
}

function userLabel(row: AdminUsageRankingRow) {
  return row.identity.user.label;
}

function tenantLabel(row: AdminUsageRankingRow) {
  return row.identity.tenant.label;
}

// DsTable 行按 props.rows 顺序渲染,用事件委托把行索引映射回行数据
function handleTableClick(event: MouseEvent) {
  if (props.loading) return;
  const tr = (event.target as HTMLElement | null)?.closest("tr");
  if (!tr?.parentElement) return;
  const index = Array.prototype.indexOf.call(tr.parentElement.children, tr);
  const row = props.rows[index];
  if (row) emit("selectUser", row.user_id);
}
</script>

<template>
  <section class="usage-ranking-panel">
    <div class="usage-panel-toolbar">
      <div class="usage-panel-context">
        <p class="usage-panel-context__title">当前口径</p>
        <div class="usage-panel-context__chips">
          <DsTag v-if="!filterChips.length" tone="neutral">未附加字段筛选</DsTag>
          <DsTag v-for="chip in filterChips" :key="chip.key" tone="accent">
            {{ chip.label }} · {{ chip.value }}
          </DsTag>
        </div>
      </div>

      <div class="usage-panel-actions">
        <el-select v-model="limitModel" size="default" class="usage-panel-limit">
          <el-option :value="20" label="Top 20" />
          <el-option :value="50" label="Top 50" />
          <el-option :value="100" label="Top 100" />
        </el-select>
        <el-button @click="emit('switchToRecords')">去请求记录筛选</el-button>
        <el-button type="primary" :loading="loading" @click="emit('refresh')">刷新</el-button>
      </div>
    </div>

    <div class="usage-ranking-table" @click="handleTableClick">
      <DsTable
        :frame="false"
        :columns="columns"
        :rows="rows"
        row-key="user_id"
        :loading="loading"
        empty-title="暂无排行数据"
        empty-description="当前窗口没有可统计的用户用量"
      >
        <template #cell-rank="{ index }">
          <span class="ranking-index">{{ index + 1 }}</span>
        </template>

        <template #cell-user="{ row }">
          <div class="stack-cell">
            <span class="stack-cell__main">{{ userLabel(row) }}</span>
            <span class="stack-cell__sub">
              {{ formatTimestamp(row.last_requested_at) || "暂无请求时间" }}
              <template v-if="userLabel(row) !== row.user_id"> · {{ row.user_id }}</template>
            </span>
          </div>
        </template>

        <template #cell-tenant="{ row }">
          <div class="stack-cell">
            <span class="stack-cell__main">{{ tenantLabel(row) }}</span>
            <span v-if="tenantLabel(row) !== row.tenant_id" class="stack-cell__sub">{{ row.tenant_id }}</span>
          </div>
        </template>

        <template #cell-success_rate="{ row }">
          {{ successRate(row) }}
        </template>

        <template #cell-total_tokens="{ row }">
          {{ formatNumber(row.total_tokens) }}
        </template>

        <template #cell-charged="{ row }">
          <span class="ranking-cost">{{ formatUSD(row.total_user_charged_usd) }}</span>
        </template>

        <template #cell-outcome="{ row }">
          {{ row.success_count }} / {{ row.failed_count }}
        </template>
      </DsTable>
    </div>
  </section>
</template>

<style scoped>
.usage-ranking-panel {
  display: grid;
  gap: 16px;
}

.usage-ranking-table :deep(.ds-table__row) {
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

.usage-panel-limit {
  width: 110px;
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

.stack-cell__sub {
  color: var(--ds-faint);
  font-size: 11px;
  line-height: 1.4;
}

.ranking-index {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 28px;
  min-height: 28px;
  border-radius: var(--ds-radius-pill);
  background: color-mix(in srgb, var(--ds-accent-soft) 68%, var(--ds-panel));
  color: var(--ds-accent-hover);
  font-size: 12px;
  font-weight: 700;
}

.ranking-cost {
  color: var(--ds-positive);
}
</style>
