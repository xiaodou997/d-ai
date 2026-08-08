<!--
  AI 工作台来源分布图(echarts 环形图 + 图例列表)。
  重构:分类色由硬编码 hex 改为 DsUI token(父级传 colorToken,本组件渲染时对主题子树内
       元素解析,chartTokens.ts),描边色取 --ds-panel,解析失败回退默认色板/CSS 兜底。
-->
<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { Loading } from "@element-plus/icons-vue";
import { PieChart } from "echarts/charts";
import { TooltipComponent } from "echarts/components";
import { init, use, type EChartsType } from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";

import { resolveDsColor } from "./chartTokens";

use([PieChart, TooltipComponent, CanvasRenderer]);

interface SourceInsightItem {
  key: string;
  label: string;
  colorToken: string;
  requestCount: number;
  shareText: string;
  successRateText: string;
  amountText: string;
  tokensText: string;
}

const props = defineProps<{
  loading: boolean;
  items: SourceInsightItem[];
  summary: string;
}>();

const chartRef = ref<HTMLElement | null>(null);
let chartInstance: EChartsType | null = null;

const activeItems = computed(() => props.items.filter((item) => item.requestCount > 0));
const totalRequestsText = computed(() =>
  activeItems.value.reduce((sum, item) => sum + item.requestCount, 0).toLocaleString("zh-CN")
);

// 各来源分类色:token 在渲染时对主题子树内元素解析(解析失败回退 echarts 默认色板/CSS 兜底)
const resolvedColors = ref<Record<string, string>>({});

const renderChart = () => {
  if (!chartRef.value || !activeItems.value.length) {
    chartInstance?.clear();
    return;
  }

  if (!chartInstance) {
    chartInstance = init(chartRef.value);
  }

  const colors: Record<string, string> = {};
  for (const item of props.items) {
    colors[item.key] = resolveDsColor(chartRef.value, item.colorToken);
  }
  resolvedColors.value = colors;
  const borderColor = resolveDsColor(chartRef.value, "--ds-panel") || undefined;

  chartInstance.setOption({
    animationDuration: 500,
    tooltip: {
      trigger: "item",
      formatter: (params: { name: string; value: number; percent: number }) =>
        `${params.name}<br/>${params.value.toLocaleString("zh-CN")} 次调用 (${params.percent}%)`
    },
    series: [
      {
        type: "pie",
        radius: ["56%", "78%"],
        center: ["50%", "50%"],
        avoidLabelOverlap: false,
        label: { show: false },
        labelLine: { show: false },
        itemStyle: {
          borderColor,
          borderWidth: 4,
          borderRadius: 10
        },
        emphasis: {
          scale: true,
          scaleSize: 8
        },
        data: activeItems.value.map((item) => ({
          name: item.label,
          value: item.requestCount,
          itemStyle: { color: colors[item.key] || undefined }
        }))
      }
    ]
  });
};

const handleResize = () => {
  chartInstance?.resize();
};

watch(activeItems, async () => {
  await nextTick();
  renderChart();
}, { deep: true });

onMounted(async () => {
  await nextTick();
  renderChart();
  window.addEventListener("resize", handleResize);
});

onUnmounted(() => {
  chartInstance?.dispose();
  window.removeEventListener("resize", handleResize);
});
</script>

<template>
  <div class="source-insight">
    <div class="source-insight__header">
      <div>
        <h3 class="source-insight__title">来源分布</h3>
        <p class="source-insight__desc">用环形图先看入口结构，再看每类入口的成功率和费用占比。</p>
      </div>
      <span class="source-insight__summary">{{ summary }}</span>
    </div>

    <div v-if="loading" class="source-insight__loading">
      <el-icon class="source-insight__spinner" :size="32"><Loading /></el-icon>
    </div>

    <div v-else-if="!activeItems.length" class="source-insight__empty">暂无调用样本</div>

    <div v-else class="source-insight__body">
      <div class="source-insight__chart-shell">
        <div ref="chartRef" class="source-insight__chart"></div>
        <div class="source-insight__center">
          <span>样本请求</span>
          <strong>{{ totalRequestsText }}</strong>
        </div>
      </div>

      <div class="source-insight__legend">
        <article v-for="item in items" :key="item.key" class="source-insight__legend-item">
          <div class="source-insight__legend-copy">
            <span class="source-insight__dot" :style="{ backgroundColor: resolvedColors[item.key] || undefined }"></span>
            <div>
              <p class="source-insight__legend-label">{{ item.label }}</p>
              <p class="source-insight__legend-meta">{{ item.shareText }} 样本占比 · {{ item.successRateText }} 成功率</p>
            </div>
          </div>
          <div class="source-insight__legend-values">
            <strong>{{ item.requestCount.toLocaleString("zh-CN") }}</strong>
            <span>{{ item.amountText }} / {{ item.tokensText }} Token</span>
          </div>
        </article>
      </div>
    </div>
  </div>
</template>

<style scoped>
.source-insight {
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.source-insight__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid var(--ds-line);
  padding: 24px 24px 18px;
}

.source-insight__title {
  margin: 0;
  color: var(--ds-ink);
  font-size: 16px;
  font-weight: 900;
  letter-spacing: -0.02em;
}

.source-insight__desc {
  margin: 6px 0 0;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.6;
}

.source-insight__summary {
  display: inline-flex;
  align-items: center;
  border-radius: var(--ds-radius-pill);
  background: var(--ds-accent-soft);
  padding: 8px 12px;
  color: var(--ds-accent-hover);
  font-size: 11px;
  font-weight: 800;
  line-height: 1.5;
}

.source-insight__loading,
.source-insight__empty {
  display: flex;
  min-height: 360px;
  align-items: center;
  justify-content: center;
  color: var(--ds-faint);
  font-size: 13px;
  font-weight: 700;
}

.source-insight__spinner {
  color: var(--ds-faint);
  animation: source-spin 1s linear infinite;
}

.source-insight__body {
  display: grid;
  grid-template-columns: minmax(240px, 280px) minmax(0, 1fr);
  gap: 18px;
  padding: 22px 24px 24px;
}

.source-insight__chart-shell {
  position: relative;
  display: flex;
  min-height: 280px;
  align-items: center;
  justify-content: center;
}

.source-insight__chart {
  height: 280px;
  width: 100%;
}

.source-insight__center {
  position: absolute;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  pointer-events: none;
}

.source-insight__center span {
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.source-insight__center strong {
  color: var(--ds-ink);
  font-size: 28px;
  font-weight: 900;
  letter-spacing: -0.04em;
}

.source-insight__legend {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.source-insight__legend-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  padding: 14px 16px;
  background: var(--ds-panel-muted);
}

.source-insight__legend-copy {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 12px;
}

.source-insight__dot {
  height: 10px;
  width: 10px;
  flex-shrink: 0;
  border-radius: 999px;
  background: var(--ds-faint);
  box-shadow: 0 0 0 6px var(--ds-panel-muted);
}

.source-insight__legend-label {
  margin: 0;
  color: var(--ds-ink);
  font-size: 13px;
  font-weight: 800;
}

.source-insight__legend-meta {
  margin: 4px 0 0;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.5;
}

.source-insight__legend-values {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
  text-align: right;
}

.source-insight__legend-values strong {
  color: var(--ds-ink);
  font-size: 18px;
  font-weight: 900;
  letter-spacing: -0.03em;
}

.source-insight__legend-values span {
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 700;
}

@media (max-width: 1024px) {
  .source-insight__body {
    grid-template-columns: 1fr;
  }
}

@keyframes source-spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}
</style>
