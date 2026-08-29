<!-- 租户任务中心适配层；任务过滤、轮询和表格由共享 PortalTaskWorkspace 负责。 -->
<script setup lang="ts">
import { confirmDialog, notifyError, notifySuccess } from "@/platform";
import {
  PortalTaskWorkspace,
  portalTaskTypeLabel,
  type PortalTaskApi,
  type PortalTaskRecord
} from "@/platform/ai/tasks";

import { runtimeTaskApi } from "@/api/aiTenant";

const taskApi: PortalTaskApi = {
  listTasks: (query) => runtimeTaskApi.listTasks(query),
  getTask: (taskId) => runtimeTaskApi.getTask(taskId),
  cancelTask: (taskId) => runtimeTaskApi.cancelTask(taskId),
  deleteTask: (taskId) => runtimeTaskApi.deleteTask(taskId)
};

function confirmDelete(task: PortalTaskRecord): Promise<boolean> {
  return confirmDialog({
    title: "删除任务",
    message: `确定删除${portalTaskTypeLabel(task.type)}任务「${task.id}」吗？此操作不可恢复。`,
    confirmButtonText: "删除",
    cancelButtonText: "取消",
    type: "warning",
    confirmButtonClass: "el-button--danger"
  });
}
</script>

<template>
  <PortalTaskWorkspace
    mode="tenant"
    eyebrow="智能服务 / 任务管理"
    :api="taskApi"
    :notify-success="notifySuccess"
    :notify-error="notifyError"
    :confirm-delete="confirmDelete"
  />
</template>
