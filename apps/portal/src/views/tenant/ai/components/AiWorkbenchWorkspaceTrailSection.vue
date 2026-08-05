<!--
  AI 工作台最近工作记录区:最近对话 + 最近生图。
  重构:迁移至 DsUI——TenantWorkbenchSection(自研卡片)→ AiWorkbenchSection 分区,
       会话列表 el-table → DsTable(子面板内 :frame="false"),空态 DsEmpty;
       生图表复用已迁移的 PortalImageJobTable(DsTable 版)。
-->
<script setup lang="ts">
import { PortalImageJobTable } from "@/platform/ai/images";
import { Loading } from "@element-plus/icons-vue";
import { DsEmpty, DsTable, type DsTableColumn } from "@/shared/ui";

import AiWorkbenchSection from "./AiWorkbenchSection.vue";
import { formatCredits } from "@/api/aiTenant";
import type { ChatSession, ConsoleImageJob } from "@/api/types/aiTenant";

defineProps<{
  sessions: ChatSession[];
  jobs: ConsoleImageJob[];
  loading: boolean;
}>();

const sessionColumns: DsTableColumn[] = [
  { key: "title", title: "标题" },
  { key: "model", title: "目标", width: 160 },
  { key: "updated", title: "更新时间", width: 180 }
];

const formatTime = (ts?: number | null) => {
  if (!ts) return "—";
  return new Date(ts).toLocaleString("zh-CN");
};
</script>

<template>
  <AiWorkbenchSection
    eyebrow="Workspace Trail"
    title="最近工作记录"
    description="保留用户最近的对话和生图任务，这一块不跟随上方分析窗口切换，专门承接真实工作流。"
  >
    <div class="ai-trail-grid">
      <div class="wt-panel">
        <div class="wt-panel__head">
          <div>
            <h3 class="wt-panel__title">最近对话</h3>
            <p class="wt-panel__desc">工作台统一入口同步的最近会话</p>
          </div>
        </div>

        <div v-if="loading && !sessions.length" class="wt-panel__loading">
          <el-icon class="wt-panel__spinner animate-spin" :size="32"><Loading /></el-icon>
        </div>

        <DsTable v-else :frame="false" :columns="sessionColumns" :rows="sessions" row-key="id">
          <template #empty>
            <DsEmpty title="暂无数据" />
          </template>
          <template #cell-title="{ row }">
            <span class="wt-panel__strong">{{ row.title || "新对话" }}</span>
          </template>
          <template #cell-model="{ row }">
            {{ row.model_code || "-" }}
          </template>
          <template #cell-updated="{ row }">
            <span class="wt-panel__time">{{ formatTime(row.updated_at) }}</span>
          </template>
        </DsTable>
      </div>

      <div class="wt-panel">
        <div class="wt-panel__head">
          <div>
            <h3 class="wt-panel__title">最近生图</h3>
            <p class="wt-panel__desc">工作台统一入口同步的最近任务</p>
          </div>
        </div>

        <div v-if="loading && !jobs.length" class="wt-panel__loading">
          <el-icon class="wt-panel__spinner animate-spin" :size="32"><Loading /></el-icon>
        </div>

        <PortalImageJobTable v-else :jobs="jobs" :format-credits="formatCredits" />
      </div>
    </div>
  </AiWorkbenchSection>
</template>

<style scoped>
.ai-trail-grid {
  display: grid;
  grid-template-columns: repeat(1, minmax(0, 1fr));
  gap: 16px;
}

@media (min-width: 1280px) {
  .ai-trail-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

.wt-panel {
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.wt-panel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--ds-line);
  padding: 18px 24px;
}

.wt-panel__title {
  margin: 0;
  color: var(--ds-ink);
  font-size: 15px;
  font-weight: 650;
}

.wt-panel__desc {
  margin: 2px 0 0;
  color: var(--ds-faint);
  font-size: 12px;
}

.wt-panel__loading {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 0;
}

.wt-panel__spinner {
  color: var(--ds-faint);
}

.wt-panel__strong {
  font-weight: 700;
  color: var(--ds-ink);
}

.wt-panel__time {
  font-size: 12px;
  color: var(--ds-muted);
}
</style>
