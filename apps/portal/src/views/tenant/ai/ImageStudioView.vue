<script setup lang="ts">
import {
  notifyError,
  notifySuccess
} from "@/platform";
import {
  portalImageWorkspaceIconProps,
  PortalImageStudioWorkspace,
  type PortalImageApi
} from "@/platform/ai/images";

import { formatUSD, runtimeImageApi } from "@/api/aiTenant";
import { apiBaseUrl } from "@/api/request";

const imageApi: PortalImageApi = {
  listModels: () => runtimeImageApi.listModels(),
  listJobs: () => runtimeImageApi.listJobs(),
  createTask: (payload) => runtimeImageApi.createTask(payload),
  getTask: (taskId) => runtimeImageApi.getTask(taskId),
  cancelTask: (taskId) => runtimeImageApi.cancelTask(taskId),
  deleteTask: (taskId) => runtimeImageApi.deleteTask(taskId)
};
</script>

<template>
  <PortalImageStudioWorkspace
    :api="imageApi"
    :format-u-s-d="formatUSD"
    usage-message="消耗会计入租户用量"
    :asset-base-url="apiBaseUrl"
    v-bind="portalImageWorkspaceIconProps"
    :notify-success="notifySuccess"
    :notify-error="notifyError"
  />
</template>
