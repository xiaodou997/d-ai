<script setup lang="ts">
import {
  createNamedConfirmDialog,
  notifyError,
  notifySuccess
} from "@/platform";
import {
  portalAppManagementIconProps,
  PortalAppManagementWorkspace,
  type PortalAppApi
} from "@/platform/ai/apps";

import { aiTenantApi, runtimeChatApi, runtimeImageApi } from "@/api/aiTenant";
import type { ChatModel, ConsoleImageModel, TenantAppDTO, TenantAppPromptDTO, TenantAppPromptDetailDTO } from "@/api/types/aiTenant";

type TenantAppModel = ChatModel | ConsoleImageModel;

const agentApi: PortalAppApi<TenantAppPromptDTO, TenantAppPromptDetailDTO, TenantAppDTO, TenantAppModel> = {
  listTemplates: () => aiTenantApi.listTenantAppTemplates(),
  listApps: () => aiTenantApi.listTenantApps(),
  listPrompts: () => aiTenantApi.listTenantAppPrompts(),
  async listModels() {
    const [chatModels, imageModels] = await Promise.all([runtimeChatApi.listModels(), runtimeImageApi.listModels()]);
    return [...chatModels, ...imageModels];
  },
  getPrompt: (promptId) => aiTenantApi.getTenantAppPrompt(promptId),
  createApp: (payload) => aiTenantApi.createTenantApp(payload),
  updateApp: (appId, payload) => aiTenantApi.updateTenantApp(appId, payload),
  deleteApp: (appId) => aiTenantApi.deleteTenantApp(appId),
  setPublication: (appId, published) => aiTenantApi.setTenantAppPublication(appId, published),
  previewApp: (appId, payload) => aiTenantApi.previewTenantApp(appId, payload)
};
const confirmDelete = createNamedConfirmDialog({
  title: "删除应用",
  message: (name) => `确定删除租户应用「${name}」吗？`,
  type: "warning",
  confirmButtonText: "删除",
  cancelButtonText: "取消"
});
</script>

<template>
  <PortalAppManagementWorkspace
    :api="agentApi"
    scope="tenant"
    v-bind="portalAppManagementIconProps"
    :notify-success="notifySuccess"
    :notify-error="notifyError"
    :confirm-delete="confirmDelete"
  />
</template>
