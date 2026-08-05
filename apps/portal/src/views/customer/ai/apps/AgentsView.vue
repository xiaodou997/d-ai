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

import { aiCustomerApi, runtimeChatApi, runtimeImageApi } from "@/api/aiCustomer";
import type { ChatModel, ConsoleImageModel, UserAppDTO, UserAppPromptDTO, UserAppPromptDetailDTO } from "@/api/types/aiCustomer";

type UserAppModel = ChatModel | ConsoleImageModel;

const agentApi: PortalAppApi<UserAppPromptDTO, UserAppPromptDetailDTO, UserAppDTO, UserAppModel> = {
  listTemplates: () => aiCustomerApi.listUserAppTemplates(),
  listApps: () => aiCustomerApi.listUserApps(),
  listPrompts: () => aiCustomerApi.listUserAppPrompts(),
  async listModels() {
    const [chatModels, imageModels] = await Promise.all([runtimeChatApi.listModels(), runtimeImageApi.listModels()]);
    return [...chatModels, ...imageModels];
  },
  getPrompt: (promptId) => aiCustomerApi.getUserAppPrompt(promptId),
  createApp: (payload) => aiCustomerApi.createUserApp(payload),
  updateApp: (appId, payload) => aiCustomerApi.updateUserApp(appId, payload),
  deleteApp: (appId) => aiCustomerApi.deleteUserApp(appId),
  previewApp: (appId, payload) => aiCustomerApi.previewUserApp(appId, payload)
};
const confirmDelete = createNamedConfirmDialog({
  title: "删除应用",
  message: (name) => `确定删除我的应用「${name}」吗？`,
  type: "warning",
  confirmButtonText: "删除",
  cancelButtonText: "取消"
});
</script>

<template>
  <PortalAppManagementWorkspace
    :api="agentApi"
    scope="user"
    v-bind="portalAppManagementIconProps"
    :notify-success="notifySuccess"
    :notify-error="notifyError"
    :confirm-delete="confirmDelete"
  />
</template>
