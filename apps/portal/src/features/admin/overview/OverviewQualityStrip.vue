<script setup lang="ts">
import { PortalContentCard } from "@/platform";
import { formatNumber, rateText } from "./overviewUtils";
import type { OverviewSummary } from "./overviewModel";

defineProps<{ summary: OverviewSummary; compact?: boolean }>();
</script>

<template>
  <PortalContentCard title="请求质量" description="成功率与账号缓存读命中率，按当前时间范围统计。" :class="{ 'quality-strip--compact': compact }">
    <div class="quality-strip">
      <div class="quality-item">
        <span>平台成功率</span>
        <strong>{{ rateText(summary.successful_requests, summary.total_requests) }}</strong>
        <small>{{ formatNumber(summary.failed_requests) }} 次失败</small>
      </div>
      <div class="quality-item">
        <span>账号缓存率</span>
        <strong>{{ rateText(summary.cache_read_tokens, summary.total_prompt_tokens + summary.cache_read_tokens) }}</strong>
        <small>{{ formatNumber(summary.cache_read_tokens) }} 缓存读 Token</small>
      </div>
      <div class="quality-item">
        <span>活跃账号</span>
        <strong>{{ formatNumber(summary.active_accounts) }}</strong>
        <small>当前窗口产生请求</small>
      </div>
    </div>
  </PortalContentCard>
</template>

<style scoped>
.quality-strip { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.quality-item { display: grid; gap: 5px; padding: 14px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-control); background: var(--ds-panel-muted); }
.quality-item span, .quality-item small { color: var(--ds-muted); font-size: 11px; }
.quality-item strong { color: var(--ds-ink); font-size: 22px; line-height: 1.1; }
.quality-strip--compact :deep(.portal-content-card__body) { padding-top: 0; }
@media (max-width: 680px) { .quality-strip { grid-template-columns: 1fr; } }
</style>
