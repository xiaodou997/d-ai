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

import { aiTenantApi } from "@/api/aiTenant";
import type { TenantAppPromptDTO, TenantAppPromptDetailDTO } from "@/api/types/aiTenant";

const promptApi: PortalAppPromptApi<TenantAppPromptDTO, TenantAppPromptDetailDTO> = {
  listPrompts: () => aiTenantApi.listTenantAppPrompts(),
  getPrompt: (promptId) => aiTenantApi.getTenantAppPrompt(promptId),
  createPrompt: (payload) => aiTenantApi.createTenantAppPrompt(payload),
  updatePrompt: (promptId, payload) => aiTenantApi.updateTenantAppPrompt(promptId, payload),
  deletePrompt: (promptId) => aiTenantApi.deleteTenantAppPrompt(promptId)
};
const confirmDelete = createNamedConfirmDialog({
  title: "删除提示词",
  message: (name) => `确定删除租户提示词「${name}」吗？`,
  type: "warning",
  confirmButtonText: "删除",
  cancelButtonText: "取消"
});
</script>

<template>
  <PortalPromptManagementWorkspace
    :api="promptApi"
    scope="tenant"
    v-bind="portalAppManagementIconProps"
    :notify-success="notifySuccess"
    :notify-error="notifyError"
    :confirm-delete="confirmDelete"
  />
</template>
