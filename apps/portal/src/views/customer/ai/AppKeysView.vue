<script setup lang="ts">
import {
  confirmDialog,
  createNamedConfirmDialog,
  notifyError,
  notifySuccess,
  notifyWarning,
  resolvePortalPublicBaseUrl
} from "@dai/app-core";
import {
  portalAppKeyWorkspaceIconProps,
  PortalAppKeyWorkspace,
  type PortalAppKeyApi
} from "@dai/app-core/ai/app-keys";

import { aiCustomerApi } from "../../api/aiCustomer";
import { portalEnv } from "../../env";

defineProps<{
  embedded?: boolean;
}>();

const appKeyApi: PortalAppKeyApi = {
  listAppKeys: () => aiCustomerApi.listRunKeys(),
  listVisibleAgents: () => aiCustomerApi.listVisibleAgents(),
  createAppKey: (payload) => aiCustomerApi.createRunKey(payload),
  updateAppKey: (appKeyId, payload) => aiCustomerApi.updateRunKey(appKeyId, payload),
  deleteAppKey: (appKeyId) => aiCustomerApi.deleteRunKey(appKeyId),
  revealAppKey: (appKeyId) => aiCustomerApi.revealRunKey(appKeyId),
  rotateAppKey: (appKeyId) => aiCustomerApi.rotateRunKey(appKeyId)
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
    chat-example-input="请帮我总结这段内容"
    name-placeholder="例如：个人知识库入口"
    v-bind="portalAppKeyWorkspaceIconProps"
    :notify-success="notifySuccess"
    :notify-error="notifyError"
    :notify-warning="notifyWarning"
    :confirm-delete="confirmDelete"
    :confirm-rotate="confirmRotate"
    :embedded="embedded"
  />
</template>
