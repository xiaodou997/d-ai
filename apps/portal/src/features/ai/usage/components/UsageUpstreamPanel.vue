<!--
  按上游资源面板:每个上游账号/凭证池在当前窗口的产出,用于和账本对成本。
  汇总接口一次性返回窗口内全部上游行、无分页参数,故不渲染分页器。
-->
<script setup lang="ts">
import { computed } from "vue";
import { DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

import type { UsageFilterChip, UsageUpstreamSummaryRowDTO } from "../model";
import { formatCompactNumber, formatNumber, formatUSD2 } from "../format";

const props = defineProps<{
  filterChips: UsageFilterChip[];
  loading: boolean;
  rows: UsageUpstreamSummaryRowDTO[];
}>();

const emit = defineEmits<{
  refresh: [];
}>();

const columns: DsTableColumn[] = [
  { key: "resource", title: "上游资源", width: 240 },
  { key: "request_count", title: "请求数", width: 110, align: "right", mono: true },
  { key: "success_rate", title: "成功率", width: 110, align: "right", mono: true },
  { key: "outcome", title: "成功 / 失败", width: 140, align: "right", mono: true },
  { key: "prompt_tokens", title: "输入 Token", width: 126, align: "right", mono: true },
  { key: "completion_tokens", title: "输出 Token", width: 126, align: "right", mono: true },
  { key: "total_tokens", title: "总 Token", width: 126, align: "right", mono: true },
  { key: "image_units", title: "图片张数", width: 110, align: "right", mono: true },
  { key: "payable", title: "租户结算应收", width: 140, align: "right", mono: true }
];

function rowKey(row: UsageUpstreamSummaryRowDTO) {
  return `${row.target_kind}:${row.target_id}`;
}

// DsTable 的 row-key 只接受字段名,为行补上 target_kind:target_id 复合键
const tableRows = computed(() =>
  props.rows.map((row) => ({ ...row, __row_key: rowKey(row) }))
);

// 资源被删除后 target_name 为空，但历史用量仍要能对账，所以依次回退到
// 请求时的 provider_code 快照、最后到裸 ID，绝不隐藏这一行。
function resourceLabel(row: UsageUpstreamSummaryRowDTO) {
  return row.target_name || row.provider_code || row.target_id;
}

function isDeleted(row: UsageUpstreamSummaryRowDTO) {
  return !row.target_name;
}

function kindLabel(row: UsageUpstreamSummaryRowDTO) {
  return row.target_kind === "oauth_pool" ? "凭证池" : "上游账号";
}

function successRate(row: UsageUpstreamSummaryRowDTO) {
  if (!row.request_count) return "0%";
  return `${((row.success_count / row.request_count) * 100).toFixed(1)}%`;
}
</script>

<template>
  <section class="usage-upstream-panel">
    <div class="usage-panel-toolbar">
      <div class="usage-panel-context">
        <p class="usage-panel-context__title">当前口径</p>
        <div class="usage-panel-context__chips">
          <DsTag v-if="!props.filterChips.length" tone="neutral">未附加字段筛选</DsTag>
          <DsTag v-for="chip in props.filterChips" :key="chip.key" tone="accent">
            {{ chip.label }} · {{ chip.value }}
          </DsTag>
        </div>
      </div>

      <div class="usage-panel-actions">
        <el-button type="primary" :loading="props.loading" @click="emit('refresh')">刷新</el-button>
      </div>
    </div>

    <DsTable
      :frame="false"
      :columns="columns"
      :rows="tableRows"
      row-key="__row_key"
      :loading="props.loading"
      empty-title="当前窗口没有命中任何上游资源"
    >
      <template #cell-resource="{ row }">
        <div class="stack-cell">
          <span class="stack-cell__main">{{ resourceLabel(row) }}</span>
          <span class="stack-cell__sub">
            {{ kindLabel(row) }}
            <template v-if="isDeleted(row)"> · 资源已删除，按请求时快照展示</template>
          </span>
        </div>
      </template>

      <template #cell-success_rate="{ row }">
        {{ successRate(row) }}
      </template>

      <template #cell-outcome="{ row }">
        {{ row.success_count }} / {{ row.failed_count }}
      </template>

      <template #cell-prompt_tokens="{ row }">
        {{ formatCompactNumber(row.total_prompt_tokens) }}
      </template>

      <template #cell-completion_tokens="{ row }">
        {{ formatCompactNumber(row.total_completion_tokens) }}
      </template>

      <template #cell-total_tokens="{ row }">
        {{ formatCompactNumber(row.total_tokens) }}
      </template>

      <template #cell-image_units="{ row }">
        {{ row.image_units ? formatNumber(row.image_units) : "—" }}
      </template>

      <template #cell-payable="{ row }">
        <span class="upstream-payable">{{ formatUSD2(row.tenant_payable_usd) }}</span>
      </template>
    </DsTable>
  </section>
</template>

<style scoped>
.usage-upstream-panel {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 16px;
  min-width: 0;
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
  align-items: center;
  gap: 10px;
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

.upstream-payable {
  font-weight: 700;
}
</style>
