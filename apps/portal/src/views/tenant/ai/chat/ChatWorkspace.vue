<script setup lang="ts">
import {
  createNamedConfirmDialog,
  notifyError,
  notifySuccess,
  notifyWarning
} from "@dai/app-core";
import {
  portalChatWorkspaceIconProps,
  PortalChatWorkspace,
  type PortalChatApi
} from "@dai/app-core/ai/chat";

import { runtimeChatApi, streamRuntimeChatMessage } from "../../../api/aiTenant";
import type { ChatMessageDTO, ChatModel, ChatSession } from "../../../types/aiTenant";

const chatApi: PortalChatApi<ChatModel, ChatSession, ChatMessageDTO> = {
  listModels: () => runtimeChatApi.listModels(),
  listSessions: () => runtimeChatApi.listSessions(),
  createSession: (payload) => runtimeChatApi.createSession(payload),
  getSession: (sessionId) => runtimeChatApi.getSession(sessionId),
  deleteSession: (sessionId) => runtimeChatApi.deleteSession(sessionId),
  streamMessage: (payload) => streamRuntimeChatMessage(payload)
};
const confirmDelete = createNamedConfirmDialog({
  title: "删除对话",
  message: (name) => `确定删除「${name}」吗？`,
  type: "warning",
  confirmButtonText: "删除",
  cancelButtonText: "取消"
});
</script>

<template>
  <PortalChatWorkspace
    :api="chatApi"
    usage-message="消耗计入租户用量。"
    source-label="租户网页对话"
    v-bind="portalChatWorkspaceIconProps"
    :notify-success="notifySuccess"
    :notify-warning="notifyWarning"
    :notify-error="notifyError"
    :confirm-delete="confirmDelete"
  />
</template>
