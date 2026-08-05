<script setup lang="ts">
import {
  notifyError,
  notifySuccess
} from "@dai/app-core";
import {
  portalImageWorkspaceIconProps,
  PortalImageStudioWorkspace,
  type PortalImageApi
} from "@dai/app-core/ai/images";

import { formatCredits, runtimeImageApi } from "../../api/aiCustomer";
import { serviceBaseUrl } from "../../api/request";

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
    :format-credits="formatCredits"
    usage-message="消耗会计入个人用量"
    :asset-base-url="serviceBaseUrl('ai')"
    v-bind="portalImageWorkspaceIconProps"
    :notify-success="notifySuccess"
    :notify-error="notifyError"
  />
</template>
