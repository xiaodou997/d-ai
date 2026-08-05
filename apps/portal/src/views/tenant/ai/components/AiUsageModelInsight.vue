<!--
  AI 工作台模型消耗分布图(echarts 横向条形图 + 明细列表)。
  重构:图表色值(坐标轴/背景轨/渐变)由硬编码 hex 改为运行时解析 var(--ds-*) token
       (chartTokens.ts,对主题子树内元素取计算样式),解析失败回退 echarts 默认色板。
-->
<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { Loading } from "@element-plus/icons-vue";
import { BarChart } from "echarts/charts";
import { GridComponent, TooltipComponent } from "echarts/components";
import { init, use, type EChartsType, graphic } from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";

import { formatCredits } from "../../../api/aiTenant";
import type { TenantAiDashboardTopModel } from "../../../types/aiTenant";
import { resolveDsColor } from "./chartTokens";

use([BarChart, GridComponent, TooltipComponent, CanvasRenderer]);

const props = defineProps<{
  loading: boolean;
  items: TenantAiDashboardTopModel[];
  rangeLabel: string;
}>();

const chartRef = ref<HTMLElement | null>(null);
let chartInstance: EChartsType | null = null;

const activeItems = computed(() => props.items.filter((item) => item.total_tenant_payable_credits > 0).slice(0, 8));
const totalCreditsText = computed(() =>
  formatCredits(activeItems.value.reduce((sum, item) => sum + Number(item.total_tenant_payable_credits || 0), 0))
);

const truncateLabel = (value: string) => (value.length > 12 ? `${value.slice(0, 12)}…` : value);

const renderChart = () => {
  if (!chartRef.value || !activeItems.value.length) {
    chartInstance?.clear();
    return;
  }

  if (!chartInstance) {
    chartInstance = init(chartRef.value);
  }

  const chartRows = [...activeItems.value].reverse();

  // echarts 无法消费 CSS var,对主题子树内元素解析 token;解析失败则交给 echarts 默认色板
  const axisColor = resolveDsColor(chartRef.value, "--ds-muted") || undefined;
  const trackColor = resolveDsColor(chartRef.value, "--ds-line") || undefined;
  const barFrom = resolveDsColor(chartRef.value, "--ds-accent");
  const barTo = resolveDsColor(chartRef.value, "--ds-accent-hover");
  const barColor =
    barFrom && barTo
      ? new graphic.LinearGradient(1, 0, 0, 0, [
          { offset: 0, color: barFrom },
          { offset: 1, color: barTo }
        ])
      : undefined;

  chartInstance.setOption({
    animationDuration: 500,
    grid: {
      left: 16,
      right: 16,
      top: 10,
      bottom: 8,
      containLabel: true
    },
    tooltip: {
      trigger: "axis",
      axisPointer: { type: "none" },
      formatter: (params: Array<{ dataIndex: number }>) => {
        const item = chartRows[params[0].dataIndex];
        return `${item.model_code}<br/>${formatCredits(item.total_tenant_payable_credits)} · ${item.request_count.toLocaleString("zh-CN")} 次请求 · ${item.total_tokens.toLocaleString("zh-CN")} Token`;
      }
    },
    xAxis: {
      type: "value",
      show: false
    },
    yAxis: {
      type: "category",
      data: chartRows.map((item) => truncateLabel(item.model_code)),
      axisTick: { show: false },
      axisLine: { show: false },
      axisLabel: {
        color: axisColor,
        fontSize: 11,
        fontWeight: 700
      }
    },
    series: [
      {
        type: "bar",
        data: chartRows.map((item) => item.total_tenant_payable_credits),
        barWidth: 16,
        showBackground: true,
        backgroundStyle: {
          color: trackColor,
          borderRadius: 999
        },
        itemStyle: {
          borderRadius: 999,
          color: barColor
        }
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
  <div class="model-insight">
    <div class="model-insight__header">
      <div>
        <h3 class="model-insight__title">模型消耗分布</h3>
        <p class="model-insight__desc">沿用 sub2api 思路，把 {{ props.rangeLabel }}高消耗模型作为首页第一图表。</p>
      </div>
      <span class="model-insight__summary">Top 模型合计 {{ totalCreditsText }}</span>
    </div>

    <div v-if="loading" class="model-insight__loading">
      <el-icon class="model-insight__spinner" :size="32"><Loading /></el-icon>
    </div>

    <div v-else-if="!activeItems.length" class="model-insight__empty">暂无模型消耗数据</div>

    <div v-else class="model-insight__body">
      <div ref="chartRef" class="model-insight__chart"></div>

      <div class="model-insight__list">
        <article v-for="item in activeItems" :key="item.model_code" class="model-insight__row">
          <div class="model-insight__copy">
            <p class="model-insight__name">{{ item.model_code }}</p>
            <p class="model-insight__meta">
              {{ item.request_count.toLocaleString("zh-CN") }} 次请求 · {{ item.total_tokens.toLocaleString("zh-CN") }} Token
            </p>
          </div>
          <div class="model-insight__value">
            <strong>{{ formatCredits(item.total_tenant_payable_credits) }}</strong>
          </div>
        </article>
      </div>
    </div>
  </div>
</template>

<style scoped>
.model-insight {
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.model-insight__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid var(--ds-line);
  padding: 24px 24px 18px;
}

.model-insight__title {
  margin: 0;
  color: var(--ds-ink);
  font-size: 16px;
  font-weight: 900;
  letter-spacing: -0.02em;
}

.model-insight__desc {
  margin: 6px 0 0;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.6;
}

.model-insight__summary {
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

.model-insight__loading,
.model-insight__empty {
  display: flex;
  min-height: 360px;
  align-items: center;
  justify-content: center;
  color: var(--ds-faint);
  font-size: 13px;
  font-weight: 700;
}

.model-insight__spinner {
  color: var(--ds-faint);
  animation: model-spin 1s linear infinite;
}

.model-insight__body {
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 18px 24px 24px;
}

.model-insight__chart {
  height: 248px;
  width: 100%;
}

.model-insight__list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.model-insight__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  padding: 12px 14px;
  background: var(--ds-panel-muted);
}

.model-insight__copy {
  min-width: 0;
}

.model-insight__name {
  margin: 0;
  overflow: hidden;
  color: var(--ds-ink);
  font-size: 13px;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.model-insight__meta {
  margin: 4px 0 0;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.5;
}

.model-insight__value strong {
  color: var(--ds-accent-hover);
  font-size: 18px;
  font-weight: 900;
  letter-spacing: -0.03em;
}

@keyframes model-spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}
</style>
