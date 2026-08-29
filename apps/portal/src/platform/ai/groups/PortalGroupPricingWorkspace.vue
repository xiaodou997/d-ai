<!--
  门户「模型定价」工作台：按分组查看可见模型与生效价格（当前仅用户端 GroupsView 使用）。
  重构：迁移至 DsUI 一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       页头 props 由 eyebrow/title 换成 icon/breadcrumbs);卡片配色全部由 --ds-* token
       派生(color-mix),去掉硬编码 hex;业务逻辑与请求不变。
-->
<script setup lang="ts">
import { computed, onMounted, shallowRef, type Component } from "vue";

import PortalContentCard from "../../page/PortalContentCard.vue";
import PortalPagePanel, { type PortalPagePanelBreadcrumb } from "../../page/PortalPagePanel.vue";
import { formatMultiplier as formatMultiplierValue } from "../utils";
import type {
  PortalGroupEffectivePriceRecord,
  PortalGroupPricingApi,
  PortalVisibleGroupRecord
} from "./types";

const props = withDefaults(
  defineProps<{
    api: PortalGroupPricingApi;
    /** 页头身份图标(lucide 组件) */
    icon?: Component;
    /** 面包屑路径,末级即页面标题 */
    breadcrumbs: PortalPagePanelBreadcrumb[];
    description: string;
    emptyHint?: string;
    refreshIcon?: unknown;
    capabilityOptions?: Array<{ label: string; value: string }>;
    notifyError?: (message: string) => void;
  }>(),
  {
    icon: undefined,
    emptyHint: "当前账号还没有开放分组，请联系管理员处理。",
    capabilityOptions: () => []
  }
);

type PriceLineTone = "input" | "output" | "default" | "resolution" | "cache" | "audio";

interface PricingDisplayLine {
  label: string;
  usd: string;
  tone: PriceLineTone;
}

interface PricingDisplaySection {
  key: string;
  title: string;
  lines: PricingDisplayLine[];
  emptyText?: string;
}

interface PricingDisplayCard {
  key: string;
  modelCode: string;
  capabilityName: string;
  capabilityTheme: "token" | "image" | "video" | "audio";
  sections: PricingDisplaySection[];
}

const groupsLoading = shallowRef(false);
const pricesLoading = shallowRef(false);
const errorMessage = shallowRef("");
const groups = shallowRef<PortalVisibleGroupRecord[]>([]);
const selectedGroup = shallowRef<PortalVisibleGroupRecord | null>(null);
const prices = shallowRef<PortalGroupEffectivePriceRecord[]>([]);
const priceRequestId = shallowRef(0);

const showErrorBanner = computed(() => Boolean(errorMessage.value));
const modelCountLabel = computed(() => `${prices.value.length} 个模型`);
const pricingEmptyText = computed(() => {
  if (groupsLoading.value || pricesLoading.value) return "正在加载，请稍候...";
  if (errorMessage.value) return errorMessage.value;
  if (!selectedGroup.value) return props.emptyHint;
  return "该分组暂无可用模型或价格条目";
});
const pricingHint = computed(() => {
  if (!selectedGroup.value) return "";
  return "缓存写入和读取按命中档位的对应价格结算。";
});

function capabilityLabel(value: string) {
  return props.capabilityOptions.find((item) => item.value === value)?.label || value || "-";
}

function capabilityTheme(value: string): "token" | "image" | "video" | "audio" {
  if (value === "image") return "image";
  if (value === "video") return "video";
  if (value === "audio_tts" || value === "audio_stt") return "audio";
  return "token";
}

function formatMultiplier(value?: number | null) {
  return `x${formatMultiplierValue(value)}`;
}

function isResolutionPriced(row: Pick<PortalGroupEffectivePriceRecord, "capability_type">) {
  return row.capability_type === "image" || row.capability_type === "video";
}

