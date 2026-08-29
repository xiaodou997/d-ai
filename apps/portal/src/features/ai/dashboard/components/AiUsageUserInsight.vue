<!--
  业务概览核心调用用户 Top 图（dashboard feature，echarts 横向条形图 + 排名列表）。
  重构:图表色值由硬编码 hex 改为运行时解析 var(--ds-*) token(chartTokens.ts),
       解析失败回退 echarts 默认色板。
-->
<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { Loading } from "@element-plus/icons-vue";
import { BarChart } from "echarts/charts";
import { GridComponent, TooltipComponent } from "echarts/components";
import { init, use, type EChartsType, graphic } from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";

import { resolveDsColor } from "./chartTokens";

use([BarChart, GridComponent, TooltipComponent, CanvasRenderer]);

interface UserInsightItem {
  key: string;
  userLabel: string;
  totalAmountUSD: number;
  amountText: string;
  requestCount: number;
  successRateText: string;
  lastActiveText: string;
}

const props = defineProps<{
  loading: boolean;
  items: UserInsightItem[];
}>();

const chartRef = ref<HTMLElement | null>(null);
let chartInstance: EChartsType | null = null;

const activeItems = computed(() => props.items.filter((item) => item.requestCount > 0));

const truncateLabel = (value: string) => (value.length > 10 ? `${value.slice(0, 10)}…` : value);

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
      formatter: (params: Array<{ name: string; value: number; dataIndex: number }>) => {
        const param = params[0];
        const item = chartRows[param.dataIndex];
        return `${item.userLabel}<br/>${item.requestCount.toLocaleString("zh-CN")} 次请求 · ${item.successRateText} 成功率 · ${item.amountText}`;
      }
    },
    xAxis: {
      type: "value",
      show: false
    },
    yAxis: {
      type: "category",
      data: chartRows.map((item) => truncateLabel(item.userLabel)),
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
        data: chartRows.map((item) => item.requestCount),
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
  <div class="user-insight">
    <div class="user-insight__header">
      <div>
        <h3 class="user-insight__title">核心调用用户 Top 6</h3>
        <p class="user-insight__desc">基于当前调用样本按请求量排序，并补充成功率、结算金额和最近活跃时间。</p>
      </div>
    </div>

    <div v-if="loading" class="user-insight__loading">
      <el-icon class="user-insight__spinner" :size="32"><Loading /></el-icon>
    </div>

    <div v-else-if="!activeItems.length" class="user-insight__empty">暂无用户调用数据</div>

    <div v-else class="user-insight__body">
      <div ref="chartRef" class="user-insight__chart"></div>

      <div class="user-insight__list">
        <article v-for="(item, index) in activeItems" :key="item.key" class="user-insight__row">
          <div class="user-insight__rank">{{ index + 1 }}</div>
          <div class="user-insight__copy">
            <p class="user-insight__name">{{ item.userLabel }}</p>
            <p class="user-insight__meta">{{ item.successRateText }} 成功率 · {{ item.amountText }}</p>
          </div>
          <div class="user-insight__value">
            <strong>{{ item.requestCount.toLocaleString("zh-CN") }} 次</strong>
            <span>{{ item.lastActiveText }}</span>
          </div>
        </article>
      </div>
    </div>
  </div>
</template>

<style scoped>
.user-insight {
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.user-insight__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid var(--ds-line);
  padding: 24px 24px 18px;
}

.user-insight__title {
  margin: 0;
  color: var(--ds-ink);
  font-size: 16px;
  font-weight: 900;
  letter-spacing: -0.02em;
}

.user-insight__desc {
  margin: 6px 0 0;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.6;
}

.user-insight__loading,
.user-insight__empty {
  display: flex;
  min-height: 360px;
  align-items: center;
  justify-content: center;
  color: var(--ds-faint);
  font-size: 13px;
  font-weight: 700;
}

.user-insight__spinner {
  color: var(--ds-faint);
  animation: user-spin 1s linear infinite;
}

.user-insight__body {
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 18px 24px 24px;
}

.user-insight__chart {
  height: 248px;
  width: 100%;
}

.user-insight__list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.user-insight__row {
  display: grid;
  grid-template-columns: 40px minmax(0, 1fr) auto;
  align-items: center;
  gap: 14px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  padding: 12px 14px;
  background: var(--ds-panel-muted);
}

.user-insight__rank {
  display: flex;
  height: 32px;
  width: 32px;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  background: var(--ds-accent-soft);
  color: var(--ds-accent-hover);
  font-size: 13px;
  font-weight: 900;
}

.user-insight__copy {
  min-width: 0;
}

.user-insight__name {
  margin: 0;
  overflow: hidden;
  color: var(--ds-ink);
  font-size: 13px;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-insight__meta {
  margin: 4px 0 0;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.5;
}

.user-insight__value {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
  text-align: right;
}

.user-insight__value strong {
  color: var(--ds-accent-hover);
  font-size: 17px;
  font-weight: 900;
  letter-spacing: -0.03em;
}

.user-insight__value span {
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 700;
}

@keyframes user-spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}
</style>
