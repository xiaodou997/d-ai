<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ElMessage } from "element-plus";
import {
  AlertTriangle,
  Braces,
  Building2,
  ChevronDown,
  CircleDollarSign,
  Clock3,
  Copy,
  KeyRound,
  Route,
  UserRound
} from "lucide-vue-next";

import { PortalContentCard } from "@/platform";
import { formatMultiplier } from "@/platform/ai/utils";
import { formatMs, formatNumber, formatUSD, UsageTag } from "@/platform/ai/usage";

import { normalizeUsageAttempts, type UsageAttemptDetail, type UsageLogDetailDTO } from "../model";
import {
  formatJSON,
  formatTimestamp,
  resolveFirstResponseByteMs,
  resolveHeaderMs,
  resolveRequestSetupMs,
  resolveRequestTotalMs,
  resolveResponseTailMs
} from "../format";

const props = defineProps<{
  detail: UsageLogDetailDTO | null;
  loading: boolean;
}>();

type PayloadKey = "request_params" | "request_messages" | "response_message" | "media_refs" | "billing_breakdown";

const payloadOpen = ref(false);
const activePayload = ref<PayloadKey>("request_params");
const attemptsOpen = ref(false);

watch(
  () => props.detail?.request_id,
  () => {
    payloadOpen.value = false;
    activePayload.value = "request_params";
    attemptsOpen.value = false;
  }
);

const detailReady = computed(() => Boolean(props.detail));
const resolvedRequestId = computed(() => props.detail?.request_id || "");
const resolvedStatus = computed(() => props.detail?.request_status || "");
const resolvedTrace = computed(() => props.detail?.trace_id || "—");
const resolvedHeadline = computed(() => {
  const requested = props.detail?.requested_model || props.detail?.model_code || "—";
  const resolved = props.detail?.resolved_logical_model || requested;
  return requested === resolved ? requested : `${requested} → ${resolved}`;
});
const tenantLabel = computed(() => props.detail?.tenant_name || props.detail?.tenant_id || "—");
const userLabel = computed(() =>
  props.detail?.username || props.detail?.external_user_id || props.detail?.user_id || "—"
);
const groupLabel = computed(() =>
  props.detail?.group_name_snapshot || props.detail?.billing_group_label_snapshot || props.detail?.group_id || "—"
);
const accountLabel = computed(() => props.detail?.api_key_id || props.detail?.auth_method || "—");
const requestSourceLabel = computed(() => sourceLabel(props.detail?.request_source));
const billingSourceText = computed(() => billingSourceLabel(props.detail?.billing_source));

function multiplierLabel(value?: number | null) {
  return value == null ? "—" : `×${formatMultiplier(value)}`;
}

function durationLabel(value?: number | null) {
  const formatted = formatMs(value ?? null);
  return formatted === "-" ? "—" : formatted;
}

function moneyLabel(value?: number | null) {
  return value == null ? "—" : formatUSD(value);
}

function sourceLabel(value?: string | null) {
  return {
    api_key: "API Key",
    workspace: "工作台",
    openai_compatible: "OpenAI 兼容",
    internal: "内部调用"
  }[value || ""] || value || "—";
}

function billingSourceLabel(value?: string | null) {
  return {
    payg: "按量计费",
    subscription: "订阅内"
  }[value || ""] || value || "—";
}

function billingStatusLabel(status?: string | null) {
  return {
    free: "免费",
    pending: "待结算",
    settled: "已结算",
    failed: "结算失败"
  }[status || ""] || status || "—";
}

function refundStatusLabel(status?: string | null) {
  return status === "refunded" ? "已退款" : "未退款";
}

function timestampLabel(value?: number | null) {
  return value ? formatTimestamp(value) : "—";
}

function attemptOutcomeLabel(value: string) {
  return value === "success" ? "成功" : value === "failed" ? "失败" : value || "未知";
}

const timingSource = computed(() => ({
  request_total_ms: props.detail?.request_total_ms,
  final_attempt_total_ms: props.detail?.final_attempt_total_ms,
  first_token_latency_ms: props.detail?.first_token_latency_ms,
  latency_ms: props.detail?.latency_ms,
  first_response_byte_ms: props.detail?.first_response_byte_ms,
  final_attempt_header_ms: props.detail?.final_attempt_header_ms,
  request_setup_ms: props.detail?.request_setup_ms,
  response_tail_ms: props.detail?.response_tail_ms
}));

