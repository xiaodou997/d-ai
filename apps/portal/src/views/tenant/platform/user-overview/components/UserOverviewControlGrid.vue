<!--
  用户详情管控区:AI 配置摘要(分组+限流)与风险/接口授权信号。
  重构:el-tag → DsTag(tone 语义色)、el-empty → DsEmpty;布局与数据渲染逻辑不变。
-->
<script setup lang="ts">
import { formatMultiplier } from "@/platform/ai/utils";
import { DsEmpty, DsTag } from "@/shared/ui";

import type { TenantAiLimitPolicy } from "@/api/types/aiTenant";
import { formatNumber } from "../formatters";
import type {
  UserOverviewAccessibleGroup,
  UserOverviewGroupSummary,
  UserOverviewRiskSignal
} from "../model";

const props = defineProps<{
  loading: boolean;
  aiPolicy: TenantAiLimitPolicy | null;
  accessibleGroups: UserOverviewAccessibleGroup[];
  groupSummary: UserOverviewGroupSummary;
  riskSignals: UserOverviewRiskSignal[];
  aiAvailable: boolean;
}>()

const emit = defineEmits<{
  (e: "open-ai-config"): void;
}>()

function toneClass(tone: UserOverviewRiskSignal["tone"]) {
  return `risk-item--${tone}`;
}

function sourceLabel(source: UserOverviewAccessibleGroup["source"]) {
  return source === "custom" ? "用户例外" : "默认开放";
}
</script>

<template>
  <section class="control-grid">
    <article v-loading="loading" class="control-card">
      <header class="control-header">
        <div>
          <h2 class="control-title">AI 配置摘要</h2>
          <p class="control-desc">这里聚合当前用户能看到的 AI 分组，以及用户级专属限流策略。</p>
        </div>
        <el-button v-if="aiAvailable" link type="primary" @click="emit('open-ai-config')">配置策略</el-button>
      </header>

      <template v-if="aiAvailable">
        <div class="mini-stats">
          <div class="mini-stat">
            <span class="mini-stat-label">可见分组</span>
            <strong class="mini-stat-value">{{ formatNumber(groupSummary.accessible) }}</strong>
          </div>
          <div class="mini-stat">
            <span class="mini-stat-label">默认开放</span>
            <strong class="mini-stat-value">{{ formatNumber(groupSummary.defaultVisible) }}</strong>
          </div>
          <div class="mini-stat">
            <span class="mini-stat-label">用户例外</span>
            <strong class="mini-stat-value">{{ formatNumber(groupSummary.customBindings) }}</strong>
          </div>
        </div>

        <div class="group-list">
          <div v-for="group in accessibleGroups" :key="group.id" class="group-chip">
            <div class="group-chip-head">
              <strong class="group-chip-title">{{ group.name }}</strong>
              <DsTag :tone="group.source === 'custom' ? 'warning' : 'positive'">
                {{ sourceLabel(group.source) }}
              </DsTag>
            </div>
            <p class="group-chip-meta">
              {{ group.source === "custom" ? "例外策略" : "默认策略" }} · 用户扣费倍率 ×{{ formatMultiplier(group.effectiveUserMultiplier) }}
            </p>
          </div>

          <DsEmpty v-if="!accessibleGroups.length" title="当前没有对该用户开放的 AI 分组" />
        </div>

        <div class="policy-panel">
          <div class="policy-header">
            <strong class="policy-title">用户级限流</strong>
            <DsTag :tone="aiPolicy?.status === 'disabled' ? 'warning' : aiPolicy ? 'positive' : 'neutral'">
              {{ aiPolicy ? (aiPolicy.status === "disabled" ? "已停用" : "启用") : "未配置" }}
            </DsTag>
          </div>
          <div class="policy-grid">
            <div class="policy-item">
              <span class="policy-label">最大同时请求数</span>
              <strong class="policy-value">{{ aiPolicy?.concurrency_limit ?? "—" }}</strong>
            </div>
          </div>
        </div>
      </template>

      <DsEmpty v-else title="当前租户未开通智能服务" />
    </article>

    <article v-loading="loading" class="control-card">
      <header class="control-header">
        <div>
          <h2 class="control-title">风险与限制</h2>
          <p class="control-desc">当前系统还没有单独的违规处罚流水，这里先展示停用、失败、限流与权限限制信号。</p>
        </div>
      </header>

      <div class="permission-card">
        <strong class="permission-title">平台限制</strong>
        <p class="permission-meta">权限、限流和账号状态统一由当前 Portal 的 AI 运营策略决定。</p>
      </div>

      <div class="risk-list">
        <div v-for="signal in riskSignals" :key="signal.id" class="risk-item" :class="toneClass(signal.tone)">
          <div class="risk-item-head">
            <strong class="risk-item-title">{{ signal.title }}</strong>
            <span class="risk-item-value">{{ signal.value }}</span>
          </div>
          <p class="risk-item-desc">{{ signal.description }}</p>
        </div>
      </div>
    </article>
  </section>
</template>

<style scoped>
.control-grid {
  display: grid;
  grid-template-columns: repeat(12, minmax(0, 1fr));
  gap: 16px;
}

.control-card {
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 20px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.control-card:first-child {
  grid-column: span 7;
}

.control-card:last-child {
  grid-column: span 5;
}

.control-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.control-title {
  margin: 0;
  font-size: 18px;
  font-weight: 700;
  color: var(--ds-ink);
}

.control-desc {
  margin: 6px 0 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--ds-muted);
}

.mini-stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.mini-stat {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 14px 16px;
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
}

.mini-stat-label,
.policy-label {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--ds-muted);
}

.mini-stat-value,
.policy-value {
  font-size: 26px;
  font-weight: 700;
  color: var(--ds-ink);
}

.group-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.group-chip {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 14px 16px;
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
  border: 1px solid var(--ds-line);
}

.group-chip-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.group-chip-title {
  font-size: 14px;
  line-height: 1.4;
  color: var(--ds-ink);
}

.group-chip-meta,
.permission-meta,
.risk-item-desc {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--ds-muted);
}

.policy-panel {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 16px;
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel-muted);
}

.policy-header,
.permission-head,
.risk-item-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.policy-title,
.permission-title,
.risk-item-title {
  font-size: 14px;
  font-weight: 700;
  color: var(--ds-ink);
}

.policy-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.policy-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 14px 16px;
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel);
}

.permission-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 16px;
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel-muted);
}

.permission-value,
.risk-item-value {
  font-size: 16px;
  font-weight: 700;
  color: var(--ds-ink);
}

.permission-bar {
  position: relative;
  width: 100%;
  height: 10px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--ds-line);
}

.permission-bar-fill {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--ds-accent);
}

.risk-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.risk-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 16px;
  border-radius: var(--ds-radius-control);
  border: 1px solid var(--ds-line);
}

.risk-item--success {
  background: var(--ds-positive-soft);
}

.risk-item--info {
  background: var(--ds-info-soft);
}

.risk-item--warning {
  background: var(--ds-warning-soft);
}

.risk-item--danger {
  background: var(--ds-danger-soft);
}

@media (max-width: 1200px) {
  .control-card:first-child,
  .control-card:last-child {
    grid-column: span 12;
  }
}

@media (max-width: 768px) {
  .mini-stats,
  .policy-grid,
  .group-list {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 640px) {
  .control-card {
    padding: 18px;
  }

  .group-chip-head,
  .policy-header,
  .permission-head,
  .risk-item-head {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
