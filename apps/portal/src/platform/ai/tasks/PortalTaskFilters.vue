<!--
  任务中心筛选条(租户端/用户端共用)。
  重构:迁移至 DsUI——PortalFilterBar → DsFilterBar + DsFilterField,
       el-select 固定 160px 宽,el-segmented 选中色改走 var(--ds-*) token;
       筛选/刷新交互与查询参数不变。
-->
<script setup lang="ts">
import { Refresh, Search } from "@element-plus/icons-vue";
import { DsFilterBar, DsFilterField } from "@/shared/ui";

import type { PortalTaskOwnerScope, PortalTaskPortalMode, PortalTaskStatus, PortalTaskType } from "./types";

const props = defineProps<{ mode: PortalTaskPortalMode; loading?: boolean }>();
const emit = defineEmits<{ search: []; refresh: [] }>();

const ownerScope = defineModel<"" | PortalTaskOwnerScope>("ownerScope", { default: "" });
const userID = defineModel<string>("userId", { default: "" });
const status = defineModel<"" | PortalTaskStatus>("status", { default: "" });
const taskType = defineModel<"" | PortalTaskType>("taskType", { default: "" });

const ownerOptions = [
  { label: "全部任务", value: "" },
  { label: "租户任务", value: "tenant" },
  { label: "用户任务", value: "user" }
];

function applySelect(): void {
  if (ownerScope.value !== "user") userID.value = "";
  emit("search");
}
</script>

<template>
  <DsFilterBar>
    <DsFilterField v-if="props.mode === 'tenant'" label="归属">
      <el-segmented v-model="ownerScope" :options="ownerOptions" @change="applySelect" />
    </DsFilterField>
    <DsFilterField v-if="props.mode === 'tenant' && ownerScope === 'user'" label="用户编号">
      <el-input
        v-model="userID"
        clearable
        placeholder="用户编号"
        class="task-filter__user"
        @keyup.enter="emit('search')"
        @clear="emit('search')"
      />
    </DsFilterField>
    <DsFilterField label="状态">
      <el-select v-model="status" class="task-filter__select" placeholder="全部状态" @change="emit('search')">
        <el-option label="全部状态" value="" />
        <el-option label="待执行" value="pending" />
        <el-option label="执行中" value="running" />
        <el-option label="已完成" value="completed" />
        <el-option label="失败" value="failed" />
        <el-option label="已取消" value="cancelled" />
      </el-select>
    </DsFilterField>
    <DsFilterField label="类型">
      <el-select v-model="taskType" class="task-filter__select" placeholder="全部类型" @change="emit('search')">
        <el-option label="全部类型" value="" />
        <el-option label="AI 对话" value="chat.completions" />
        <el-option label="文生图" value="images.generation" />
        <el-option label="图生图" value="images.edit" />
      </el-select>
    </DsFilterField>

    <template #actions>
      <el-tooltip content="查询">
        <el-button :icon="Search" aria-label="查询任务" @click="emit('search')" />
      </el-tooltip>
      <el-tooltip content="刷新">
        <el-button :icon="Refresh" :loading="props.loading" aria-label="刷新任务" @click="emit('refresh')" />
      </el-tooltip>
    </template>
  </DsFilterBar>
</template>

<style scoped>
.task-filter__select {
  width: 160px;
}

.task-filter__user {
  width: 220px;
}

:deep(.el-segmented) {
  --el-segmented-item-selected-bg-color: var(--ds-accent);
  --el-segmented-item-selected-color: var(--ds-accent-contrast);
}

@media (max-width: 768px) {
  .task-filter__select,
  .task-filter__user {
    width: min(100%, 220px);
  }
}
</style>
