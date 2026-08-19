<script setup lang="ts">
import { computed } from "vue";

export interface OverviewTrendSeries {
  label: string;
  values: number[];
  color: string;
}

const props = defineProps<{
  labels: string[];
  series: OverviewTrendSeries[];
  valueFormatter?: (value: number) => string;
}>();

const width = 720;
const height = 230;
const padding = { top: 16, right: 12, bottom: 30, left: 12 };
const chartWidth = width - padding.left - padding.right;
const chartHeight = height - padding.top - padding.bottom;

const maxValue = computed(() => Math.max(1, ...props.series.flatMap((item) => item.values.map((value) => Number(value) || 0))));
const hasData = computed(() => props.labels.length > 0 && props.series.some((item) => item.values.some((value) => Number(value) > 0)));
const gridLines = [0, 1, 2, 3, 4];

function points(values: number[]) {
  if (!values.length) return "";
  const step = chartWidth / Math.max(values.length - 1, 1);
  return values.map((rawValue, index) => {
    const value = Math.max(0, Number(rawValue) || 0);
    const x = padding.left + index * step;
    const y = padding.top + chartHeight * (1 - value / maxValue.value);
    return `${x.toFixed(2)},${y.toFixed(2)}`;
  }).join(" ");
}

function labelAt(index: number) {
  return props.labels[index] || "";
}

const visibleLabels = computed(() => {
  const count = props.labels.length;
  if (!count) return [];
  const step = Math.max(1, Math.ceil(count / 6));
  return props.labels.map((label, index) => ({ label, index })).filter((item) => item.index % step === 0 || item.index === count - 1);
});

function formatValue(value: number) {
  return props.valueFormatter ? props.valueFormatter(value) : Number(value || 0).toLocaleString("zh-CN");
}
</script>

<template>
  <div class="overview-trend-chart">
    <div v-if="hasData" class="overview-trend-chart__legend">
      <span v-for="item in series" :key="item.label" class="overview-trend-chart__legend-item">
        <i :style="{ background: item.color }"></i>{{ item.label }}
      </span>
    </div>
    <div v-if="hasData" class="overview-trend-chart__canvas">
      <svg :viewBox="`0 0 ${width} ${height}`" role="img" aria-label="趋势图">
        <line
          v-for="line in gridLines"
          :key="line"
          :x1="padding.left"
          :x2="width - padding.right"
          :y1="padding.top + chartHeight * (line / (gridLines.length - 1))"
          :y2="padding.top + chartHeight * (line / (gridLines.length - 1))"
          stroke="var(--ds-line)"
          stroke-width="1"
        />
        <polyline
          v-for="item in series"
          :key="item.label"
          :points="points(item.values)"
          fill="none"
          :stroke="item.color"
          stroke-width="2.5"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
        <text
          v-for="item in visibleLabels"
          :key="`${item.label}-${item.index}`"
          :x="padding.left + (chartWidth / Math.max(labels.length - 1, 1)) * item.index"
          :y="height - 8"
          text-anchor="middle"
          fill="var(--ds-muted)"
          font-size="11"
        >{{ labelAt(item.index) }}</text>
      </svg>
    </div>
    <div v-else class="overview-trend-chart__empty">暂无趋势数据</div>
    <div v-if="hasData" class="overview-trend-chart__values">
      <span v-for="item in series" :key="item.label"><b :style="{ color: item.color }">{{ item.label }}</b> {{ formatValue(item.values[item.values.length - 1] || 0) }}</span>
    </div>
  </div>
</template>

<style scoped>
.overview-trend-chart { min-width: 0; }
.overview-trend-chart__legend { display: flex; flex-wrap: wrap; gap: 12px; margin-bottom: 10px; color: var(--ds-muted); font-size: 11px; }
.overview-trend-chart__legend-item { display: inline-flex; align-items: center; gap: 5px; }
.overview-trend-chart__legend-item i { display: inline-block; width: 8px; height: 8px; border-radius: 50%; }
.overview-trend-chart__canvas { width: 100%; overflow: hidden; }
.overview-trend-chart__canvas svg { display: block; width: 100%; height: auto; min-height: 190px; }
.overview-trend-chart__empty { display: grid; min-height: 190px; place-items: center; color: var(--ds-muted); font-size: 13px; }
.overview-trend-chart__values { display: flex; flex-wrap: wrap; gap: 14px; margin-top: 4px; color: var(--ds-muted); font-size: 11px; }
.overview-trend-chart__values b { margin-right: 3px; font-weight: 650; }
</style>
