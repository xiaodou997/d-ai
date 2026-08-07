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

import { aiCustomerApi, formatCredits, formatWholeCredits, statusOptions } from "@/api/aiCustomer";
import { portalEnv } from "@/env";
import type { AiApiKey, AiApiKeyWriteRequest } from "@/api/types/aiCustomer";

defineProps<{
  embedded?: boolean;
}>();

function normalizeWrite(payload: PortalApiKeyWriteInput): AiApiKeyWriteRequest {
  return {
    name: payload.name,
    group_id: payload.group_id,
    quota_limit_credits: payload.quota_limit_credits ?? null,
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

const apiKeyApi: PortalApiKeyApi<AiApiKey> = {
  listApiKeys: () => aiCustomerApi.listApiKeys(),
  createApiKey: (payload) => aiCustomerApi.createApiKey(normalizeWrite(payload)),
  updateApiKey: (apiKeyId, payload) => aiCustomerApi.updateApiKey(apiKeyId, normalizeWrite(payload)),
  updateApiKeyStatus: (apiKeyId, status) => aiCustomerApi.updateApiKeyStatus(apiKeyId, status),
  revealApiKey: (apiKeyId) => aiCustomerApi.revealApiKey(apiKeyId),
  rotateApiKey: (apiKeyId) => aiCustomerApi.rotateApiKey(apiKeyId),
  deleteApiKey: (apiKeyId) => aiCustomerApi.deleteApiKey(apiKeyId),
  listGroups: () => aiCustomerApi.listMyGroups()
};

const publicBaseUrl = resolvePortalPublicBaseUrl(portalEnv.publicBaseUrl);

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
    title="我的模型 API 密钥"
    description="从“我的分组”里选择一个有权使用的分组，统一配置个人 API 密钥的额度和独立限流"
    eyebrow="智能服务 / 开发接入 / API 密钥"
    :public-base-url="publicBaseUrl"
    v-bind="portalApiKeyWorkspaceIconProps"
    :status-options="statusOptions"
    :format-credits="formatCredits"
    :format-whole-credits="formatWholeCredits"
    :notify-success="notifySuccess"
    :notify-error="notifyError"
    :notify-warning="notifyWarning"
    :confirm-delete="confirmDelete"
    :confirm-rotate="confirmRotate"
    :embedded="embedded"
  />
</template>
