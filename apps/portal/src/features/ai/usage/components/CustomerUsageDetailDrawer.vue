<script setup lang="ts">
import { computed } from "vue";
import {
  UsageTag,
  formatMs,
  formatTokenCount,
  formatUSD,
  requestSourceLabel
} from "@/platform/ai/usage";
import { formatMultiplier } from "@/platform/ai/utils";

import type { CustomerUsageLog } from "../model";

const props = defineProps<{ open: boolean; row: CustomerUsageLog | null }>();
const emit = defineEmits<{ close: [] }>();
const drawerOpen = computed({ get: () => props.open, set: (value) => { if (!value) emit("close"); } });

function targetLabel(row: CustomerUsageLog) {
  return row.model_code || "-";
}
function groupLabel(row: CustomerUsageLog) {
  return row.billing_group_label_snapshot || row.group_name_snapshot || row.group_id || "-";
}
function multiplierLabel(value?: number | null) {
  if (value == null) return "-";
  return `${formatMultiplier(value)}x`;
}
function serviceTierLabel(row: CustomerUsageLog) {
  return row.service_tier || "-";
}
</script>

<template>
  <el-drawer v-model="drawerOpen" title="请求详情" size="480px" append-to-body destroy-on-close>
    <div v-if="row" class="detail-sections">
      <section class="detail-section">
        <h3>基础信息</h3>
        <dl>
          <dt>Request ID</dt><dd class="mono">{{ row.request_id }}</dd>
          <dt>Trace ID</dt><dd class="mono">{{ row.trace_id || "-" }}</dd>
          <dt>时间</dt><dd>{{ row.created_at ? new Date(row.created_at).toLocaleString("zh-CN", { hour12: false }) : "-" }}</dd>
          <dt>模型</dt><dd>{{ targetLabel(row) }}</dd>
          <dt>分组</dt><dd>{{ groupLabel(row) }}</dd>
          <dt>请求倍率</dt><dd class="mono">{{ multiplierLabel(row.effective_user_multiplier_snapshot) }}</dd>
          <dt>服务档位</dt><dd>{{ serviceTierLabel(row) }}</dd>
          <dt>调用来源</dt><dd>{{ requestSourceLabel(row.request_source) }}</dd>
        </dl>
      </section>
      <section class="detail-section">
        <h3>状态</h3>
        <dl>
          <dt>请求状态</dt><dd><UsageTag kind="status" :value="row.request_status" /></dd>
          <dt>HTTP 状态</dt><dd>{{ row.http_status ?? "-" }}</dd>
          <dt>错误码</dt><dd class="mono">{{ row.error_code ?? "-" }}</dd>
          <dt>错误信息</dt><dd class="error-message">{{ row.error_message ?? "-" }}</dd>
        </dl>
      </section>
      <section class="detail-section">
        <h3>Token 用量</h3>
        <dl>
          <dt>输入 Token</dt><dd class="mono">{{ formatTokenCount(row.prompt_tokens) }}</dd>
          <dt>输出 Token</dt><dd class="mono">{{ formatTokenCount(row.completion_tokens) }}</dd>
          <dt>缓存读</dt><dd class="mono">{{ formatTokenCount(row.cache_read_tokens) }}</dd>
          <dt>缓存写</dt><dd class="mono">{{ formatTokenCount(row.cache_write_tokens) }}</dd>
          <dt>推理 Token</dt><dd class="mono">{{ formatTokenCount(row.reasoning_tokens) }}</dd>
          <dt>总 Token</dt><dd class="mono">{{ formatTokenCount(row.total_tokens) }}</dd>
        </dl>
      </section>
      <section class="detail-section">
        <h3>费用与性能</h3>
        <dl>
          <dt>计费单位</dt><dd>{{ row.billable_unit_type || "-" }}</dd>
          <dt>计费数量</dt><dd class="mono">{{ row.billable_units.toLocaleString("zh-CN") }}</dd>
          <dt>实际扣款</dt><dd class="mono accent">{{ formatUSD(row.user_charged_usd) }}</dd>
          <dt>总延迟</dt><dd class="mono">{{ formatMs(row.latency_ms) }}</dd>
          <dt>首 Token 延迟</dt><dd class="mono">{{ formatMs(row.first_token_latency_ms) }}</dd>
        </dl>
      </section>
    </div>
  </el-drawer>
</template>

<style scoped>
.detail-sections { display: flex; flex-direction: column; gap: 14px; }
.detail-section { padding: 14px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-card); background: var(--ds-surface); }
.detail-section h3 { margin: 0 0 12px; color: var(--ds-ink); font-size: 14px; font-weight: 700; }
.detail-section dl { display: grid; grid-template-columns: 112px minmax(0, 1fr); gap: 9px 12px; margin: 0; }
.detail-section dt { color: var(--ds-muted); font-size: 12px; }
.detail-section dd { min-width: 0; margin: 0; color: var(--ds-ink); overflow-wrap: anywhere; font-size: 13px; }
.mono { font-family: "SF Mono", "Fira Code", monospace; }
.error-message { color: var(--ds-danger) !important; }
.accent { color: var(--ds-accent-hover); font-weight: 700; }
@media (max-width: 720px) { .detail-section dl { grid-template-columns: 1fr; } }
</style>
