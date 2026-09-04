<script setup lang="ts">
import {
  PortalContentCard,
  PortalMetricGrid
} from "@/platform";
import {
  UsageTag
} from "@/platform/ai/usage";

import type { AdminUsageRow, DailyTrendRowDTO, UsageMetric } from "../model";
import type { UsageDistributionItem } from "../format";
import { formatCompactNumber, formatNumber, formatPercent, formatTimestamp, formatUSD2, resolveRequestTotalMs } from "../format";
import UsageTrendChart from "./UsageTrendChart.vue";

interface TrendSeries {
  label: string;
  color: string;
  points: number[];
}

const props = defineProps<{
  failedLogs: AdminUsageRow[];
  loading: boolean;
  metrics: UsageMetric[];
  modelDistribution: UsageDistributionItem[];
  requestTrendSeries: TrendSeries[];
  rows: DailyTrendRowDTO[];
  slowLogs: AdminUsageRow[];
  tokenTrendSeries: TrendSeries[];
  unitDistribution: UsageDistributionItem[];
}>();

const emit = defineEmits<{
  selectRecord: [row: AdminUsageRow];
}>();
</script>

<template>
  <div class="usage-analytics">
    <PortalMetricGrid :metrics="metrics" />

    <section class="usage-analytics__grid usage-analytics__grid--trend">
      <PortalContentCard title="请求趋势" description="看量级、成功与失败是否出现同步拐点。">
        <UsageTrendChart :rows="rows" :series="requestTrendSeries" />
      </PortalContentCard>
      <PortalContentCard title="Token 趋势" description="输入与输出拆开看，便于识别生图或长文本请求带来的结构变化。">
        <UsageTrendChart :rows="rows" :series="tokenTrendSeries" />
      </PortalContentCard>
    </section>

    <section class="usage-analytics__grid">
      <PortalContentCard title="模型成本分布" description="按用户计费金额降序，识别主要预算消耗。">
        <div class="usage-dist-list">
          <article v-for="item in modelDistribution" :key="item.name" class="usage-dist-row">
            <div class="usage-dist-row__head">
              <span class="usage-dist-row__name">{{ item.name }}</span>
              <span class="usage-dist-row__percent">{{ formatPercent(item.percent) }}</span>
            </div>
            <div class="usage-dist-row__bar">
              <div class="usage-dist-row__fill" :style="{ width: `${item.percent}%` }"></div>
            </div>
            <div class="usage-dist-row__meta">
              <span>{{ formatUSD2(item.amountUSD) }}</span>
              <span>{{ formatNumber(item.requests) }} 次</span>
            </div>
          </article>
          <p v-if="!modelDistribution.length" class="usage-empty">暂无模型分布数据</p>
        </div>
      </PortalContentCard>

      <PortalContentCard title="计费单位分布" description="判断当前窗口更偏 token、请求数还是图片/时长。">
        <div class="usage-dist-list">
          <article v-for="item in unitDistribution" :key="item.name" class="usage-dist-row">
            <div class="usage-dist-row__head">
              <span class="usage-dist-row__name">{{ item.name }}</span>
              <span class="usage-dist-row__percent">{{ formatPercent(item.percent) }}</span>
            </div>
            <div class="usage-dist-row__bar">
              <div class="usage-dist-row__fill usage-dist-row__fill--info" :style="{ width: `${item.percent}%` }"></div>
            </div>
            <div class="usage-dist-row__meta">
              <span>{{ formatUSD2(item.amountUSD) }}</span>
              <span>{{ formatCompactNumber(item.units || 0) }} 计费量</span>
            </div>
          </article>
          <p v-if="!unitDistribution.length" class="usage-empty">暂无计费单位结构数据</p>
        </div>
      </PortalContentCard>
    </section>

    <section class="usage-analytics__grid">
      <PortalContentCard title="失败样本" description="失败率高时，先看这一组，直接进详情检查链路。">
        <div class="sample-list">
          <button
            v-for="row in failedLogs"
            :key="row.request_id"
            type="button"
            class="sample-list__item"
            @click="emit('selectRecord', row)"
          >
            <div class="sample-list__head">
              <UsageTag kind="status" :value="row.request_status" />
              <span class="sample-list__mono">{{ row.http_status ?? "—" }}/{{ row.upstream_status ?? "—" }}</span>
            </div>
            <strong>{{ row.model_code }}</strong>
            <span>{{ row.error_message || row.error_code || row.request_id }}</span>
            <small>{{ formatTimestamp(row.created_at) }}</small>
          </button>
          <p v-if="!failedLogs.length" class="usage-empty">当前页暂无失败样本</p>
        </div>
      </PortalContentCard>

      <PortalContentCard title="慢请求样本" description="这里按总耗时排序，先看用户真正感知到的慢请求，再进详情拆首响与连接。">
        <div class="sample-list">
          <button
            v-for="row in slowLogs"
            :key="row.request_id"
            type="button"
            class="sample-list__item"
            @click="emit('selectRecord', row)"
          >
            <div class="sample-list__head">
              <UsageTag kind="source" :value="row.request_source" />
              <span class="sample-list__mono">{{ resolveRequestTotalMs(row) }} ms</span>
            </div>
            <strong>{{ row.model_code }}</strong>
            <span>{{ row.error_message || row.request_id }}</span>
            <small>{{ formatTimestamp(row.created_at) }}</small>
          </button>
          <p v-if="!slowLogs.length" class="usage-empty">当前页暂无慢请求样本</p>
        </div>
      </PortalContentCard>
    </section>
  </div>
</template>

<style scoped>
.usage-analytics {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 20px;
  min-width: 0;
}

.usage-analytics__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 20px;
}

.usage-analytics__grid--trend {
  align-items: stretch;
}

.usage-dist-list,
.sample-list {
  display: grid;
  gap: 12px;
}

.usage-dist-row,
.sample-list__item {
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
  padding: 12px;
}

.usage-dist-row {
  display: grid;
  gap: 8px;
}

.usage-dist-row__head,
.sample-list__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.usage-dist-row__name {
  color: var(--ds-ink);
  font-size: 13px;
  font-weight: 650;
}

.usage-dist-row__percent {
  color: var(--ds-accent);
  font-size: 12px;
  font-weight: 700;
}

.usage-dist-row__bar {
  height: 6px;
  border-radius: var(--ds-radius-pill);
  overflow: hidden;
  background: var(--ds-panel);
}

.usage-dist-row__fill {
  height: 100%;
  border-radius: var(--ds-radius-inherit);
  background: linear-gradient(90deg, var(--ds-accent), color-mix(in srgb, var(--ds-warning) 58%, var(--ds-accent)));
}

.usage-dist-row__fill--info {
  background: linear-gradient(90deg, var(--ds-info), color-mix(in srgb, var(--ds-accent) 46%, var(--ds-info)));
}

.usage-dist-row__meta {
  display: flex;
  gap: 12px;
  color: var(--ds-faint);
  font-size: 11px;
}

.sample-list__item {
  display: grid;
  width: 100%;
  gap: 4px;
  text-align: left;
  cursor: pointer;
}

.sample-list__item strong {
  color: var(--ds-ink);
  font-size: 13px;
}

.sample-list__item span,
.sample-list__item small {
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.5;
}

.sample-list__mono {
  font-family: "SF Mono", "Fira Code", monospace;
  color: var(--ds-faint);
  font-size: 12px;
}

.usage-empty {
  margin: 0;
  color: var(--ds-faint);
  font-size: 12px;
  line-height: 1.6;
}

@media (max-width: 960px) {
  .usage-analytics__grid {
    grid-template-columns: 1fr;
  }
}
</style>