const timingSummary = computed(() => ({
  totalMs: resolveRequestTotalMs(timingSource.value) || null,
  firstResponseMs: resolveFirstResponseByteMs(timingSource.value) || null,
  headerMs: resolveHeaderMs(timingSource.value) || null
}));

const performanceFacts = computed(() => [
  { label: "网关准备", value: durationLabel(resolveRequestSetupMs(timingSource.value)) },
  { label: "上游响应头", value: durationLabel(resolveHeaderMs(timingSource.value)) },
  { label: "首个响应字节", value: durationLabel(resolveFirstResponseByteMs(timingSource.value)) },
  { label: "响应尾程", value: durationLabel(resolveResponseTailMs(timingSource.value)) }
].filter((fact) => fact.value !== "—"));

const routeSteps = computed(() => [
  {
    title: "客户请求模型",
    value: props.detail?.requested_model || props.detail?.model_code || "—",
    hint: `入口格式：${props.detail?.client_api_format || "—"}`
  },
  {
    title: "售价计费模型",
    value: props.detail?.resolved_logical_model || "—",
    hint: props.detail?.matched_dispatch_rule_summary || "未命中规则，原样路由"
  },
  {
    title: "成本计费模型",
    value: props.detail?.selected_upstream_model || props.detail?.upstream_model || "—",
    hint: `上游目标：${props.detail?.resolved_provider_family || "不限制"}`
  },
  {
    title: "客户端响应模型",
    value: props.detail?.public_response_model || "—",
    hint: `上游格式：${props.detail?.provider_api_format || "—"}`
  }
]);

const attempts = computed<UsageAttemptDetail[]>(() => normalizeUsageAttempts(props.detail?.attempts_detail));

interface BillingPriceLineSnapshot {
  input_context_tokens?: number;
  token_price_tier_index?: number;
  token_price_tier_up_to_input_tokens?: number | null;
}

interface BillingCostLineSnapshot {
  applied_multiplier?: number;
  raw_usd?: number;
  charge_usd_equivalent?: number;
  price_lines?: BillingPriceLineSnapshot;
}

interface BillingBreakdownSnapshot {
  version?: number;
  catalog_base?: BillingCostLineSnapshot;
  tenant_payable?: BillingCostLineSnapshot;
  user_payable?: BillingCostLineSnapshot;
  user_charged_micro?: number;
  price_lines?: BillingPriceLineSnapshot;
}

function parseBillingBreakdown(raw: unknown): BillingBreakdownSnapshot | null {
  if (typeof raw === "string") {
    try {
      return JSON.parse(raw) as BillingBreakdownSnapshot;
    } catch {
      return null;
    }
  }
  return raw && typeof raw === "object" ? raw as BillingBreakdownSnapshot : null;
}

const billingContext = computed(() => {
  const value = parseBillingBreakdown(props.detail?.billing_breakdown);
  if (!value || Number(value.version) < 2) return null;
  const sell = value.user_payable?.price_lines || value.price_lines;
  const provider = value.catalog_base?.price_lines;
  return {
    inputTokens: sell?.input_context_tokens,
    sellTier: formatContextTier(sell),
    providerTier: formatContextTier(provider)
  };
});

function formatContextTier(line?: BillingPriceLineSnapshot) {
  if (!line || line.token_price_tier_index == null) return "—";
  const upper = line.token_price_tier_up_to_input_tokens == null
    ? "无上限"
    : `≤ ${Number(line.token_price_tier_up_to_input_tokens).toLocaleString("zh-CN")}`;
  return `档位 ${Number(line.token_price_tier_index) + 1} · ${upper}`;
}

const payloadSections = computed(() => [
  { key: "request_params" as const, title: "请求参数", value: props.detail?.request_params },
  { key: "request_messages" as const, title: "请求消息", value: props.detail?.request_messages },
  { key: "response_message" as const, title: "响应摘要", value: props.detail?.response_message },
  { key: "media_refs" as const, title: "媒体引用", value: props.detail?.media_refs },
  { key: "billing_breakdown" as const, title: "计费明细", value: props.detail?.billing_breakdown }
].map((section) => ({
  ...section,
  present: section.value != null && section.value !== "",
  size: payloadSizeLabel(section.value)
})));

