<!--
  AI 工作台异常与质量信号区:最近请求错误 + 样本用户消耗 Top。
  重构:迁移至 DsUI——TenantWorkbenchSection(自研卡片)→ AiWorkbenchSection 分区,
       错误列表 el-table → DsTable(子面板内 :frame="false"),状态 el-tag → DsTag。
-->
<script setup lang="ts">
import { ArrowRight, Loading } from "@element-plus/icons-vue";
import { useRouter } from "vue-router";
import { DsEmpty, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

import AiWorkbenchSection from "./AiWorkbenchSection.vue";
import AiUsageUserInsight from "./AiUsageUserInsight.vue";
import type { TenantAiDashboardRecentError } from "@/api/types/aiTenant";

interface UserInsightItem {
  key: string;
  userLabel: string;
  totalCredits: number;
  creditsText: string;
  requestCount: number;
  successRateText: string;
  lastActiveText: string;
}

const props = defineProps<{
  recentErrors: TenantAiDashboardRecentError[];
  errorsLoading: boolean;
  userInsights: UserInsightItem[];
  usersLoading: boolean;
  rangeLabel: string;
}>();

const router = useRouter();

const errorColumns: DsTableColumn[] = [
  { key: "time", title: "时间", width: 150 },
  { key: "model_code", title: "模型", width: 120, mono: true },
  { key: "status", title: "状态", width: 90 },
  { key: "error_message", title: "错误信息" }
];

const formatTime = (ts?: number | null) => {
  if (!ts) return "—";
  return new Date(ts).toLocaleString("zh-CN");
};

const errorTone = (value: string): "positive" | "danger" | "warning" | "neutral" =>
  (({ success: "positive", failed: "danger", rejected: "warning", partial: "warning" } as Record<string, "positive" | "danger" | "warning">)[value] || "neutral");
</script>

<template>
  <AiWorkbenchSection
    eyebrow="Quality Signals"
    title="异常与质量信号"
    :description="`把 ${props.rangeLabel} 的失败请求和调用样本中的高消耗用户放在同一区块，快速判断哪里不稳、谁最值得盯。`"
  >
    <template #actions>
      <el-button text type="primary" class="!text-xs font-bold" @click="router.push('/workspace/user-consumption')">
        查看更多信号 <el-icon class="ml-1"><ArrowRight /></el-icon>
      </el-button>
    </template>

    <div class="ai-quality-grid">
      <div class="qs-panel">
        <div class="qs-panel__head">
          <div>
            <h3 class="qs-panel__title">最近请求错误</h3>
            <p class="qs-panel__desc">{{ props.rangeLabel }}请求失败记录</p>
          </div>
        </div>

        <div v-if="errorsLoading" class="qs-panel__loading">
          <el-icon class="qs-panel__spinner animate-spin" :size="32"><Loading /></el-icon>
        </div>

        <DsTable v-else :frame="false" :columns="errorColumns" :rows="recentErrors" row-key="request_id">
          <template #empty>
            <DsEmpty title="暂无错误记录" />
          </template>
          <template #cell-time="{ row }">{{ formatTime(row.created_at) }}</template>
          <template #cell-status="{ row }">
            <DsTag :tone="errorTone(row.request_status)">{{ row.request_status }}</DsTag>
          </template>
        </DsTable>
      </div>

      <AiUsageUserInsight :loading="usersLoading" :items="userInsights" />
    </div>
  </AiWorkbenchSection>
</template>

<style scoped>
.ai-quality-grid {
  display: grid;
  grid-template-columns: repeat(1, minmax(0, 1fr));
  gap: 16px;
}

@media (min-width: 1280px) {
  .ai-quality-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

.qs-panel {
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.qs-panel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--ds-line);
  padding: 18px 24px;
}

.qs-panel__title {
  margin: 0;
  color: var(--ds-ink);
  font-size: 15px;
  font-weight: 650;
}

.qs-panel__desc {
  margin: 2px 0 0;
  color: var(--ds-faint);
  font-size: 12px;
}

.qs-panel__loading {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 0;
}

.qs-panel__spinner {
  color: var(--ds-faint);
}
</style>
