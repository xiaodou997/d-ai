<!--
  AI 工作台模型与入口版区块:模型消耗分布 + 来源分布。
  重构:迁移至 DsUI——TenantWorkbenchSection(自研卡片)→ AiWorkbenchSection 分区,
       来源条目色值由硬编码 hex 改为 DsUI token(在 AiUsageSourceInsight 内解析)。
-->
<script setup lang="ts">
import { ArrowRight } from "@element-plus/icons-vue";
import { useRouter } from "vue-router";

import AiWorkbenchSection from "./AiWorkbenchSection.vue";
import AiUsageModelInsight from "./AiUsageModelInsight.vue";
import AiUsageSourceInsight from "./AiUsageSourceInsight.vue";
import type { TenantAiDashboardTopModel } from "@/api/types/aiTenant";

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
  topModels: TenantAiDashboardTopModel[];
  modelLoading: boolean;
  sourceItems: SourceInsightItem[];
  sourceLoading: boolean;
  sourceSummary: string;
  rangeLabel: string;
}>();

const router = useRouter();
</script>

<template>
  <AiWorkbenchSection
    eyebrow="Usage Map"
    title="模型与入口版图"
    :description="`围绕 ${props.rangeLabel} 聚合模型消耗与入口结构，先看版图，再进明细。`"
  >
    <template #actions>
      <el-button text type="primary" class="!text-xs font-bold" @click="router.push('/tenant/ai/usage')">
        查看消耗明细 <el-icon class="ml-1"><ArrowRight /></el-icon>
      </el-button>
    </template>

    <div class="ai-charts-grid">
      <AiUsageModelInsight :loading="modelLoading" :items="topModels" :range-label="props.rangeLabel" />
      <AiUsageSourceInsight :loading="sourceLoading" :items="sourceItems" :summary="sourceSummary" />
    </div>
  </AiWorkbenchSection>
</template>

<style scoped>
.ai-charts-grid {
  display: grid;
  grid-template-columns: repeat(1, minmax(0, 1fr));
  gap: 16px;
}

@media (min-width: 1280px) {
  .ai-charts-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
