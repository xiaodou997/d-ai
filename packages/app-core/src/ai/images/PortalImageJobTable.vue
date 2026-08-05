<!--
  生图任务记录表(工作台/最近生图区块共用)。
  重构:迁移至 DsUI——el-table → DsTable(嵌入面板 :frame="false"),el-tag → DsTag,
       空态 DsEmpty;外层 320px 滚动容器保留原版式,业务逻辑不变。
-->
<script setup lang="ts">
import { DsEmpty, DsTable, DsTag, type DsTableColumn } from "@dai/ui";

import type { PortalImageJobRecord } from "./types";

const props = defineProps<{
  jobs: PortalImageJobRecord[];
  formatCredits: (value: number | null | undefined) => string;
}>();

const columns: DsTableColumn[] = [
  { key: "operation", title: "操作", width: 100 },
  { key: "model_code", title: "模型", width: 160, mono: true },
  { key: "prompt", title: "提示词" },
  { key: "size", title: "规格", width: 180 },
  { key: "status", title: "状态", width: 100 },
  { key: "archive", title: "归档", width: 100 },
  { key: "result", title: "结果摘要", width: 150 },
  { key: "credits", title: "消耗", width: 110, align: "right" },
  { key: "time", title: "时间", width: 180 }
];

function formatTimestamp(value?: number) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}

function archivePolicyLabel(job: PortalImageJobRecord) {
  return job.raw_image_retained ? "保留原图" : "仅摘要";
}

function resultSummary(job: PortalImageJobRecord) {
  const parts = [`${job.image_count || 0} 张`];
  if (job.inline_count) parts.push(`inline ${job.inline_count}`);
  if (job.url_count) parts.push(`url ${job.url_count}`);
  return parts.join(" / ");
}
</script>

<template>
  <div class="image-job-table">
    <DsTable :frame="false" :columns="columns" :rows="jobs" row-key="id">
      <template #empty>
        <DsEmpty title="暂无生图任务" />
      </template>
      <template #cell-operation="{ row }">
        <DsTag :tone="row.operation === 'edit' ? 'warning' : 'positive'">
          {{ row.operation === "edit" ? "参考图" : "文生图" }}
        </DsTag>
      </template>
      <template #cell-prompt="{ row }">
        <span>{{ row.prompt || "-" }}</span>
      </template>
      <template #cell-size="{ row }">
        <span>{{ row.size || "-" }}</span>
      </template>
      <template #cell-status="{ row }">
        <DsTag :tone="row.status === 'completed' ? 'positive' : 'danger'">
          {{ row.status === "completed" ? "完成" : "失败" }}
        </DsTag>
      </template>
      <template #cell-archive="{ row }">
        <DsTag :tone="row.raw_image_retained ? 'positive' : 'warning'">
          {{ archivePolicyLabel(row) }}
        </DsTag>
      </template>
      <template #cell-result="{ row }">
        <span>{{ resultSummary(row) }}</span>
      </template>
      <template #cell-credits="{ row }">{{ props.formatCredits(row.caller_charge_credits) }}</template>
      <template #cell-time="{ row }">
        <span class="image-job-table__time">{{ formatTimestamp(row.created_at) }}</span>
      </template>
    </DsTable>
  </div>
</template>

<style scoped>
.image-job-table {
  max-height: 320px;
  overflow: auto;
}

.image-job-table__time {
  font-size: 12px;
  color: var(--ds-muted);
}
</style>
