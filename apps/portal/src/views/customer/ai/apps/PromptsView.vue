<script setup lang="ts">
import {
  createNamedConfirmDialog,
  notifyError,
  notifySuccess
} from "@/platform";
import {
  PortalPromptManagementWorkspace,
  portalAppManagementIconProps,
  type PortalAppPromptApi
} from "@/platform/ai/apps";

import { aiCustomerApi } from "@/api/aiCustomer";
import type { UserAppPromptDTO, UserAppPromptDetailDTO } from "@/api/types/aiCustomer";

const promptApi: PortalAppPromptApi<UserAppPromptDTO, UserAppPromptDetailDTO> = {
  listPrompts: () => aiCustomerApi.listUserAppPrompts(),
  getPrompt: (promptId) => aiCustomerApi.getUserAppPrompt(promptId),
  createPrompt: (payload) => aiCustomerApi.createUserAppPrompt(payload),
  updatePrompt: (promptId, payload) => aiCustomerApi.updateUserAppPrompt(promptId, payload),
  deletePrompt: (promptId) => aiCustomerApi.deleteUserAppPrompt(promptId)
};
const confirmDelete = createNamedConfirmDialog({
  title: "删除提示词",
  message: (name) => `确定删除我的提示词「${name}」吗？`,
  type: "warning",
  confirmButtonText: "删除",
  cancelButtonText: "取消"
});
</script>

<template>
  <PortalPromptManagementWorkspace
    :api="promptApi"
    scope="user"
    v-bind="portalAppManagementIconProps"
    :notify-success="notifySuccess"
    :notify-error="notifyError"
    :confirm-delete="confirmDelete"
  />
</template>