function isTokenPriced(row: Pick<PortalGroupEffectivePriceRecord, "capability_type">) {
  return !["image", "video", "audio_tts", "audio_stt"].includes(row.capability_type);
}

function formatUSD(value: number) {
  const n = Number(value) || 0;
  return `$${n.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 6 })}`;
}

function pricePair(
  usdValue: number | null | undefined,
  unit: string,
  tone: PriceLineTone,
  label: string
): PricingDisplayLine {
  return { label, usd: `${formatUSD(Number(usdValue) || 0)}${unit}`, tone };
}

function standardSection(row: PortalGroupEffectivePriceRecord): PricingDisplaySection {
  if (row.capability_type === "image") {
    return { key: "standard", title: "默认价格", lines: [pricePair(row.image_default_price_usd, "/张", "default", "默认")] };
  }
  if (row.capability_type === "video") {
    return { key: "standard", title: "默认价格", lines: [pricePair(row.video_default_price_usd, "/秒", "default", "默认")] };
  }
  if (row.capability_type === "audio_tts") {
    return { key: "standard", title: "合成价格", lines: [pricePair(row.audio_tts_per_1m_chars_usd, "/1M字符", "audio", "合成")] };
  }
  if (row.capability_type === "audio_stt") {
    return { key: "standard", title: "识别价格", lines: [pricePair(row.audio_stt_per_minute_usd, "/分钟", "audio", "识别")] };
  }
  return { key: "standard", title: "Token 价格", lines: [] };
}

// 仅图像/视频有规格覆盖；对话/音频模型不渲染此面板。
function overrideSection(row: PortalGroupEffectivePriceRecord): PricingDisplaySection | null {
  if (!isResolutionPriced(row)) return null;
  const items = row.capability_type === "image" ? row.image_prices || [] : row.video_prices || [];
  const unit = row.capability_type === "image" ? "/张" : "/秒";
  return {
    key: "override",
    title: "规格覆盖",
    lines: items.map((item) => pricePair(item.price, unit, "resolution", item.resolution)),
    emptyText: "无规格覆盖"
  };
}

function tokenSections(row: PortalGroupEffectivePriceRecord): PricingDisplaySection[] {
  return (row.token_price_tiers || []).map((tier, index) => {
    const upper = tier.up_to_input_tokens == null ? "无上限" : `≤ ${tier.up_to_input_tokens.toLocaleString("zh-CN")}`;
    const lines = [
      pricePair(tier.input_per_1m_usd, "/1M", "input", "输入"),
      pricePair(tier.output_per_1m_usd, "/1M", "output", "输出")
    ];
    lines.push(
      pricePair(tier.cache_write_per_1m_usd, "/1M", "cache", "缓存写入"),
      pricePair(tier.cache_read_per_1m_usd, "/1M", "cache", "缓存读取")
    );
    return { key: `tier-${index}`, title: `输入上下文 ${upper}`, lines };
  });
}

function buildSections(row: PortalGroupEffectivePriceRecord): PricingDisplaySection[] {
  if (isTokenPriced(row)) return tokenSections(row);
  const sections: PricingDisplaySection[] = [standardSection(row)];
  const override = overrideSection(row);
  if (override) sections.push(override);
  return sections;
}

const displayCards = computed<PricingDisplayCard[]>(() =>
  prices.value.map((row) => ({
    key: `${row.model_code}::${row.capability_type}`,
    modelCode: row.model_code,
    capabilityName: capabilityLabel(row.capability_type),
    capabilityTheme: capabilityTheme(row.capability_type),
    sections: buildSections(row)
  }))
);

async function loadPrices(groupId: string) {
  const requestId = priceRequestId.value + 1;
  priceRequestId.value = requestId;
  pricesLoading.value = true;
  errorMessage.value = "";
  try {
    const res = await props.api.getGroupEffectivePrices(groupId);
    if (requestId !== priceRequestId.value) return;
    prices.value = res.items || [];
  } catch (error) {
    if (requestId !== priceRequestId.value) return;
    prices.value = [];
    errorMessage.value = (error as Error).message || "加载分组价格失败，请稍后重试。";
    props.notifyError?.(errorMessage.value);
  } finally {
    if (requestId === priceRequestId.value) {
      pricesLoading.value = false;
    }
  }
}

