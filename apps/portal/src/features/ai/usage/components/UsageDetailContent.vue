<!--
  请求详情内容:由 UsageDetailDrawer 的抽屉内容提取而来,供独立详情页使用。
  与原抽屉的差异:
  - 只依赖详情 DTO(页面按 requestId 直接拉取,不再有列表行上下文),
    租户结算/用户扣款等列表行专有字段不再展示;
  - 版面按宽页排版(左主右辅双栏),不再按抽屉窄栏压缩;
  - 去掉了上一条/下一条导航(页面形态下由面包屑返回列表);
  - el-table 边框色等硬编码 hex 已 token 化。
-->
<script setup lang="ts">
import { computed } from "vue";
import { CopyDocument } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import {
  PortalContentCard,
  PortalDetailLayout
} from "@dai/app-core";
import { formatMultiplier } from "@dai/app-core/ai/utils";
import {
  formatMs,
  UsageTag,
  UsageTimingCell,
  UsageTokenCell
} from "@dai/app-core/ai/usage";

import { normalizeUsageAttempts, type UsageAttemptDetail, type UsageLogDetailDTO } from "../model";
import {
  formatJSON,
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

const resolvedRequestId = computed(() => props.detail?.request_id || "");
const resolvedStatus = computed(() => props.detail?.request_status || "");
const resolvedTrace = computed(() => props.detail?.trace_id || "—");
const resolvedHeadline = computed(() => {
  const requested = props.detail?.requested_model || "—";
  const resolved = props.detail?.resolved_logical_model || requested;
  return requested === resolved ? requested : `${requested} → ${resolved}`;
});

function multiplierLabel(value?: number | null) {
  return value == null ? "—" : `x${formatMultiplier(value)}`;
}

const summaryFacts = computed(() => {
  const facts = [
    { label: "租户", value: props.detail?.tenant_id || "—" },
    { label: "分组", value: props.detail?.group_name_snapshot || "—" },
    { label: "计费分组", value: props.detail?.billing_group_label_snapshot || "—" }
  ];
  if (props.detail?.resolution) {
    facts.push({ label: "图片/视频规格", value: props.detail.resolution });
  }
  return facts;
});

const routeSteps = computed(() => [
  {
    title: "客户端请求模型",
    value: props.detail?.requested_model || "—",
    hint: `入口格式：${props.detail?.client_api_format || "—"}`
  },
  {
    title: "售价计费模型",
    value: props.detail?.resolved_logical_model || "—",
    hint: props.detail?.matched_dispatch_rule_summary || "未命中规则，原样路由"
  },
  {
    title: "成本计费模型",
    value: props.detail?.selected_upstream_model || "—",
    hint: `上游目标：${props.detail?.resolved_provider_family || "不限制"}`
  },
  {
    title: "客户端响应模型",
    value: props.detail?.public_response_model || "—",
    hint: `上游格式：${props.detail?.provider_api_format || "—"}`
  }
]);

const payloadSections = computed(() => [
  { key: "request_params", title: "请求参数", value: props.detail?.request_params },
  { key: "request_messages", title: "请求消息", value: props.detail?.request_messages },
  { key: "response_message", title: "响应摘要", value: props.detail?.response_message },
  { key: "media_refs", title: "媒体引用", value: props.detail?.media_refs },
  { key: "billing_breakdown", title: "计费明细", value: props.detail?.billing_breakdown }
]);

const attempts = computed<UsageAttemptDetail[]>(() => normalizeUsageAttempts(props.detail?.attempts_detail));
const resolvedTimingSource = computed(() => ({
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
  totalMs: resolveRequestTotalMs(resolvedTimingSource.value) || null,
  firstResponseMs: resolveFirstResponseByteMs(resolvedTimingSource.value) || null,
  headerMs: resolveHeaderMs(resolvedTimingSource.value) || null
}));
const timingFacts = computed(() =>
  [
    { label: "请求准备", value: formatTimeValue(resolveRequestSetupMs(resolvedTimingSource.value)), tone: "neutral" },
    { label: "最终连接", value: formatTimeValue(resolveHeaderMs(resolvedTimingSource.value)), tone: "info" },
    { label: "首响后尾程", value: formatTimeValue(resolveResponseTailMs(resolvedTimingSource.value)), tone: "accent" },
    {
      label: "最终尝试",
      value: formatTimeValue(Number(resolvedTimingSource.value.final_attempt_total_ms ?? 0) || 0),
      tone: "warning"
    }
  ].filter((fact) => fact.value !== "—")
);

interface BillingPriceLineSnapshot {
  input_context_tokens?: number;
  token_price_tier_index?: number;
  token_price_tier_up_to_input_tokens?: number | null;
}

interface BillingCostLineSnapshot {
  price_lines?: BillingPriceLineSnapshot;
}

// 对应后端 billingBreakdownSnapshot（v4）。这是 JSON.parse 出来的不透明 blob，
// 类型检查不会在后端改键名时报错，所以字段名与 pricebook_billing.go 的 json tag 必须手工对齐。
interface BillingBreakdownSnapshot {
  version?: number;
  /** 目录基准价（倍率 1），谁都不付，只作基数。 */
  catalog_base?: BillingCostLineSnapshot;
  /** 租户应付平台。 */
  tenant_payable?: BillingCostLineSnapshot;
  /** 用户应付租户；租户自有 key 的请求没有这一段。 */
  user_payable?: BillingCostLineSnapshot;
  price_lines?: BillingPriceLineSnapshot;
}

const billingContext = computed(() => {
  const raw = props.detail?.billing_breakdown;
  let value: BillingBreakdownSnapshot | null = null;
  if (typeof raw === "string") {
    try { value = JSON.parse(raw) as BillingBreakdownSnapshot; } catch { return null; }
  } else if (raw && typeof raw === "object") {
    value = raw as BillingBreakdownSnapshot;
  }
  if (!value || Number(value.version) < 2) return null;
  const sell = value.user_payable?.price_lines || value.price_lines;
  const provider = value.catalog_base?.price_lines;
  return {
    inputTokens: sell?.input_context_tokens,
    sellTier: formatContextTier(sell),
    providerTier: formatContextTier(provider)
  };
});

async function copyText(value: string) {
  if (!value || value === "—") return;
  try {
    await navigator.clipboard.writeText(value);
    ElMessage.success("已复制");
  } catch {
    ElMessage.error("复制失败");
  }
}

function formatTimeValue(value?: number | null) {
  return formatMs(value ?? null);
}

function formatContextTier(line: BillingPriceLineSnapshot | undefined) {
  if (!line || line.token_price_tier_index == null) return "—";
  const upper = line.token_price_tier_up_to_input_tokens == null
    ? "无上限"
    : `≤ ${Number(line.token_price_tier_up_to_input_tokens).toLocaleString("zh-CN")}`;
  return `档位 ${Number(line.token_price_tier_index) + 1} · ${upper}`;
}
</script>

<template>
  <div class="usage-detail" v-loading="loading">
    <PortalDetailLayout summary-width="320px">
      <template #summary>
        <PortalContentCard class="usage-detail-card">
          <div class="usage-detail-hero">
            <div class="usage-detail-hero__header">
              <div>
                <p class="usage-detail-eyebrow">请求身份</p>
                <h3 class="usage-detail-title">{{ resolvedHeadline }}</h3>
              </div>
              <el-button
                v-if="resolvedRequestId"
                link
                type="primary"
                :icon="CopyDocument"
                @click="copyText(resolvedRequestId)"
              >
                复制 ID
              </el-button>
            </div>

            <div class="usage-detail-tags">
              <UsageTag kind="status" :value="resolvedStatus" />
              <UsageTag v-if="detail" kind="stream" :value="detail.stream" />
              <UsageTag v-if="detail?.reasoning_effort" kind="effort" :value="detail.reasoning_effort" />
            </div>

            <div class="usage-detail-facts">
              <article v-for="fact in summaryFacts" :key="fact.label" class="usage-detail-fact">
                <span>{{ fact.label }}</span>
                <strong>{{ fact.value }}</strong>
              </article>
            </div>
          </div>
        </PortalContentCard>

        <PortalContentCard title="执行剖面" description="总耗时是主指标，连接、首响与尾程用于拆慢点。">
          <div class="usage-detail-snapshot">
            <article class="usage-detail-snapshot__item">
              <span>Token</span>
              <UsageTokenCell
                :prompt="detail?.prompt_tokens ?? 0"
                :completion="detail?.completion_tokens ?? 0"
                :cache-read="detail?.cache_read_tokens ?? 0"
                :cache-write="detail?.cache_write_tokens ?? 0"
                :reasoning="detail?.reasoning_tokens ?? 0"
              />
            </article>
            <article class="usage-detail-snapshot__item">
              <span>总耗时</span>
              <UsageTimingCell
                :total-ms="timingSummary.totalMs"
                :first-response-byte-ms="timingSummary.firstResponseMs"
                :header-ms="timingSummary.headerMs"
              />
            </article>
            <article class="usage-detail-snapshot__item">
              <span>尝试次数</span>
              <strong>{{ detail?.attempts_count ?? "—" }}</strong>
            </article>
          </div>
          <div v-if="timingFacts.length" class="usage-detail-timing">
            <article
              v-for="fact in timingFacts"
              :key="fact.label"
              class="usage-detail-timing__item"
              :class="`usage-detail-timing__item--${fact.tone}`"
            >
              <span>{{ fact.label }}</span>
              <strong>{{ fact.value }}</strong>
            </article>
          </div>
        </PortalContentCard>

        <PortalContentCard title="计费档位">
          <div class="usage-detail-kv">
            <div>
              <span>服务档位</span>
              <strong>{{ detail?.service_tier || "—" }}</strong>
            </div>
            <div>
              <span>用户有效倍率</span>
              <strong>{{ multiplierLabel(detail?.effective_user_multiplier_snapshot) }}</strong>
            </div>
            <div>
              <span>分组用户默认倍率</span>
              <strong>{{ multiplierLabel(detail?.group_default_user_multiplier_snapshot) }}</strong>
            </div>
            <div>
              <span>输入上下文</span>
              <strong>{{ billingContext?.inputTokens?.toLocaleString("zh-CN") || "—" }}</strong>
            </div>
            <div>
              <span>零售价上下文档</span>
              <strong>{{ billingContext?.sellTier || "—" }}</strong>
            </div>
            <div>
              <span>账号价格上下文档</span>
              <strong>{{ billingContext?.providerTier || "—" }}</strong>
            </div>
            <div>
              <span>用户覆盖</span>
              <strong>{{ multiplierLabel(detail?.user_multiplier_override_snapshot) }}</strong>
            </div>
          </div>
        </PortalContentCard>
      </template>

      <div class="usage-detail-main">
        <PortalContentCard title="总览" description="把排障最常看的字段收敛到一块，不再靠 descriptions 全铺开。">
          <div class="usage-detail-overview">
            <div class="usage-detail-overview__grid">
              <article class="usage-detail-overview__item">
                <span>Trace ID</span>
                <strong>{{ resolvedTrace }}</strong>
              </article>
              <article class="usage-detail-overview__item">
                <span>HTTP / 上游状态</span>
                <strong>{{ detail?.http_status ?? "—" }} / {{ detail?.upstream_status ?? "—" }}</strong>
              </article>
              <article class="usage-detail-overview__item">
                <span>请求路径</span>
                <strong>{{ detail?.request_path || "—" }}</strong>
              </article>
              <article class="usage-detail-overview__item">
                <span>客户端 IP</span>
                <strong>{{ detail?.client_ip || "—" }}</strong>
              </article>
              <article class="usage-detail-overview__item usage-detail-overview__item--wide">
                <span>User-Agent</span>
                <strong>{{ detail?.user_agent || "—" }}</strong>
              </article>
              <article class="usage-detail-overview__item usage-detail-overview__item--wide">
                <span>错误信息</span>
                <strong>{{ detail?.error_message || detail?.error_code || "—" }}</strong>
              </article>
            </div>
          </div>
        </PortalContentCard>

        <PortalContentCard title="路由链路" description="请求、售价、成本与响应模型身份。">
          <div class="route-timeline">
            <article v-for="step in routeSteps" :key="step.title" class="route-timeline__item">
              <span class="route-timeline__dot"></span>
              <div class="route-timeline__copy">
                <strong>{{ step.title }}</strong>
                <span>{{ step.value }}</span>
                <small>{{ step.hint }}</small>
              </div>
            </article>
          </div>
        </PortalContentCard>

        <PortalContentCard title="载荷与响应" description="JSON 不再散落在弹框底部，统一收进结构化分区。">
          <div class="payload-grid">
            <section v-for="section in payloadSections" :key="section.key" class="payload-panel">
              <header class="payload-panel__head">
                <h4>{{ section.title }}</h4>
                <el-button
                  v-if="section.value != null"
                  link
                  size="small"
                  :icon="CopyDocument"
                  @click="copyText(formatJSON(section.value))"
                >
                  复制
                </el-button>
              </header>
              <pre>{{ formatJSON(section.value) }}</pre>
            </section>
          </div>
        </PortalContentCard>

        <PortalContentCard
          v-if="detail && detail.request_status !== 'success' && (detail.internal_error_detail || detail.failed_step)"
          title="失败详情"
          description="仅管理员可见，用于确认失败阶段和底层原始错误。"
        >
          <div class="failure-card">
            <div class="failure-card__meta">
              <span>失败阶段</span>
              <strong>{{ detail.failed_step || "—" }}</strong>
            </div>
            <pre>{{ detail.internal_error_detail || "—" }}</pre>
          </div>
        </PortalContentCard>

        <PortalContentCard
          v-if="attempts.length"
          title="重试链路"
          :description="`本次请求共 ${attempts.length} 次候选尝试，按发生顺序展开。`"
        >
          <el-table :data="attempts" size="small" class="attempts-table">
            <el-table-column label="#" type="index" width="44" />
            <el-table-column label="供应商" prop="provider_code" min-width="120">
              <template #default="{ row }">{{ row.provider_code || "—" }}</template>
            </el-table-column>
            <el-table-column label="上游模型" prop="upstream_model" min-width="180" show-overflow-tooltip>
              <template #default="{ row }">{{ row.upstream_model || "—" }}</template>
            </el-table-column>
            <el-table-column label="HTTP" width="80" align="right">
              <template #default="{ row }">{{ row.http_status ?? "—" }}</template>
            </el-table-column>
            <el-table-column label="结果" width="110">
              <template #default="{ row }">
                <el-tag size="small" :type="row.outcome === 'success' ? 'success' : 'danger'">{{ row.outcome }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="连接" width="94" align="right">
              <template #default="{ row }">{{ row.latency_ms != null ? `${row.latency_ms} ms` : "—" }}</template>
            </el-table-column>
            <el-table-column label="首响" width="94" align="right">
              <template #default="{ row }">{{ row.first_byte_ms != null ? `${row.first_byte_ms} ms` : "—" }}</template>
            </el-table-column>
            <el-table-column label="总耗时" width="98" align="right">
              <template #default="{ row }">{{ row.total_ms != null ? `${row.total_ms} ms` : "—" }}</template>
            </el-table-column>
            <el-table-column label="错误" min-width="240" show-overflow-tooltip>
              <template #default="{ row }">{{ row.error || "—" }}</template>
            </el-table-column>
          </el-table>
        </PortalContentCard>
      </div>
    </PortalDetailLayout>
  </div>
</template>

<style scoped>
.usage-detail {
  min-height: 240px;
}

.usage-detail-hero,
.usage-detail-main {
  display: grid;
  gap: 16px;
}

.usage-detail-hero__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.usage-detail-eyebrow {
  margin: 0 0 4px;
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.usage-detail-title {
  margin: 0;
  color: var(--ds-ink);
  font-size: 18px;
  line-height: 1.35;
}

.usage-detail-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.usage-detail-facts,
.usage-detail-overview__grid,
.usage-detail-kv {
  display: grid;
  gap: 10px;
}

.usage-detail-facts {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.usage-detail-fact,
.usage-detail-overview__item,
.usage-detail-kv > div,
.usage-detail-snapshot__item {
  display: grid;
  gap: 4px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
  padding: 10px 12px;
}

.usage-detail-fact span,
.usage-detail-overview__item span,
.usage-detail-kv span,
.usage-detail-snapshot__item span,
.failure-card__meta span {
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 700;
}

.usage-detail-fact strong,
.usage-detail-overview__item strong,
.usage-detail-kv strong,
.usage-detail-snapshot__item strong,
.failure-card__meta strong {
  color: var(--ds-ink);
  font-size: 13px;
  line-height: 1.45;
  word-break: break-word;
}

.usage-detail-snapshot {
  display: grid;
  gap: 10px;
}

.usage-detail-timing {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin-top: 10px;
}

.usage-detail-timing__item {
  display: grid;
  gap: 4px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
  padding: 10px 12px;
}

.usage-detail-timing__item span {
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 700;
}

.usage-detail-timing__item strong {
  color: var(--ds-ink);
  font-size: 13px;
  line-height: 1.45;
}

.usage-detail-timing__item--info {
  border-color: color-mix(in srgb, var(--ds-info) 18%, var(--ds-info-soft));
}

.usage-detail-timing__item--info strong {
  color: var(--ds-info);
}

.usage-detail-timing__item--accent {
  border-color: color-mix(in srgb, var(--ds-accent) 20%, var(--ds-accent-soft));
}

.usage-detail-timing__item--accent strong {
  color: var(--ds-accent);
}

.usage-detail-timing__item--warning {
  border-color: color-mix(in srgb, var(--ds-warning) 22%, var(--ds-warning-soft));
}

.usage-detail-timing__item--warning strong {
  color: var(--ds-warning);
}

.usage-detail-kv {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.usage-detail-overview__grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.usage-detail-overview__item--wide {
  grid-column: span 2;
}

.route-timeline {
  display: grid;
  gap: 14px;
}

.route-timeline__item {
  display: grid;
  grid-template-columns: 18px minmax(0, 1fr);
  gap: 12px;
}

.route-timeline__dot {
  width: 10px;
  height: 10px;
  margin-top: 6px;
  border-radius: 999px;
  background: var(--ds-accent);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--ds-accent-soft) 80%, transparent);
}

.route-timeline__copy {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.route-timeline__copy strong {
  color: var(--ds-ink);
  font-size: 13px;
}

.route-timeline__copy span {
  color: var(--ds-ink-soft);
  font-size: 13px;
  line-height: 1.45;
}

.route-timeline__copy small {
  color: var(--ds-faint);
  font-size: 12px;
}

.payload-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.payload-panel {
  display: grid;
  gap: 8px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
  padding: 12px;
  min-width: 0;
}

.payload-panel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.payload-panel__head h4 {
  margin: 0;
  color: var(--ds-ink);
  font-size: 13px;
  font-weight: 700;
}

.payload-panel pre,
.failure-card pre {
  margin: 0;
  max-height: 280px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--ds-ink-soft);
  font-size: 12px;
  line-height: 1.55;
}

.failure-card {
  display: grid;
  gap: 12px;
}

.failure-card__meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

:deep(.attempts-table.el-table) {
  --el-table-border-color: var(--ds-line);
  --el-table-header-bg-color: transparent;
}

:deep(.attempts-table .el-table__inner-wrapper::before) {
  display: none;
}

@media (max-width: 960px) {
  .usage-detail-facts,
  .usage-detail-kv,
  .usage-detail-timing,
  .usage-detail-overview__grid,
  .payload-grid {
    grid-template-columns: 1fr;
  }

  .usage-detail-overview__item--wide {
    grid-column: auto;
  }
}
</style>
