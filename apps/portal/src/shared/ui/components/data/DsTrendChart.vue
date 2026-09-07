<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { LineChart } from "echarts/charts";
import { GridComponent, LegendComponent, TooltipComponent } from "echarts/components";
import { init, use, type EChartsType } from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";
use([LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer]);
export interface DsTrendSeries { label: string; values: number[]; color?: string; }
interface DsTooltipItem { dataIndex?: number; color?: string; seriesName?: string; value?: number; }
const props = withDefaults(defineProps<{ labels: string[]; series: DsTrendSeries[]; valueFormatter?: (value: number) => string; height?: number; emptyText?: string }>(), { height: 240, emptyText: "暂无趋势数据" });
const chartRef = ref<HTMLElement | null>(null); let chart: EChartsType | null = null;
const hasData = () => props.labels.length > 0 && props.series.some((s) => s.values.some((v) => Number(v) > 0));
const cssColor = (token: string) => chartRef.value ? getComputedStyle(chartRef.value).getPropertyValue(token).trim() : "";
const resolveSeriesColor = (value?: string) => {
  const match = value?.match(/^var\((--[^),]+)(?:,.*)?\)$/);
  return match ? cssColor(match[1]) : value || cssColor("--ds-accent");
};
const formatValue = (value: number) => props.valueFormatter ? props.valueFormatter(value) : Number(value || 0).toLocaleString("zh-CN");
function render() {
  if (!chartRef.value) return; if (!hasData()) { chart?.clear(); return; } chart ||= init(chartRef.value);
  const axis = cssColor("--ds-muted"), faint = cssColor("--ds-faint") || axis, line = cssColor("--ds-line"), panel = cssColor("--ds-panel");
  chart.setOption({ animationDuration: 500, animationEasing: "cubicOut", color: props.series.map((s) => resolveSeriesColor(s.color)), grid: { left: 12, right: 16, top: 30, bottom: 30, containLabel: true }, legend: { show: props.series.length > 1, top: 0, left: 0, icon: "roundRect", itemWidth: 16, itemHeight: 3, itemGap: 16, textStyle: { color: axis, fontSize: 11, fontWeight: 600 } }, tooltip: { trigger: "axis", axisPointer: { type: "line", lineStyle: { color: line, type: "dashed" } }, backgroundColor: panel, borderColor: line, textStyle: { color: axis, fontSize: 12 }, padding: [9, 12], formatter: (items: DsTooltipItem[]) => { const title = props.labels[items[0]?.dataIndex ?? 0] || ""; return `<div style="font-weight:700;margin-bottom:6px">${title}</div>${items.map((item) => `<div style="display:flex;gap:8px;justify-content:space-between"><span><i style="display:inline-block;width:7px;height:7px;border-radius:var(--ds-radius-circle);background:${item.color};margin-right:6px"></i>${item.seriesName}</span><b>${formatValue(Number(item.value) || 0)}</b></div>`).join("")}`; } }, xAxis: { type: "category", boundaryGap: false, data: props.labels, axisLine: { lineStyle: { color: line } }, axisTick: { show: false }, axisLabel: { color: faint, fontSize: 10, margin: 10, hideOverlap: true } }, yAxis: { type: "value", min: 0, splitNumber: 4, axisLine: { show: false }, axisTick: { show: false }, axisLabel: { color: faint, fontSize: 10, formatter: (v: number) => formatValue(v) }, splitLine: { lineStyle: { color: line, type: "dashed", opacity: 0.7 } } }, series: props.series.map((s) => ({ name: s.label, type: "line", data: s.values.map((v) => Number(v) || 0), smooth: 0.28, showSymbol: false, symbol: "circle", symbolSize: 7, lineStyle: { width: 2.5 }, itemStyle: { color: resolveSeriesColor(s.color) }, emphasis: { focus: "series", showSymbol: true }, areaStyle: { opacity: 0.08 } })) }, true);
}
function resize() { chart?.resize(); }
watch(() => [props.labels, props.series], async () => { await nextTick(); render(); }, { deep: true });
onMounted(async () => { await nextTick(); render(); window.addEventListener("resize", resize); });
onUnmounted(() => { chart?.dispose(); window.removeEventListener("resize", resize); });
</script>
<template><div ref="chartRef" class="ds-trend-chart" :style="{ height: `${height}px` }"><div v-if="!hasData()" class="ds-trend-chart__empty">{{ emptyText }}</div></div></template>
<style scoped>.ds-trend-chart { width: 100%; min-height: 190px; }.ds-trend-chart__empty { display: grid; height: 100%; min-height: 190px; place-items: center; color: var(--ds-muted); font-size: 13px; }</style>
