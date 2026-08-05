<!--
  使用记录明细表:DsTable 高密度摘要,整行点击进入请求详情页。
  DsTable 暂无行点击/行高亮能力,行点击通过容器事件委托实现(保持"整行点开详情"语义);
  迁移后不再保留选中行高亮。
-->
<script setup lang="ts">
import { UserFilled, Wallet } from "@element-plus/icons-vue";

import {
  formatMs,
  formatTokenCount,
  UsageTag
} from "@/platform/ai/usage";
import { PortalIdentityCell } from "@/platform/ai/identity";
import { DsTable, type DsTableColumn } from "@/shared/ui";

import type { AdminUsageRow } from "../model";
import {
  formatCredits,
  formatTimestamp,
  modelRouteLabel,
  resolveFirstResponseByteMs,
  resolveRequestTotalMs,
  unitLabel
} from "../format";

const props = defineProps<{
  rows: AdminUsageRow[];
  loading: boolean;
  showUpstreamDetails?: boolean;
}>();

const emit = defineEmits<{
  selectRecord: [row: AdminUsageRow];
}>();

const columns: DsTableColumn[] = [
  { key: "created_at", title: "时间", width: 148 },
  { key: "subject", title: "调用主体" },
  { key: "chain", title: "模型链路", width: 220 },
  { key: "profile", title: "请求画像", width: 210 },
  { key: "cost", title: "消耗", width: 116, align: "right" },
  { key: "token", title: "Token", width: 216 },
  { key: "timing", title: "耗时", width: 128, align: "right" },
  { key: "result", title: "结果摘要", width: 320 }
];

// DsTable 行按 props.rows 顺序渲染,用事件委托把行索引映射回行数据
function handleTableClick(event: MouseEvent) {
  if (props.loading) return;
  const tr = (event.target as HTMLElement | null)?.closest("tr");
  if (!tr?.parentElement) return;
  const index = Array.prototype.indexOf.call(tr.parentElement.children, tr);
  const row = props.rows[index];
  if (row) emit("selectRecord", row);
}

function statusToneClass(status?: number | null) {
  if (status == null) return "status-text status-text--neutral";
  if (status >= 500) return "status-text status-text--danger";
  if (status === 429) return "status-text status-text--warning";
  if (status >= 400) return "status-text status-text--danger";
  if (status >= 200 && status < 300) return "status-text status-text--positive";
  return "status-text status-text--neutral";
}

function requestProfileTitle(row: AdminUsageRow) {
  return row.app_name || row.billing_group_label_snapshot || row.auth_method;
}

function requestProfileResolution(row: AdminUsageRow) {
  if (row.billable_unit_type !== "image") return "";
  return row.resolution || "";
}

function modelChainMeta(row: AdminUsageRow) {
  if (props.showUpstreamDetails !== false) {
    return [row.provider_code, row.upstream_model].filter(Boolean).join(" · ") || row.group_name_snapshot || "未命中路由摘要";
  }
  return row.group_name_snapshot || "上游信息已隐藏";
}

function firstTokenText(row: AdminUsageRow) {
  return formatMs(resolveFirstResponseByteMs(row) || null);
}

function totalDurationText(row: AdminUsageRow) {
  return formatMs(resolveRequestTotalMs(row) || null);
}

function subjectMeta(row: AdminUsageRow) {
  return [row.identity.user.label, row.identity.user.meta].filter(Boolean).join(" · ");
}
</script>

