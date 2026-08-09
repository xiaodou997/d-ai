<!--
  租户模型 API 密钥页:仅做 API/确认弹窗/文案适配,实际渲染在
  @/platform 的 PortalApiKeyWorkspace(DsUI 一体面板 + DsTable + DsPagination,
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
  portalApiKeyWorkspaceIconProps,
  PortalApiKeyWorkspace,
  type PortalApiKeyApi,
  type PortalApiKeyWriteInput
} from "@/platform/ai/apikeys";

import { aiTenantApi, statusOptions } from "@/api/aiTenant";
import { portalEnv } from "@/env";
import type { TenantAiApiKey, TenantAiApiKeyWriteRequest } from "@/api/types/aiTenant";

defineProps<{
  embedded?: boolean;
}>();

function normalizeWrite(payload: PortalApiKeyWriteInput): TenantAiApiKeyWriteRequest {
  return {
    name: payload.name,
    group_id: payload.group_id,
    quota_limit_micro_usd: payload.quota_limit_micro_usd ?? null,
    status: payload.status,
    expires_at: payload.expires_at ?? null,
    limit_policy: payload.limit_policy
      ? {
          concurrency_limit: payload.limit_policy.concurrency_limit ?? null,
          status: payload.limit_policy.status ?? undefined
        }
      : undefined
  };
}

const apiKeyApi: PortalApiKeyApi<TenantAiApiKey> = {
  listApiKeys: () => aiTenantApi.listApiKeys(),
  createApiKey: (payload) => aiTenantApi.createApiKey(normalizeWrite(payload)),
  updateApiKey: (apiKeyId, payload) => aiTenantApi.updateApiKey(apiKeyId, normalizeWrite(payload)),
  updateApiKeyStatus: (apiKeyId, status) => aiTenantApi.updateApiKeyStatus(apiKeyId, status),
  revealApiKey: (apiKeyId) => aiTenantApi.revealApiKey(apiKeyId),
  rotateApiKey: (apiKeyId) => aiTenantApi.rotateApiKey(apiKeyId),
  deleteApiKey: (apiKeyId) => aiTenantApi.deleteApiKey(apiKeyId),
  listGroups: () => aiTenantApi.listMyGroups()
};

const publicBaseUrl = resolvePortalPublicBaseUrl(portalEnv.apiBaseUrl);

const confirmDelete = createNamedConfirmDialog({
  title: "删除 API 密钥",
  message: (name) => `确定删除 API 密钥「${name}」？删除后无法恢复，且立即失效。`,
  confirmButtonText: "确定删除",
  cancelButtonText: "取消",
  type: "warning",
  confirmButtonClass: "el-button--danger"
});

const confirmRotate = () =>
  confirmDialog({
    title: "轮换 API 密钥",
    message: "轮换后旧密钥立即失效，新密钥会立即生效，并且后续仍可再次复制。",
    confirmButtonText: "确定轮换",
    cancelButtonText: "取消",
    type: "warning"
  });
</script>

<template>
  <PortalApiKeyWorkspace
    :api="apiKeyApi"
    title="租户模型 API 密钥"
    description="用于租户自用或匿名调用场景，每个密钥绑定一个分组，并独立管理额度和限流"
    eyebrow="智能服务 / 开发接入 / API 密钥"
    :public-base-url="publicBaseUrl"
    v-bind="portalApiKeyWorkspaceIconProps"
    :status-options="statusOptions"
    :notify-success="notifySuccess"
    :notify-error="notifyError"
    :notify-warning="notifyWarning"
    :confirm-delete="confirmDelete"
    :confirm-rotate="confirmRotate"
    :embedded="embedded"
  />
</template>
