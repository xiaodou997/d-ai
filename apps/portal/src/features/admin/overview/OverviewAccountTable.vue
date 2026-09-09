<script setup lang="ts">
import { computed } from "vue";
import { PortalContentCard } from "@/platform";
import { DsEmpty, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";
import type { OverviewAccount } from "./overviewModel";
import { formatNumber, formatUSDStat, rateText } from "./overviewUtils";

const props = withDefaults(defineProps<{ accounts: OverviewAccount[]; title?: string; description?: string; limit?: number }>(), {
  title: "账号经营表现",
  description: "按请求量查看上游账号与账号池的最终请求质量、缓存效率和结算金额。",
  limit: 8
});

const columns: DsTableColumn[] = [
  { key: "resource", title: "账号 / 账号池" },
  { key: "request_count", title: "请求", width: 90, align: "right", mono: true },
  { key: "success_rate", title: "成功率", width: 90, align: "right", mono: true },
  { key: "cache_rate", title: "缓存率", width: 90, align: "right", mono: true },
  { key: "total_tokens", title: "Token", width: 100, align: "right", mono: true },
  { key: "tenant_payable_usd", title: "应收", width: 100, align: "right", mono: true }
];

const rows = computed(() => props.accounts.slice(0, props.limit).map((row) => ({ ...row, __row_key: `${row.target_kind}:${row.target_id}` })));
function label(row: OverviewAccount) { return row.target_name || row.provider_code || row.target_id; }
function kind(row: OverviewAccount) { return row.target_kind === "oauth_pool" ? "账号池" : "上游账号"; }
function tone(rate?: number) { return rate != null && rate < 95 ? "warning" : "positive"; }
</script>

<template>
  <PortalContentCard :title="title" :description="description">
    <DsTable v-if="rows.length" :columns="columns" :rows="rows" row-key="__row_key" :frame="false">
      <template #cell-resource="{ row }"><div class="resource-cell"><strong>{{ label(row) }}</strong><small>{{ kind(row) }} · {{ row.provider_code || "未知供应商" }}</small></div></template>
      <template #cell-request_count="{ row }">{{ formatNumber(row.request_count) }}</template>
      <template #cell-success_rate="{ row }"><DsTag :tone="tone(row.success_rate)">{{ rateText(row.success_count, row.request_count) }}</DsTag></template>
      <template #cell-cache_rate="{ row }">{{ rateText(row.cache_read_tokens, row.prompt_tokens + row.cache_read_tokens) }}</template>
      <template #cell-total_tokens="{ row }">{{ formatNumber(row.total_tokens) }}</template>
      <template #cell-tenant_payable_usd="{ row }">{{ formatUSDStat(row.tenant_payable_usd) }}</template>
    </DsTable>
    <DsEmpty v-else title="暂无账号产出" description="当前时间范围内还没有关联上游账号的请求。" />
  </PortalContentCard>
</template>

<style scoped>
.resource-cell { display: grid; gap: 3px; min-width: 0; }
.resource-cell strong { overflow: hidden; color: var(--ds-ink); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.resource-cell small { color: var(--ds-muted); font-size: 10px; }
</style>