<template>
  <div class="usage-explorer-table" @click="handleTableClick">
    <DsTable
      :frame="false"
      :columns="columns"
      :rows="rows"
      row-key="request_id"
      :loading="loading"
      empty-title="暂无使用记录"
      empty-description="调整筛选条件或时间窗口后重试"
    >
      <template #cell-created_at="{ row }">
        <div class="time-cell">
          <span class="time-cell__main">{{ formatTimestamp(row.created_at) }}</span>
        </div>
      </template>

      <template #cell-subject="{ row }">
        <PortalIdentityCell :label="row.identity.tenant.label" :meta="subjectMeta(row)" />
      </template>

      <template #cell-chain="{ row }">
        <div class="stack-cell">
          <span class="stack-cell__main">{{ modelRouteLabel(row) }}</span>
          <span class="stack-cell__sub">
            {{ modelChainMeta(row) }}
          </span>
        </div>
      </template>

      <template #cell-profile="{ row }">
        <div class="stack-cell">
          <span class="stack-cell__headline">
            <span class="stack-cell__main stack-cell__main--truncate">{{ requestProfileTitle(row) }}</span>
            <span v-if="requestProfileResolution(row)" class="profile-chip profile-chip--resolution mono">
              {{ requestProfileResolution(row) }}
            </span>
          </span>
          <span class="stack-cell__sub stack-cell__sub--inline">
            <UsageTag kind="source" :value="row.request_source" />
            <UsageTag kind="stream" :value="row.stream" />
            <span class="profile-chip">{{ unitLabel(row.billable_unit_type) }}</span>
          </span>
        </div>
      </template>

      <template #cell-cost="{ row }">
        <div class="stack-cell stack-cell--right stack-cell--compact">
          <span v-if="Number(row.tenant_payable_credits)" class="cost-line cost-line--tenant" title="平台向租户结算">
            <el-icon class="cost-line__icon"><Wallet /></el-icon>
            <span class="mono">{{ formatCredits(row.tenant_payable_credits) }}</span>
          </span>
          <span v-if="Number(row.user_charged_credits)" class="cost-line cost-line--user" title="用户实际扣款">
            <el-icon class="cost-line__icon"><UserFilled /></el-icon>
            <span class="mono">{{ formatCredits(row.user_charged_credits) }}</span>
          </span>
          <span v-if="!Number(row.tenant_payable_credits) && !Number(row.user_charged_credits)" class="stack-cell__sub">—</span>
        </div>
      </template>

      <template #cell-token="{ row }">
        <div class="token-breakdown">
          <span class="token-stat token-stat--input">
            <span class="token-stat__label">输入</span>
            <span class="token-stat__value mono">{{ formatTokenCount(row.prompt_tokens) }}</span>
          </span>
          <span class="token-stat token-stat--output">
            <span class="token-stat__label">输出</span>
            <span class="token-stat__value mono">{{ formatTokenCount(row.completion_tokens) }}</span>
          </span>
          <template v-if="row.cache_read_tokens || row.cache_write_tokens">
            <span class="token-stat token-stat--cache">
              <span class="token-stat__label">缓存读</span>
              <span class="token-stat__value mono">{{ formatTokenCount(row.cache_read_tokens) }}</span>
            </span>
            <span class="token-stat token-stat--cache">
              <span class="token-stat__label">缓存写</span>
              <span class="token-stat__value mono">{{ formatTokenCount(row.cache_write_tokens) }}</span>
            </span>
          </template>
          <span v-if="row.reasoning_tokens" class="token-stat token-stat--reasoning">
            <span class="token-stat__label">推理</span>
            <span class="token-stat__value mono">{{ formatTokenCount(row.reasoning_tokens) }}</span>
          </span>
        </div>
      </template>

      <template #cell-timing="{ row }">
        <div class="usage-metric usage-metric--timing">
          <span class="usage-metric__top mono">首 Token {{ firstTokenText(row) }}</span>
          <span class="usage-metric__bottom mono">总耗时 {{ totalDurationText(row) }}</span>
        </div>
      </template>

      <template #cell-result="{ row }">
        <div class="stack-cell">
          <span class="stack-cell__main stack-cell__main--inline">
            <UsageTag kind="status" :value="row.request_status" />
            <span :class="statusToneClass(row.http_status)" class="mono">HTTP {{ row.http_status ?? "—" }}</span>
            <span :class="statusToneClass(row.upstream_status)" class="mono">UP {{ row.upstream_status ?? "—" }}</span>
          </span>
          <span
            class="stack-cell__sub"
            :class="{ 'stack-cell__sub--danger': Boolean(row.error_message || row.error_code) }"
          >
            {{ row.error_message || row.error_code || row.trace_id || "点击整行查看完整链路与载荷" }}
          </span>
        </div>
      </template>
    </DsTable>
  </div>
</template>

<style scoped>
.usage-explorer-table {
  width: 100%;
}