function selectGroup(group: PortalVisibleGroupRecord) {
  selectedGroup.value = group;
  void loadPrices(group.id);
}

async function loadGroups() {
  groupsLoading.value = true;
  errorMessage.value = "";
  try {
    const res = await props.api.listGroups();
    groups.value = res.items || [];
    if (groups.value.length === 0) {
      selectedGroup.value = null;
      prices.value = [];
      return;
    }
    const next = groups.value.find((group) => group.id === selectedGroup.value?.id) || groups.value[0];
    selectGroup(next);
  } catch (error) {
    groups.value = [];
    selectedGroup.value = null;
    prices.value = [];
    errorMessage.value = (error as Error).message || "加载分组失败，请稍后重试。";
    props.notifyError?.(errorMessage.value);
  } finally {
    groupsLoading.value = false;
  }
}

onMounted(loadGroups);
</script>

<template>
  <div class="page-container group-pricing-page">
    <PortalPagePanel fill :icon="icon" :breadcrumbs="breadcrumbs" :description="description">
      <template #actions>
        <slot name="actions" />
        <el-button :icon="refreshIcon" :loading="groupsLoading || pricesLoading" @click="loadGroups">刷新</el-button>
      </template>

      <!-- 面板 body 无内边距,用 24px 容器承载错误横幅与定价工作区 -->
      <div class="pricing-body">
        <el-alert v-if="showErrorBanner" type="danger" :closable="false" show-icon class="group-error-alert">
          <template #title>
            <div class="alert-content">
              <div>
                <strong>分组信息暂时不可用</strong>
                <p class="alert-text">{{ errorMessage }}</p>
              </div>
              <el-button plain @click="loadGroups">重试</el-button>
            </div>
          </template>
        </el-alert>

        <main class="pricing-workspace">
      <PortalContentCard title="分组" body-padding="none" class="group-list-card">
        <div v-loading="groupsLoading" class="group-list">
          <button
            v-for="group in groups"
            :key="group.id"
            type="button"
            class="group-option"
            :class="{ 'group-option--selected': selectedGroup?.id === group.id }"
            @click="selectGroup(group)"
          >
            <span class="group-option__name">{{ group.name }}</span>
            <span class="group-option__meta">
              <span class="group-option__multiplier">{{ formatMultiplier(group.effective_user_multiplier) }}</span>
            </span>
          </button>
          <div v-if="!groupsLoading && groups.length === 0" class="group-list__empty">{{ emptyHint }}</div>
        </div>
      </PortalContentCard>

      <PortalContentCard class="pricing-card">
        <template #header>
          <div class="panel-copy">
            <span class="card-title">可用模型与生效价格{{ selectedGroup ? ` · ${selectedGroup.name}` : "" }}</span>
            <span v-if="pricingHint" class="hint">{{ pricingHint }}</span>
          </div>
        </template>
        <template #actions>
          <span class="model-count">{{ modelCountLabel }}</span>
        </template>

        <div v-loading="pricesLoading" class="pricing-surface">
          <div v-if="displayCards.length" class="model-grid">
            <article
              v-for="card in displayCards"
              :key="card.key"
              class="model-card"
              :class="`model-card--${card.capabilityTheme}`"
            >
              <header class="model-card__header">
                <h3 class="model-card__title">{{ card.modelCode }}</h3>
                <span class="capability-badge" :class="`capability-badge--${card.capabilityTheme}`">{{ card.capabilityName }}</span>
              </header>

              <div class="model-card__body">
                <section v-for="section in card.sections" :key="`${card.key}-${section.key}`" class="pricing-panel">
                  <div class="pricing-panel__header">
                    <span class="pricing-panel__title">{{ section.title }}</span>
                  </div>
                  <div v-if="section.lines.length" class="pricing-panel__lines">
                    <div v-for="line in section.lines" :key="`${card.key}-${section.key}-${line.label}`" class="metric-row">
                      <span class="metric-row__label">{{ line.label }}</span>
                      <span class="metric-row__value-block">
                        <strong class="metric-row__usd" :class="`metric-row__usd--${line.tone}`">{{ line.usd }}</strong>
                      </span>
                    </div>
                  </div>
                  <p v-else class="pricing-panel__empty">{{ section.emptyText || "此能力不适用" }}</p>
                </section>
              </div>
            </article>
          </div>

          <div v-else class="pricing-empty">
            <span class="table-empty">{{ pricingEmptyText }}</span>
            <span v-if="selectedGroup && !errorMessage" class="hint">选择其他分组或稍后刷新重试。</span>
          </div>
        </div>
      </PortalContentCard>
        </main>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.group-pricing-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.pricing-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 24px;
}

