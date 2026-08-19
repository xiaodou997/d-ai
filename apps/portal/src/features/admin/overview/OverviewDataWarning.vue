<script setup lang="ts">
import { computed } from "vue";
import { AlertTriangle } from "lucide-vue-next";

import type { OverviewSection } from "./overviewTypes";

const props = defineProps<{
  sections: OverviewSection[];
}>();

const sectionLabels: Record<OverviewSection, string> = {
  summary: "核心指标",
  models: "模型排行",
  tenants: "租户排行",
  errors: "最近错误",
  trend: "趋势",
  upstreams: "上游汇总",
  system: "系统状态",
  global: "业务统计",
  modules: "系统模块",
  proxy: "代理节点"
};

const failedLabels = computed(() => props.sections.map((section) => sectionLabels[section]).join("、"));
</script>

<template>
  <div v-if="sections.length" class="overview-data-warning" role="status">
    <AlertTriangle :size="15" />
    <span>{{ failedLabels }}暂时无法加载，相关区域已重置，请稍后刷新。</span>
  </div>
</template>

<style scoped>
.overview-data-warning {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid color-mix(in srgb, var(--ds-warning) 30%, var(--ds-line));
  border-radius: var(--ds-radius-control);
  background: var(--ds-warning-soft);
  color: var(--ds-warning);
  font-size: 12px;
}

.overview-data-warning > svg {
  flex: 0 0 auto;
}
</style>
