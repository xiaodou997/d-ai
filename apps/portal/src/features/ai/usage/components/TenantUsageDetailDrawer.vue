<script setup lang="ts">
import { computed } from "vue";
import {
  UsageTag,
  formatCredits,
  formatMs,
  formatTokenCount,
  requestSourceLabel
} from "@/platform/ai/usage";
import { formatMultiplier } from "@/platform/ai/utils";

import type { TenantUsageRow } from "../model";

const props = defineProps<{
  open: boolean;
  row: TenantUsageRow | null;
}>();

const emit = defineEmits<{ close: [] }>();
const drawerOpen = computed({ get: () => props.open, set: (value) => { if (!value) emit("close"); } });

function targetLabel(row: TenantUsageRow) {
  return row.model_code || "-";
}

function groupLabel(row: TenantUsageRow) {
  return row.billing_group_label_snapshot || row.group_name_snapshot || row.group_id || "-";
}

function multiplierLabel(value?: number | null) {
  if (value == null) return "-";
  return `${formatMultiplier(value)}x`;
}
</script>

<template>
  <el-drawer v-model="drawerOpen" title="调用详情" size="480px" append-to-body destroy-on-close>
    <div v-if="row" class="detail-sections">
      <section class="detail-section">
        <h3>基础信息</h3>
        <dl>
          <dt>Request ID</dt><dd class="mono">{{ row.request_id }}</dd>
          <dt>时间</dt><dd>{{ row.created_at ? new Date(row.created_at).toLocaleString("zh-CN") : "-" }}</dd>
          <dt>模型</dt><dd>{{ targetLabel(row) }}</dd>
          <dt>请求分组</dt><dd>{{ groupLabel(row) }}</dd>
          <dt>最终倍率</dt><dd class="mono">{{ multiplierLabel(row.effective_user_multiplier_snapshot) }}</dd>
          <dt>分组默认倍率</dt><dd class="mono">{{ multiplierLabel(row.group_default_user_multiplier_snapshot) }}</dd>
          <dt>流式</dt><dd><UsageTag kind="stream" :value="row.stream" /></dd>
          <dt>推理强度</dt><dd><UsageTag kind="effort" :value="row.reasoning_effort" /></dd>
          <dt>调用来源</dt><dd>{{ requestSourceLabel(row.request_source) }}</dd>
        </dl>
      </section>
      <section class="detail-section">
        <h3>状态</h3>
        <dl>
          <dt>请求状态</dt><dd><UsageTag kind="status" :value="row.request_status" /></dd>
          <dt>HTTP 状态</dt><dd>{{ row.http_status ?? "-" }}</dd>
          <dt>错误码</dt><dd class="mono">{{ row.error_code ?? "-" }}</dd>
          <dt>错误信息</dt><dd class="error-msg">{{ row.error_message ?? "-" }}</dd>
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
          <dt>租户结算扣费</dt><dd class="mono">{{ formatCredits(row.tenant_payable_credits) }}</dd>
          <dt>零售价格表原价</dt><dd class="mono">{{ formatCredits(row.retail_base_credits) }}</dd>
          <dt>用户零售应收</dt><dd class="mono">{{ formatCredits(row.user_payable_credits) }}</dd>
          <dt>用户实际扣款</dt><dd class="mono accent">{{ formatCredits(row.user_charged_credits) }}</dd>
          <dt>计费状态</dt><dd>{{ row.billing_status_label || row.billing_status }}</dd>
          <dt>总延迟</dt><dd class="mono">{{ formatMs(row.latency_ms) }}</dd>
          <dt>首 Token 延迟</dt><dd class="mono">{{ formatMs(row.first_token_latency_ms) }}</dd>
        </dl>
      </section>
    </div>
  </el-drawer>
</template>

<style scoped>
.detail-sections { display: flex; flex-direction: column; }
.detail-section { padding: 16px 0; border-bottom: 1px solid var(--ds-line); }
.detail-section h3 { margin: 0 0 12px; color: var(--ds-faint); font-size: 11px; font-weight: 700; text-transform: uppercase; }
.detail-section dl { display: grid; grid-template-columns: 120px 1fr; gap: 6px 12px; margin: 0; }
.detail-section dt { color: var(--ds-muted); font-size: 12px; font-weight: 600; }
.detail-section dd { min-width: 0; margin: 0; color: var(--ds-ink); font-size: 13px; overflow-wrap: anywhere; }
.mono { font-family: "SF Mono", "Fira Code", monospace; }
.accent { color: var(--ds-accent-hover); font-weight: 700; }
.error-msg { color: var(--ds-danger) !important; }
</style>
