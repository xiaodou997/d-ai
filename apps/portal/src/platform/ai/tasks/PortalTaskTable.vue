<!--
  任务中心列表(租户端/用户端共用)。
  重构:迁移至 DsUI——el-table → DsTable(嵌入面板 :frame="false"),任务编号 mono,
       状态/归属/只读 el-tag → DsTag,空态 DsEmpty,消耗列右对齐;
       「加载更多」底栏上移至 PortalTaskWorkspace 分页区,行双击查看详情的交互
       由「查看详情」按钮承接(DsTable 无行事件),其余业务逻辑不变。
-->
<script setup lang="ts">
import { computed } from "vue";
import { CircleClose, Delete, View } from "@element-plus/icons-vue";
import { DsEmpty, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

import {
  formatPortalTaskCredits,
  formatPortalTaskDuration,
  formatPortalTaskTime,
  portalTaskSourceLabel,
  portalTaskStatusLabel,
  portalTaskStatusTone,
  portalTaskTypeLabel,
  shortPortalTaskID
} from "./formatters";
import type { PortalTaskPortalMode, PortalTaskRecord } from "./types";

const props = defineProps<{
  tasks: readonly PortalTaskRecord[];
  mode: PortalTaskPortalMode;
  loading?: boolean;
  operationTaskId?: string;
}>();

const emit = defineEmits<{
  view: [task: PortalTaskRecord];
  cancel: [task: PortalTaskRecord];
  delete: [task: PortalTaskRecord];
}>();

// 归属列仅租户端展示
const columns = computed<DsTableColumn[]>(() => [
  { key: "id", title: "任务编号", width: 160, mono: true },
  ...(props.mode === "tenant" ? [{ key: "owner", title: "归属", width: 150 }] : []),
  { key: "type", title: "类型", width: 105 },
  { key: "source", title: "来源", width: 105 },
  { key: "model", title: "模型" },
  { key: "status", title: "状态", width: 100 },
  { key: "time", title: "时间", width: 170 },
  { key: "credits", title: "消耗", width: 100, align: "right" as const },
  { key: "error", title: "错误", width: 180 },
  { key: "actions", title: "操作", width: 140 }
]);
</script>

<template>
  <DsTable :frame="false" :columns="columns" :rows="[...props.tasks]" row-key="id" :loading="props.loading">
    <template #empty>
      <DsEmpty title="暂无任务" description="当前筛选条件下没有任务记录" />
    </template>
    <template #cell-id="{ row }">
      <el-tooltip :content="row.id">
        <span>{{ shortPortalTaskID(row.id) }}</span>
      </el-tooltip>
    </template>
    <template #cell-owner="{ row }">
      <div class="task-table__owner">
        <DsTag :tone="row.owner.scope === 'user' ? 'warning' : 'neutral'">
          {{ row.owner.scope === "user" ? "用户" : "租户" }}
        </DsTag>
        <span v-if="row.owner.user_id" class="task-table__muted">{{ row.owner.user_id }}</span>
      </div>
    </template>
    <template #cell-type="{ row }">{{ portalTaskTypeLabel(row.type) }}</template>
    <template #cell-source="{ row }">{{ portalTaskSourceLabel(row.source) }}</template>
    <template #cell-model="{ row }">{{ row.model || "-" }}</template>
    <template #cell-status="{ row }">
      <DsTag :tone="portalTaskStatusTone(row.status)">
        {{ portalTaskStatusLabel(row.status) }}
      </DsTag>
    </template>
    <template #cell-time="{ row }">
      <div>{{ formatPortalTaskTime(row.created_at) }}</div>
      <span class="task-table__muted">{{ formatPortalTaskDuration(row) }}</span>
    </template>
    <template #cell-credits="{ row }">{{ formatPortalTaskCredits(row.usage?.cost_credits) }}</template>
    <template #cell-error="{ row }">
      <span :class="row.error ? 'task-table__error' : 'task-table__muted'">
        {{ row.error?.message || "-" }}
      </span>
    </template>
    <template #cell-actions="{ row }">
      <div class="task-table__actions">
        <el-tooltip content="查看详情">
          <el-button text :icon="View" aria-label="查看任务详情" @click.stop="emit('view', row)" />
        </el-tooltip>
        <el-tooltip v-if="row.permissions.can_cancel" content="取消任务">
          <el-button
            text
            type="warning"
            :icon="CircleClose"
            :loading="props.operationTaskId === row.id"
            aria-label="取消任务"
            @click.stop="emit('cancel', row)"
          />
        </el-tooltip>
        <el-tooltip v-if="row.permissions.can_delete" content="删除任务">
          <el-button
            text
            type="danger"
            :icon="Delete"
            :loading="props.operationTaskId === row.id"
            aria-label="删除任务"
            @click.stop="emit('delete', row)"
          />
        </el-tooltip>
        <DsTag v-if="row.permissions.read_only" tone="neutral">只读</DsTag>
      </div>
    </template>
  </DsTable>
</template>

<style scoped>
.task-table__owner,
.task-table__actions {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.task-table__muted {
  color: var(--ds-muted);
  font-size: 12px;
}

.task-table__error {
  color: var(--ds-danger);
}
</style>