.group-error-alert {
  border-radius: var(--ds-radius-panel);
  border: 1px solid color-mix(in srgb, var(--ds-danger) 22%, transparent);
  background: var(--ds-danger-soft);
  padding: 14px 16px;
}

:deep(.group-error-alert .el-alert__icon) {
  color: var(--ds-danger);
}

.alert-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.alert-content strong {
  color: var(--ds-danger);
}

.alert-text {
  margin: 4px 0 0;
  color: var(--ds-muted);
  font-size: 13px;
}

.pricing-workspace {
  display: grid;
  grid-template-columns: minmax(240px, 292px) minmax(0, 1fr);
  align-items: stretch;
  gap: 20px;
  flex: 1;
  min-height: 0;
}

/* 两张卡片随 grid 行拉伸,内部 flex 列让列表/定价区吃掉剩余高度 */
.group-list-card,
.pricing-card {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.group-list-card :deep(.portal-content-card__body),
.pricing-card :deep(.portal-content-card__body) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.group-list {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 120px;
  overflow-y: auto;
}

.group-option {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 10px;
  width: 100%;
  padding: 14px 16px;
  border: 0;
  border-bottom: 1px solid var(--ds-line);
  background: transparent;
  color: inherit;
  cursor: pointer;
  text-align: left;
}

.group-option:last-child {
  border-bottom: 0;
}

.group-option:hover {
  background: var(--ds-panel-muted);
}

.group-option--selected {
  background: var(--ds-accent-soft);
  box-shadow: var(--ds-shadow-inset-accent-wide);
}

.group-option__name {
  overflow: hidden;
  color: var(--ds-ink);
  font-size: 14px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.group-option__meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.group-option__multiplier {
  color: var(--ds-ink-soft);
  font-family: var(--ds-font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  font-weight: 700;
}

.group-list__empty {
  padding: 24px 16px;
  color: var(--ds-faint);
  font-size: 12px;
  line-height: 1.6;
}

.card-title {
  font-weight: 700;
  color: var(--ds-ink);
}

.panel-copy {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.hint,
.table-empty {
  color: var(--ds-faint);
  font-size: 12px;
}

.model-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 5px 9px;
  border-radius: var(--ds-radius-pill);
  background: var(--ds-accent-soft);
  color: var(--ds-ink);
  font-size: 10px;
  font-weight: 800;
}

.rate-chip {
  display: inline-flex;
  align-items: center;
  padding: 5px 10px;
  border-radius: var(--ds-radius-pill);
  border: 1px solid color-mix(in srgb, var(--ds-accent) 24%, transparent);
  background: var(--ds-accent-soft);
  color: var(--ds-accent);
  font-size: 12px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.pricing-surface {
  flex: 1;
  min-height: 120px;
  display: flex;
  flex-direction: column;
}

.model-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.model-card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.model-card::after {
  content: "";
  position: absolute;
  inset: 0;
  border-radius: var(--ds-radius-inherit);
  pointer-events: none;
  background: linear-gradient(135deg, color-mix(in srgb, var(--ds-panel) 10%, transparent), transparent 55%);
}

/* 能力主题色统一由 --ds-* 语义 token 派生:token→info,image→warning,video→danger,audio→accent */
.model-card--token {
  border-color: color-mix(in srgb, var(--ds-info) 28%, var(--ds-line));
}

.model-card--image {
  border-color: color-mix(in srgb, var(--ds-warning) 30%, var(--ds-line));
  background: color-mix(in srgb, var(--ds-warning-soft) 55%, var(--ds-panel));
}

.model-card--video {
  border-color: color-mix(in srgb, var(--ds-danger) 26%, var(--ds-line));
  background: color-mix(in srgb, var(--ds-danger-soft) 55%, var(--ds-panel));
}

.model-card--audio {
  border-color: color-mix(in srgb, var(--ds-accent) 26%, var(--ds-line));
  background: color-mix(in srgb, var(--ds-accent-soft) 55%, var(--ds-panel));
}

.model-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.model-card__title {
  margin: 0;
  min-width: 0;
  flex: 1;
  color: var(--ds-ink);
  font-size: 14px;
  font-weight: 700;
  line-height: 1.25;
  word-break: break-word;
}

.capability-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 4px 8px;
  border-radius: var(--ds-radius-pill);
  font-size: 10px;
  font-weight: 800;
  white-space: nowrap;
}

.capability-badge--token {
  color: var(--ds-info);
  background: var(--ds-info-soft);
}

.capability-badge--image {
  color: var(--ds-warning);
  background: var(--ds-warning-soft);
}

.capability-badge--video {
  color: var(--ds-danger);
  background: var(--ds-danger-soft);
}

.capability-badge--audio {
  color: var(--ds-accent);
  background: var(--ds-accent-soft);
}

/* 面板数量随能力类型变化（对话=1，图像/开缓存=2），auto-fit 让单面板占满、双面板并排 */
.model-card__body {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 8px;
}

.pricing-panel {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px 10px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
}

.pricing-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.pricing-panel__title {
  color: var(--ds-ink-soft);
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.pricing-panel__lines {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.metric-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
}

.metric-row__label {
  color: var(--ds-muted);
  font-size: 11px;
  white-space: nowrap;
}

.metric-row__value-block {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
  text-align: right;
}

.metric-row__usd {
  font-weight: 700;
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}

.metric-row__credits {
  color: var(--ds-faint);
  font-size: 10px;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

/* 单一 USD 价格主显 */
.metric-row__credits--solo {
  color: var(--ds-ink);
  font-size: 13px;
  font-weight: 700;
}

.metric-row__usd--input {
  color: var(--ds-info);
}

.metric-row__usd--output {
  color: var(--ds-warning);
}

.metric-row__usd--default {
  color: var(--ds-positive);
}

.metric-row__usd--resolution {
  color: var(--ds-accent);
}

.metric-row__usd--cache {
  color: var(--ds-positive);
}

.metric-row__usd--reasoning {
  color: var(--ds-danger);
}

.metric-row__usd--audio {
  color: var(--ds-accent-hover);
}

.pricing-panel__empty {
  margin: auto 0;
  color: var(--ds-faint);
  font-size: 11px;
}

.pricing-empty {
  display: flex;
  flex: 1;
  min-height: 120px;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 8px;
  border: 1px dashed var(--ds-line-strong);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel-muted);
}

@media (max-width: 1360px) {
  .model-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 960px) {
  .pricing-workspace {
    grid-template-columns: 1fr;
  }

  .group-list {
    max-height: 320px;
    overflow-y: auto;
  }

  .model-card__body {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 720px) {
  .model-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .model-card {
    padding: 12px;
  }

  .pricing-panel {
    min-height: auto;
  }
}
</style>
