<script setup lang="ts">
import { DsEmpty, DsTag } from "@/shared/ui";
import type { TenantUsageLog } from "@/features/ai/usage";
import type { RechargeRecordItem } from "@/api/types/urmTenant";
import { formatCurrencyYuanFromCent, formatNumber, formatShortDateTime } from "../formatters";

type DsTagTone = "neutral" | "accent" | "positive" | "warning" | "danger" | "info";

defineProps<{
  loading: boolean;
  rechargeRecords: RechargeRecordItem[];
  rechargeTotal: number;
  aiUsageLogs: TenantUsageLog[];
  activityWindowLabel: string;
  aiAvailable: boolean;
}>();

function rechargeStatusTone(status: string): DsTagTone {
  return status === "reversed" ? "neutral" : "positive";
}

function aiStatusTone(status: string): DsTagTone {
  if (status === "success") return "positive";
  if (status === "failed" || status === "error") return "danger";
  if (status === "pending") return "warning";
  return "neutral";
}
</script>

<template>
  <section class="activity-grid">
    <article v-loading="loading" class="activity-card activity-card--recharge">
      <header class="activity-header">
        <div>
          <h2 class="activity-title">最近充值</h2>
          <p class="activity-desc">共 {{ formatNumber(rechargeTotal) }} 条充值记录，优先展示最新发生的用户充值。</p>
        </div>
      </header>

      <DsEmpty v-if="!rechargeRecords.length" title="暂无充值记录" />
      <div v-else class="activity-list">
        <div v-for="record in rechargeRecords" :key="record.orderId" class="activity-item">
          <div class="activity-item-main">
            <div class="activity-item-title-row">
              <strong class="activity-item-title">{{ record.orderId }}</strong>
              <DsTag :tone="rechargeStatusTone(record.status)">{{ record.status === "reversed" ? "已撤销" : "有效" }}</DsTag>
            </div>
            <p class="activity-item-meta">
              实付 {{ formatCurrencyYuanFromCent(record.paidAmount) }} · 到账 {{ formatNumber(record.creditAmount) }} 积分
            </p>
          </div>
          <span class="activity-item-time">{{ formatShortDateTime(record.createdTime) }}</span>
        </div>
      </div>
    </article>

    <article v-loading="loading" class="activity-card activity-card--ai">
      <header class="activity-header">
        <div>
          <h2 class="activity-title">AI 调用</h2>
          <p class="activity-desc">{{ activityWindowLabel }}内最近 8 条智能服务调用记录。</p>
        </div>
      </header>

      <DsEmpty v-if="!aiUsageLogs.length" :title="aiAvailable ? '暂无 AI 调用记录' : '当前租户未开通智能服务'" />
      <div v-else class="activity-list">
        <div v-for="log in aiUsageLogs" :key="log.id" class="activity-item">
          <div class="activity-item-main">
            <div class="activity-item-title-row">
              <strong class="activity-item-title">{{ log.model_code }}</strong>
              <div class="activity-tag-row">
                <DsTag :tone="aiStatusTone(log.request_status)">{{ log.request_status }}</DsTag>
                <DsTag>{{ formatNumber(log.user_charged_credits) }} 积分</DsTag>
              </div>
            </div>
            <p class="activity-item-meta">
              {{ log.request_source }} · {{ formatNumber(log.total_tokens) }} tokens · {{ log.billing_group_label_snapshot || log.group_name_snapshot || "未命名分组" }}
            </p>
          </div>
          <span class="activity-item-time">{{ formatShortDateTime(log.created_at) }}</span>
        </div>
      </div>
    </article>
  </section>
</template>

<style scoped>
.activity-grid {
  display: grid;
  grid-template-columns: repeat(12, minmax(0, 1fr));
  gap: 16px;
}

.activity-card {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 20px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.activity-card--recharge {
  grid-column: span 4;
}

.activity-card--ai {
  grid-column: span 8;
}

.activity-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.activity-title {
  margin: 0;
  font-size: 18px;
  font-weight: 700;
  color: var(--ds-ink);
}

.activity-desc {
  margin: 6px 0 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--ds-muted);
}

.activity-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.activity-item {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 16px;
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
}

.activity-item-main {
  min-width: 0;
}

.activity-item-title-row,
.activity-tag-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.activity-tag-row {
  gap: 8px;
}

.activity-item-title {
  font-size: 14px;
  line-height: 1.4;
  color: var(--ds-ink);
}

.activity-item-meta {
  margin: 6px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--ds-muted);
  word-break: break-word;
}

.activity-item-time {
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--ds-faint);
}

@media (max-width: 1200px) {
  .activity-card--recharge,
  .activity-card--ai {
    grid-column: span 12;
  }
}

@media (max-width: 640px) {
  .activity-card {
    padding: 18px;
  }

  .activity-item {
    flex-direction: column;
  }

  .activity-item-time {
    font-size: 11px;
  }
}
</style>
