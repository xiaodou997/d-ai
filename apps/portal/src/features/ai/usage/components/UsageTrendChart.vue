<script setup lang="ts">
import { computed } from "vue";

import type { DailyTrendRowDTO } from "../model";
import { formatShortDate } from "../format";

interface TrendSeries {
  label: string;
  color: string;
  points: number[];
}

const props = withDefaults(
  defineProps<{
    rows: DailyTrendRowDTO[];
    series: TrendSeries[];
    emptyText?: string;
  }>(),
  {
    emptyText: "暂无趋势数据"
  }
);

const chartWidth = 640;
const chartHeight = 180;
const pad = { top: 10, right: 12, bottom: 28, left: 12 };

const maxValue = computed(() =>
  Math.max(1, ...props.series.flatMap((series) => series.points.map((point) => Number(point) || 0)))
);

const xStep = computed(() => {
  if (props.rows.length <= 1) return 0;
  return (chartWidth - pad.left - pad.right) / (props.rows.length - 1);
});

const lineSeries = computed(() => {
  const yRange = chartHeight - pad.top - pad.bottom;
  return props.series.map((series) => ({
    ...series,
    polyline: series.points
      .map((point, index) => {
        const x = pad.left + xStep.value * index;
        const y = pad.top + yRange * (1 - (Number(point) || 0) / maxValue.value);
        return `${x},${y}`;
      })
      .join(" ")
  }));
});

const xTicks = computed(() => {
  if (!props.rows.length) return [];
  const tickStep = Math.max(1, Math.ceil(props.rows.length / 6));
  return props.rows
    .map((row, index) => ({ row, index }))
    .filter(({ index }) => index % tickStep === 0 || index === props.rows.length - 1)
    .map(({ row, index }) => ({
      label: formatShortDate(row.date),
      x: pad.left + xStep.value * index
    }));
});

const yGuides = computed(() => {
  const bands = 4;
  const yRange = chartHeight - pad.top - pad.bottom;
  return Array.from({ length: bands + 1 }, (_, index) => {
    const ratio = index / bands;
    return {
      y: pad.top + yRange * ratio
    };
  });
});
</script>

<template>
  <div class="usage-trend-chart">
    <div v-if="series.length" class="usage-trend-chart__legend">
      <span
        v-for="item in series"
        :key="item.label"
        class="usage-trend-chart__legend-item"
      >
        <span class="usage-trend-chart__legend-dot" :style="{ backgroundColor: item.color || undefined }"></span>
        {{ item.label }}
      </span>
    </div>

    <svg v-if="rows.length" :viewBox="`0 0 ${chartWidth} ${chartHeight}`" class="usage-trend-chart__svg">
      <line
        v-for="guide in yGuides"
        :key="guide.y"
        :x1="pad.left"
        :y1="guide.y"
        :x2="chartWidth - pad.right"
        :y2="guide.y"
        class="usage-trend-chart__guide"
      />

      <polyline
        v-for="item in lineSeries"
        :key="item.label"
        :points="item.polyline"
        fill="none"
        :style="{ stroke: item.color || undefined }"
        class="usage-trend-chart__polyline"
        stroke-linejoin="round"
        stroke-linecap="round"
        stroke-width="2.4"
      />

      <text
        v-for="tick in xTicks"
        :key="tick.label + tick.x"
        :x="tick.x"
        :y="chartHeight - 6"
        text-anchor="middle"
        class="usage-trend-chart__tick"
      >
        {{ tick.label }}
      </text>
    </svg>

    <p v-else class="usage-trend-chart__empty">{{ emptyText }}</p>
  </div>
</template>

<style scoped>
.usage-trend-chart {
  display: grid;
  gap: 14px;
}

.usage-trend-chart__legend {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
}

.usage-trend-chart__legend-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--ds-muted);
  font-size: 12px;
  font-weight: 600;
}

.usage-trend-chart__legend-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  /* series color 为空（SSR/无 DOM 时 token 解析不到）时的兜底 */
  background-color: var(--ds-accent);
}

.usage-trend-chart__svg {
  width: 100%;
  height: auto;
  overflow: visible;
}

.usage-trend-chart__guide {
  stroke: color-mix(in srgb, var(--ds-line) 85%, transparent);
  stroke-width: 1;
}

.usage-trend-chart__polyline {
  /* series color 为空（SSR/无 DOM 时 token 解析不到）时的兜底 */
  stroke: var(--ds-accent);
}

.usage-trend-chart__tick {
  fill: var(--ds-faint);
  font-size: 10px;
  font-weight: 600;
}

.usage-trend-chart__empty {
  margin: 0;
  padding: 28px 0;
  text-align: center;
  color: var(--ds-faint);
  font-size: 13px;
}
</style>