const activePayloadSection = computed(() =>
  payloadSections.value.find((section) => section.key === activePayload.value) || payloadSections.value[0]
);
const activePayloadText = computed(() => formatJSON(activePayloadSection.value?.value));
const payloadPresentCount = computed(() => payloadSections.value.filter((section) => section.present).length);

function payloadSizeLabel(value: unknown) {
  if (value == null || value === "") return "未记录";
  const length = formatJSON(value).length;
  return length < 1024 ? `${length} 字符` : `${(length / 1024).toFixed(1)} KB`;
}

async function copyText(value: string) {
  if (!value || value === "—") return;
  try {
    await navigator.clipboard.writeText(value);
    ElMessage.success("已复制");
  } catch {
    ElMessage.error("复制失败");
  }
}

function copyActivePayload() {
  void copyText(activePayloadText.value);
}
</script>

<template>
  <div class="usage-detail" v-loading="loading">
    <PortalContentCard class="usage-detail-header">
      <div class="usage-detail-header__main">
        <div class="usage-detail-eyebrow">AI 网关请求</div>
        <div class="usage-detail-header__title-row">
          <h1>{{ resolvedHeadline }}</h1>
          <UsageTag v-if="detailReady" kind="status" :value="resolvedStatus" />
          <UsageTag v-if="detail" kind="stream" :value="detail.stream" />
          <UsageTag v-if="detail?.reasoning_effort" kind="effort" :value="detail.reasoning_effort" />
        </div>
        <div class="usage-detail-header__meta">
          <span>{{ detail?.request_path || "—" }}</span>
          <span class="meta-separator">·</span>
          <span>{{ requestSourceLabel }}</span>
          <span class="meta-separator">·</span>
          <span>{{ timestampLabel(detail?.created_at) }}</span>
        </div>
      </div>
      <div class="usage-detail-header__actions">
        <button v-if="resolvedRequestId" class="detail-copy-button" type="button" @click="copyText(resolvedRequestId)">
          <Copy :size="14" />
          复制 Request ID
        </button>
        <button v-if="resolvedTrace !== '—'" class="detail-copy-button" type="button" @click="copyText(resolvedTrace)">
          <Copy :size="14" />
          复制 Trace ID
        </button>
      </div>
    </PortalContentCard>

    <PortalContentCard title="请求主体" description="先确认请求来自谁、使用哪个账号和分组，再查看技术链路。">
      <div class="identity-grid">
        <article class="identity-item identity-item--tenant">
          <div class="identity-item__label"><Building2 :size="15" />租户</div>
          <strong>{{ tenantLabel }}</strong>
          <small>{{ detail?.tenant_id || "未返回租户 ID" }}</small>
        </article>
        <article class="identity-item identity-item--user">
          <div class="identity-item__label"><UserRound :size="15" />用户</div>
          <strong>{{ userLabel }}</strong>
          <small>{{ detail?.user_id || detail?.external_user_id || "未关联终端用户" }}</small>
        </article>
        <article class="identity-item">
          <div class="identity-item__label"><KeyRound :size="15" />认证账号</div>
          <strong>{{ accountLabel }}</strong>
          <small>{{ detail?.key_owner_type || detail?.auth_method || "—" }}</small>
        </article>
        <article class="identity-item">
          <div class="identity-item__label"><Route :size="15" />分组</div>
          <strong>{{ groupLabel }}</strong>
          <small>{{ detail?.group_id || "未返回分组 ID" }}</small>
        </article>
        <article class="identity-item">
          <span>客户端 IP</span>
          <strong>{{ detail?.client_ip || "—" }}</strong>
        </article>
        <article class="identity-item identity-item--wide">
          <span>User-Agent</span>
          <strong>{{ detail?.user_agent || "—" }}</strong>
        </article>
      </div>
    </PortalContentCard>

    <PortalContentCard title="账务与倍率" description="金额单位为 USD；租户扣除积分就是平台与租户之间的结算金额。">
      <div class="billing-amount-grid">
        <article class="billing-amount billing-amount--user">
          <div class="billing-amount__label"><CircleDollarSign :size="16" />用户扣除积分</div>
          <strong>{{ moneyLabel(detail?.user_charged_usd) }}</strong>
          <small>用户账户本次实际扣除</small>
        </article>
        <article class="billing-amount billing-amount--tenant">
          <div class="billing-amount__label"><CircleDollarSign :size="16" />租户扣除积分</div>
          <strong>{{ moneyLabel(detail?.tenant_payable_usd) }}</strong>
          <small>平台与租户之间的结算金额</small>
        </article>
        <article class="billing-amount billing-amount--base">
          <div class="billing-amount__label"><CircleDollarSign :size="16" />上游参考成本</div>
          <strong>{{ moneyLabel(detail?.catalog_base_usd) }}</strong>
          <small>上游目录价基准，不代表平台实际采购价</small>
        </article>
      </div>

      <div class="billing-info-grid">
        <div class="billing-info-block">
          <div class="billing-info-block__title">计费倍率</div>
          <div class="billing-facts">
            <div><span>账号倍率</span><strong>{{ multiplierLabel(detail?.effective_user_multiplier_snapshot) }}</strong></div>
            <div><span>分组倍率</span><strong>{{ multiplierLabel(detail?.group_default_user_multiplier_snapshot) }}</strong></div>
            <div><span>账号覆盖</span><strong>{{ multiplierLabel(detail?.user_multiplier_override_snapshot) }}</strong></div>
            <div><span>计费分组</span><strong>{{ detail?.billing_group_label_snapshot || groupLabel }}</strong></div>
          </div>
        </div>
        <div class="billing-info-block">
          <div class="billing-info-block__title">结算状态</div>
          <div class="billing-facts">
            <div><span>计费来源</span><strong>{{ billingSourceText }}</strong></div>
            <div><span>计费状态</span><strong>{{ billingStatusLabel(detail?.billing_status) }}</strong></div>
            <div><span>结算时间</span><strong>{{ timestampLabel(detail?.settled_at) }}</strong></div>
            <div><span>退款状态</span><strong>{{ refundStatusLabel(detail?.refund_status) }}</strong></div>
          </div>
        </div>
      </div>

      <div class="token-strip">
        <div><span>输入 Token</span><strong>{{ formatNumber(detail?.prompt_tokens) }}</strong></div>
        <div><span>输出 Token</span><strong>{{ formatNumber(detail?.completion_tokens) }}</strong></div>
        <div><span>缓存读</span><strong>{{ formatNumber(detail?.cache_read_tokens) }}</strong></div>
        <div><span>缓存写</span><strong>{{ formatNumber(detail?.cache_write_tokens) }}</strong></div>
        <div><span>推理 Token</span><strong>{{ formatNumber(detail?.reasoning_tokens) }}</strong></div>
        <div><span>总 Token</span><strong>{{ formatNumber(detail?.total_tokens) }}</strong></div>
      </div>

      <div v-if="detail?.user_payable_usd != null || detail?.retail_base_usd != null" class="billing-secondary">
        <span>用户应付 {{ moneyLabel(detail?.user_payable_usd) }}</span>
        <span>零售原价 {{ moneyLabel(detail?.retail_base_usd) }}</span>
        <span v-if="detail?.api_key_quota_usd">Key 配额 {{ moneyLabel(detail.api_key_quota_usd) }}</span>
        <span v-if="billingContext?.inputTokens">上下文 {{ formatNumber(billingContext.inputTokens) }} tokens · {{ billingContext.sellTier }}</span>
      </div>
      <div v-if="detail?.settlement_error" class="billing-error">
        <AlertTriangle :size="15" />
        <span>结算异常：{{ detail.settlement_error }}</span>
      </div>
    </PortalContentCard>

    <div class="usage-detail-columns">
      <PortalContentCard title="性能时间线" description="总耗时是主指标，其余节点用于定位慢点。">
        <div class="performance-total">
          <div>
            <span>请求总耗时</span>
            <strong>{{ durationLabel(timingSummary.totalMs) }}</strong>
          </div>
          <div>
            <span>首响</span>
            <strong>{{ durationLabel(timingSummary.firstResponseMs) }}</strong>
          </div>
          <div>
            <span>尝试次数</span>
            <strong>{{ detail?.attempts_count ?? "—" }}</strong>
          </div>
        </div>
        <div v-if="performanceFacts.length" class="performance-facts">
          <div v-for="fact in performanceFacts" :key="fact.label" class="performance-fact">
            <Clock3 :size="14" />
            <span>{{ fact.label }}</span>
            <strong>{{ fact.value }}</strong>
          </div>
        </div>
      </PortalContentCard>

      <PortalContentCard title="路由决策" description="只展示关键模型身份，完整候选尝试按需查看。">
        <div class="route-flow">
          <article v-for="(step, index) in routeSteps" :key="step.title" class="route-step">
            <span class="route-step__index">{{ index + 1 }}</span>
            <span class="route-step__title">{{ step.title }}</span>
            <strong>{{ step.value }}</strong>
            <small>{{ step.hint }}</small>
          </article>
        </div>
        <div class="route-meta">
          <span>调度规则：{{ detail?.matched_dispatch_rule_id ? "已命中" : "未命中" }}</span>
          <span>协议转换：{{ detail?.protocol_conversion_enabled ? "已启用" : "未启用" }}</span>
          <span>模型映射：{{ detail?.upstream_model_mapping_applied ? "已发生" : "未发生" }}</span>
        </div>
        <button v-if="attempts.length" class="route-attempt-toggle" type="button" @click="attemptsOpen = !attemptsOpen">
          <span>{{ attempts.length }} 次候选尝试 · {{ attemptsOpen ? "收起明细" : "查看明细" }}</span>
          <ChevronDown :size="15" :class="{ 'is-open': attemptsOpen }" />
        </button>
        <el-table v-if="attemptsOpen" :data="attempts" size="small" class="attempts-table">
          <el-table-column label="#" type="index" width="44" />
          <el-table-column label="供应商" prop="provider_code" min-width="110">
            <template #default="{ row }">{{ row.provider_code || "—" }}</template>
          </el-table-column>
          <el-table-column label="上游模型" prop="upstream_model" min-width="160" show-overflow-tooltip>
            <template #default="{ row }">{{ row.upstream_model || "—" }}</template>
          </el-table-column>
          <el-table-column label="结果" width="80">
            <template #default="{ row }">
              <span :class="['attempt-result', row.outcome === 'success' ? 'is-success' : 'is-danger']">{{ attemptOutcomeLabel(row.outcome) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="总耗时" width="92" align="right">
            <template #default="{ row }">{{ durationLabel(row.total_ms) }}</template>
          </el-table-column>
        </el-table>
      </PortalContentCard>
    </div>

    <PortalContentCard
      v-if="detail && detail.request_status !== 'success' && (detail.internal_error_detail || detail.failed_step || detail.error_message)"
      title="失败详情"
      description="用于确认失败阶段和底层错误。"
      class="failure-card"
    >
      <div class="failure-card__summary">
        <AlertTriangle :size="17" />
        <span>失败阶段：{{ detail.failed_step || "网关请求" }}</span>
        <strong>{{ detail.error_message || detail.error_code || "请求失败" }}</strong>
      </div>
      <pre v-if="detail.internal_error_detail">{{ detail.internal_error_detail }}</pre>
    </PortalContentCard>

    <PortalContentCard title="载荷与响应" description="原始 JSON 默认收起，展开后按分区查看并复制。">
      <template #actions>
        <button class="payload-toggle" type="button" @click="payloadOpen = !payloadOpen">
          <Braces :size="15" />
          {{ payloadOpen ? "收起载荷" : "展开载荷" }}
          <ChevronDown :size="15" :class="{ 'is-open': payloadOpen }" />
        </button>
      </template>

      <button v-if="!payloadOpen" class="payload-collapsed" type="button" @click="payloadOpen = true">
        <div class="payload-collapsed__icon"><Braces :size="19" /></div>
        <div>
          <strong>原始载荷已收起</strong>
          <span>{{ payloadPresentCount }} 个分区已记录 · 请求参数、消息、响应和计费 JSON 按需查看</span>
        </div>
        <ChevronDown :size="17" />
      </button>

      <div v-else class="payload-viewer">
        <nav class="payload-tabs" aria-label="载荷分区">
          <button
            v-for="section in payloadSections"
            :key="section.key"
            type="button"
            :class="{ 'is-active': activePayload === section.key }"
            @click="activePayload = section.key"
          >
            <span>{{ section.title }}</span>
            <small>{{ section.present ? section.size : "未记录" }}</small>
          </button>
        </nav>
        <div class="payload-viewer__head">
          <span>{{ activePayloadSection?.title }} · JSON</span>
          <button v-if="activePayloadSection?.present" class="detail-copy-button" type="button" @click="copyActivePayload">
            <Copy :size="14" />复制
          </button>
        </div>
        <pre>{{ activePayloadText }}</pre>
      </div>
    </PortalContentCard>
  </div>
</template>

<style scoped>
.usage-detail {
  display: grid;
  gap: 16px;
  min-height: 240px;
}

.usage-detail-header :deep(.portal-content-card__body--md) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
}

.usage-detail-header__main {
  min-width: 0;
}

.usage-detail-eyebrow {
  margin: 0 0 5px;
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 750;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.usage-detail-header__title-row,
.usage-detail-header__meta,
.usage-detail-header__actions,
.detail-copy-button,
.identity-item__label,
.billing-amount__label,
.route-attempt-toggle,
.payload-toggle,
.failure-card__summary {
  display: flex;
  align-items: center;
}

.usage-detail-header__title-row {
  flex-wrap: wrap;
  gap: 9px;
}

.usage-detail-header h1 {
  margin: 0;
  color: var(--ds-ink);
  font-size: 21px;
  font-weight: 720;
  line-height: 1.3;
  letter-spacing: -0.025em;
}

.usage-detail-header__meta {
  flex-wrap: wrap;
  gap: 7px;
  margin-top: 8px;
  color: var(--ds-muted);
  font-size: 12px;
}

.meta-separator {
  color: var(--ds-line-strong);
}

.usage-detail-header__actions {
  flex: 0 0 auto;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.detail-copy-button,
.payload-toggle,
.route-attempt-toggle {
  gap: 6px;
  border: 0;
  background: transparent;
  color: var(--ds-accent);
  cursor: pointer;
  font: inherit;
  font-size: 12px;
  font-weight: 650;
}

.detail-copy-button:hover,
.payload-toggle:hover,
.route-attempt-toggle:hover {
  color: var(--ds-accent-hover);
}

.identity-grid,
.billing-amount-grid,
.billing-info-grid,
.token-strip,
.usage-detail-columns {
  display: grid;
  gap: 12px;
}

.identity-grid {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.identity-item,
.billing-amount,
.billing-info-block,
.performance-total,
.performance-fact,
.payload-collapsed {
  min-width: 0;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
}

.identity-item {
  display: grid;
  gap: 5px;
  padding: 12px 14px;
}

.identity-item--wide {
  grid-column: span 2;
}

.identity-item--tenant {
  border-color: color-mix(in srgb, var(--ds-accent) 24%, var(--ds-line));
  background: color-mix(in srgb, var(--ds-accent-soft) 48%, var(--ds-panel-muted));
}

.identity-item--user {
  border-color: color-mix(in srgb, var(--ds-info) 20%, var(--ds-line));
}

.identity-item__label {
  gap: 6px;
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 700;
}

.identity-item__label svg {
  color: var(--ds-accent);
}

.identity-item strong,
.billing-facts strong,
.token-strip strong,
.performance-total strong,
.performance-fact strong {
  overflow: hidden;
  color: var(--ds-ink);
  font-size: 13px;
  font-weight: 700;
  line-height: 1.45;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.identity-item small {
  overflow: hidden;
  color: var(--ds-faint);
  font-family: var(--ds-font-mono);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.billing-amount-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.billing-amount {
  display: grid;
  gap: 6px;
  padding: 15px 16px;
}

.billing-amount__label {
  gap: 7px;
  color: var(--ds-muted);
  font-size: 12px;
  font-weight: 700;
}

.billing-amount__label svg {
  color: var(--ds-accent);
}

.billing-amount strong {
  color: var(--ds-ink);
  font-family: var(--ds-font-mono);
  font-size: 23px;
  font-weight: 760;
  letter-spacing: -0.03em;
}

.billing-amount small {
  color: var(--ds-faint);
  font-size: 11px;
  line-height: 1.45;
}

.billing-amount--user {
  border-color: color-mix(in srgb, var(--ds-accent) 28%, var(--ds-line));
  background: color-mix(in srgb, var(--ds-accent-soft) 44%, var(--ds-panel-muted));
}

.billing-amount--user strong {
  color: var(--ds-accent);
}

.billing-amount--tenant {
  border-color: color-mix(in srgb, var(--ds-info) 24%, var(--ds-line));
}

.billing-amount--tenant strong {
  color: var(--ds-info);
}

.billing-info-grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
  margin-top: 12px;
}

.billing-info-block {
  padding: 13px 14px;
}

.billing-info-block__title {
  margin-bottom: 10px;
  color: var(--ds-ink-soft);
  font-size: 12px;
  font-weight: 750;
}

.billing-facts {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 9px 18px;
}

.billing-facts > div,
.token-strip > div,
.performance-total > div {
  display: grid;
  gap: 3px;
  min-width: 0;
}

.billing-facts span,
.token-strip span,
.performance-total span,
.performance-fact span {
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 650;
}

.token-strip {
  grid-template-columns: repeat(6, minmax(0, 1fr));
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--ds-line);
}

.token-strip strong,
.token-strip span,
.performance-total strong {
  font-family: var(--ds-font-mono);
}

.billing-secondary {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 18px;
  margin-top: 12px;
  color: var(--ds-muted);
  font-size: 11px;
}

.billing-error {
  display: flex;
  align-items: flex-start;
  gap: 7px;
  margin-top: 10px;
  color: var(--ds-danger);
  font-size: 11px;
  line-height: 1.5;
}

.usage-detail-columns {
  grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.1fr);
  align-items: start;
}

.performance-total {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) repeat(2, minmax(0, 0.8fr));
  gap: 12px;
  padding: 13px 14px;
}

.performance-total > div:first-child strong {
  color: var(--ds-accent);
  font-size: 20px;
}

.performance-facts {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 9px;
  margin-top: 10px;
}

.performance-fact {
  display: grid;
  grid-template-columns: 16px minmax(0, 1fr) auto;
  align-items: center;
  gap: 7px;
  padding: 9px 10px;
}

.performance-fact svg {
  color: var(--ds-accent);
}

.route-flow {
  display: flex;
  align-items: stretch;
  gap: 0;
}

.route-step {
  position: relative;
  display: grid;
  flex: 1 1 0;
  gap: 4px;
  min-width: 0;
  padding: 0 25px 0 0;
}

.route-step:not(:last-child)::after {
  position: absolute;
  top: 9px;
  right: 9px;
  color: var(--ds-line-strong);
  content: "→";
  font-size: 17px;
  font-weight: 700;
}

.route-step__index {
  display: inline-grid;
  place-items: center;
  width: 20px;
  height: 20px;
  margin-bottom: 4px;
  border-radius: var(--ds-radius-circle);
  background: var(--ds-accent-soft);
  color: var(--ds-accent);
  font-family: var(--ds-font-mono);
  font-size: 11px;
  font-weight: 750;
}

.route-step__title {
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 700;
}

.route-step strong {
  overflow: hidden;
  color: var(--ds-ink);
  font-size: 13px;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.route-step small {
  overflow: hidden;
  color: var(--ds-faint);
  font-size: 11px;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.route-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 7px 16px;
  margin-top: 17px;
  padding-top: 12px;
  border-top: 1px solid var(--ds-line);
  color: var(--ds-muted);
  font-size: 11px;
}

.route-attempt-toggle {
  justify-content: space-between;
  width: 100%;
  margin-top: 13px;
  padding: 10px 0 0;
  border-top: 1px dashed var(--ds-line-strong);
}

.route-attempt-toggle svg,
.payload-toggle svg:last-child,
.payload-collapsed > svg {
  transition: transform 160ms ease;
}

.route-attempt-toggle svg.is-open,
.payload-toggle svg.is-open {
  transform: rotate(180deg);
}

:deep(.attempts-table.el-table) {
  margin-top: 10px;
  --el-table-border-color: var(--ds-line);
  --el-table-header-bg-color: transparent;
}

:deep(.attempts-table .el-table__inner-wrapper::before) {
  display: none;
}

.attempt-result {
  font-size: 12px;
  font-weight: 700;
}

.attempt-result.is-success {
  color: var(--ds-positive);
}

.attempt-result.is-danger {
  color: var(--ds-danger);
}

.failure-card :deep(.portal-content-card__body--md) {
  padding-top: 12px;
}

.failure-card__summary {
  flex-wrap: wrap;
  gap: 8px;
  color: var(--ds-danger);
  font-size: 12px;
}

.failure-card__summary strong {
  color: var(--ds-ink);
  font-weight: 650;
}

.failure-card pre,
.payload-viewer pre {
  max-height: 340px;
  margin: 12px 0 0;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--ds-code-fg);
  font-family: var(--ds-font-mono);
  font-size: 12px;
  line-height: 1.6;
}

.failure-card pre {
  padding: 13px;
  border-radius: var(--ds-radius-control);
  background: var(--ds-code-bg);
}

.payload-toggle {
  padding: 0;
}

.payload-collapsed {
  display: flex;
  align-items: center;
  width: 100%;
  gap: 12px;
  padding: 13px 14px;
  color: var(--ds-ink);
  cursor: pointer;
  text-align: left;
}

.payload-collapsed:hover {
  border-color: var(--ds-accent);
  background: var(--ds-accent-soft);
}

.payload-collapsed__icon {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: var(--ds-radius-control);
  background: var(--ds-accent-soft);
  color: var(--ds-accent);
}

.payload-collapsed > div:nth-child(2) {
  display: grid;
  flex: 1;
  gap: 3px;
  min-width: 0;
}

.payload-collapsed strong {
  font-size: 13px;
}

.payload-collapsed span {
  overflow: hidden;
  color: var(--ds-muted);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.payload-collapsed > svg {
  color: var(--ds-muted);
  transform: rotate(-90deg);
}

.payload-viewer {
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
}

.payload-tabs {
  display: flex;
  overflow-x: auto;
  border-bottom: 1px solid var(--ds-line);
  background: var(--ds-panel-muted);
}

.payload-tabs button {
  display: grid;
  flex: 0 0 auto;
  gap: 2px;
  padding: 10px 13px;
  border: 0;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--ds-muted);
  cursor: pointer;
  font: inherit;
  font-size: 12px;
  text-align: left;
}

.payload-tabs button:hover,
.payload-tabs button.is-active {
  color: var(--ds-accent);
}

.payload-tabs button.is-active {
  border-bottom-color: var(--ds-accent);
  background: var(--ds-panel);
}

.payload-tabs small {
  color: var(--ds-faint);
  font-size: 10px;
}

.payload-viewer__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  background: var(--ds-code-header);
  color: var(--ds-code-accent);
  font-family: var(--ds-font-mono);
  font-size: 11px;
}

.payload-viewer__head .detail-copy-button {
  color: var(--ds-code-accent);
}

.payload-viewer pre {
  max-height: 460px;
  margin: 0;
  padding: 14px;
  background: var(--ds-code-bg);
}

@media (max-width: 1100px) {
  .identity-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .token-strip {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    row-gap: 12px;
  }
}

@media (max-width: 860px) {
  .usage-detail-header :deep(.portal-content-card__body--md),
  .usage-detail-columns {
    display: grid;
    grid-template-columns: 1fr;
  }

  .usage-detail-header__actions {
    justify-content: flex-start;
  }

  .billing-amount-grid,
  .billing-info-grid {
    grid-template-columns: 1fr;
  }

  .route-flow {
    display: grid;
    gap: 13px;
  }

  .route-step {
    padding: 0 0 13px 30px;
  }

  .route-step:not(:last-child)::after {
    top: auto;
    right: auto;
    bottom: -3px;
    left: 5px;
    content: "↓";
  }

  .route-step__index {
    position: absolute;
    top: 0;
    left: 0;
  }
}

@media (max-width: 560px) {
  .identity-grid,
  .billing-facts,
  .performance-facts,
  .token-strip,
  .performance-total {
    grid-template-columns: 1fr;
  }

  .identity-item--wide {
    grid-column: auto;
  }

  .billing-amount strong {
    font-size: 20px;
  }

  .usage-detail-header h1 {
    font-size: 18px;
  }
}
</style>
