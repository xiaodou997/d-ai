<!--
  AI 工作台指标区:资产与授权面 + 核心调用信号两组指标（dashboard feature）。
  重构:迁移至 DsUI——自研彩色指标卡(硬编码 hex tone)→ DsMetricCard,
       外层卡片改为 AiWorkbenchSection 分区(一体面板内 1px 分隔线),loading 时值显示「—」。
-->
<script setup lang="ts">
import { DsMetricCard } from "@/shared/ui";

import AiWorkbenchSection from "./AiWorkbenchSection.vue";

interface MetricItem {
  key: string;
  label: string;
  value: string | number;
  hint?: string;
  loading?: boolean;
}

defineProps<{
  accessMetrics: MetricItem[];
  signalMetrics: MetricItem[];
  rangeLabel: string;
}>();

const displayValue = (metric: MetricItem) => (metric.loading ? "—" : String(metric.value));
</script>

<template>
  <AiWorkbenchSection
    title="服务资产"
    description="汇总当前可用模型、模型分组和 API 密钥。"
  >
    <div class="ai-metric-grid">
      <DsMetricCard
        v-for="metric in accessMetrics"
        :key="metric.key"
        :label="metric.label"
        :value="displayValue(metric)"
        :hint="metric.hint"
      />
    </div>

    <div class="ai-metric-divider"></div>

    <div class="ai-metric-cluster">
      <div class="ai-metric-cluster__lead">
        <h3 class="ai-metric-cluster__title">调用概况</h3>
        <p class="ai-metric-cluster__desc">{{ rangeLabel }}内统一观察请求量、成功率、Token 使用量与平均延迟。</p>
      </div>

      <div class="ai-metric-grid">
        <DsMetricCard
          v-for="metric in signalMetrics"
          :key="metric.key"
          :label="metric.label"
          :value="displayValue(metric)"
          :hint="metric.hint"
        />
      </div>
    </div>
  </AiWorkbenchSection>
</template>

<style scoped>
.ai-metric-grid {
  display: grid;
  grid-template-columns: repeat(1, minmax(0, 1fr));
  gap: 16px;
}

@media (min-width: 768px) {
  .ai-metric-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (min-width: 1280px) {
  .ai-metric-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}

.ai-metric-divider {
  height: 1px;
  background: var(--ds-line);
}

.ai-metric-cluster {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.ai-metric-cluster__lead {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.ai-metric-cluster__title {
  margin: 0;
  color: var(--ds-ink);
  font-size: 15px;
  font-weight: 650;
}

.ai-metric-cluster__desc {
  margin: 0;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.6;
}
</style>