/* 整行可点击进入请求详情页 */
.usage-explorer-table :deep(.ds-table__row) {
  cursor: pointer;
}

.time-cell,
.stack-cell {
  display: inline-flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
}

.stack-cell--right {
  align-items: flex-end;
}

.stack-cell--compact {
  gap: 2px;
}

.time-cell__main,
.stack-cell__main {
  color: var(--ds-ink);
  font-size: 12px;
  font-weight: 650;
  line-height: 1.3;
}

.stack-cell__main--inline {
  display: inline-flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
}

.stack-cell__main--metric {
  font-weight: 700;
}

.time-cell__sub,
.stack-cell__sub {
  color: var(--ds-faint);
  font-size: 11px;
  line-height: 1.35;
}

.stack-cell__sub--inline {
  display: inline-flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
}

.stack-cell__headline {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.stack-cell__main--truncate {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stack-cell__sub--metric {
  font-weight: 600;
}

.stack-cell__sub--danger {
  color: var(--ds-danger);
}

.profile-chip {
  display: inline-flex;
  align-items: center;
  height: 20px;
  border: 1px solid color-mix(in srgb, var(--ds-line-strong) 82%, white 18%);
  border-radius: 999px;
  background: color-mix(in srgb, var(--ds-panel-muted) 78%, white 22%);
  padding: 0 8px;
  color: var(--ds-ink-soft);
  font-size: 11px;
  font-weight: 700;
  white-space: nowrap;
}

.profile-chip--resolution {
  color: var(--ds-info);
  background: color-mix(in srgb, var(--ds-info-soft) 78%, white 22%);
  border-color: color-mix(in srgb, var(--ds-info) 24%, var(--ds-info-soft));
}

.time-cell__main,
.stack-cell__main:not(.stack-cell__main--inline),
.stack-cell__sub:not(.stack-cell__sub--inline) {
  display: block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mono {
  font-family: var(--ds-font-mono);
}

.usage-metric {
  display: inline-grid;
  justify-items: end;
  gap: 3px;
}

.usage-metric__top {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  white-space: nowrap;
}

.usage-metric__bottom {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--ds-faint);
  font-size: 11px;
  line-height: 1.35;
  white-space: nowrap;
}

.usage-metric--timing .usage-metric__top {
  color: var(--ds-info);
  font-size: 11px;
  font-weight: 700;
}

.usage-metric--timing .usage-metric__bottom {
  color: var(--ds-ink);
  font-size: 12px;
  font-weight: 700;
}

.cost-line__icon {
  flex: 0 0 auto;
  font-size: 12px;
}

.token-breakdown {
  display: inline-grid;
  grid-template-columns: repeat(2, minmax(84px, 1fr));
  width: 100%;
  min-width: 188px;
  gap: 4px 12px;
}

.token-stat {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: baseline;
  gap: 6px;
  min-width: 0;
}

.token-stat__label {
  color: var(--ds-faint);
  font-size: 10px;
  font-weight: 600;
  line-height: 1.35;
  white-space: nowrap;
}

.token-stat__value {
  justify-self: end;
  font-size: 11.5px;
  font-weight: 700;
  line-height: 1.35;
  white-space: nowrap;
}

.token-stat--input .token-stat__value {
  color: var(--ds-positive);
}

.token-stat--output .token-stat__value {
  color: var(--ds-warning);
}

.token-stat--cache .token-stat__value {
  color: var(--ds-info);
}

.token-stat--reasoning {
  grid-column: 1 / -1;
  padding-top: 1px;
  border-top: 1px solid var(--ds-line);
}

.token-stat--reasoning .token-stat__value {
  color: color-mix(in srgb, var(--ds-warning) 76%, var(--ds-ink));
}

.cost-line {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  max-width: 100%;
  font-size: 11px;
  font-weight: 700;
  white-space: nowrap;
}

.cost-line--tenant {
  color: var(--ds-info);
}

.cost-line--user {
  color: var(--ds-accent);
}

.status-text--positive {
  color: var(--ds-positive);
}

.status-text--warning {
  color: var(--ds-warning);
}

.status-text--danger {
  color: var(--ds-danger);
}

.status-text--neutral {
  color: var(--ds-faint);
}
</style>
