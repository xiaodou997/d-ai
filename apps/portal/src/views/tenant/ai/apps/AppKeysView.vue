<!--
  租户应用运行密钥页:仅做 API/确认弹窗/文案适配,实际渲染在
  @/platform 的 PortalAppKeyWorkspace(DsUI 一体面板 + DsTable + DsPagination,
  本页始终以 embedded 模式嵌在 KeysView 的密钥管理面板内)。
-->
<script setup lang="ts">
import {
  confirmDialog,
  createNamedConfirmDialog,
  notifyError,
  notifySuccess,
  notifyWarning,
  resolvePortalPublicBaseUrl
} from "@/platform";
import {
  portalAppKeyWorkspaceIconProps,
  PortalAppKeyWorkspace,
  type PortalAppKeyApi
} from "@/platform/ai/appkeys";

import { aiTenantApi } from "@/api/aiTenant";
import { portalEnv } from "@/env";

defineProps<{
  embedded?: boolean;
}>();

const appKeyApi: PortalAppKeyApi = {
  listAppKeys: () => aiTenantApi.listRunKeys(),
  listVisibleAgents: () => aiTenantApi.listVisibleAgents(),
  createAppKey: (payload) => aiTenantApi.createRunKey(payload),
  updateAppKey: (appKeyId, payload) => aiTenantApi.updateRunKey(appKeyId, payload),
  deleteAppKey: (appKeyId) => aiTenantApi.deleteRunKey(appKeyId),
  revealAppKey: (appKeyId) => aiTenantApi.revealRunKey(appKeyId),
  rotateAppKey: (appKeyId) => aiTenantApi.rotateRunKey(appKeyId)
};

const publicBaseUrl = resolvePortalPublicBaseUrl(portalEnv.aiPublicBaseUrl);
const confirmDelete = createNamedConfirmDialog({
  title: "删除应用运行密钥",
  message: (name) => `确定删除应用运行密钥「${name}」吗？`,
  type: "warning",
  confirmButtonText: "删除",
  cancelButtonText: "取消"
});

const confirmRotate = () =>
  confirmDialog({
    title: "轮换应用运行密钥",
    message: "轮换后旧密钥立即失效，新密钥会立即生效，并且后续仍可再次查看复制。",
    confirmButtonText: "确定轮换",
    cancelButtonText: "取消",
    type: "warning"
  });
</script>

<template>
  <PortalAppKeyWorkspace
    :api="appKeyApi"
    :public-base-url="publicBaseUrl"
    chat-example-input="请帮我总结这段需求"
    name-placeholder="例如：客服聊天入口"
    v-bind="portalAppKeyWorkspaceIconProps"
    :notify-success="notifySuccess"
    :notify-error="notifyError"
    :notify-warning="notifyWarning"
    :confirm-delete="confirmDelete"
    :confirm-rotate="confirmRotate"
    :embedded="embedded"
  />
</template>
