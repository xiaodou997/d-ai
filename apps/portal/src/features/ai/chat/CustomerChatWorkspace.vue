<!--
  客户 AI 对话工作区适配层：将客户 runtime facade 与共享 PortalChatWorkspace 组合，
  具体会话状态和渲染继续由 platform/ai/chat 负责。
-->
<script setup lang="ts">
import {
  createNamedConfirmDialog,
  notifyError,
  notifySuccess,
  notifyWarning
} from "@/platform";
import {
  portalChatWorkspaceIconProps,
  PortalChatWorkspace,
  type PortalChatApi
} from "@/platform/ai/chat";

import { runtimeChatApi, streamRuntimeChatMessage } from "@/api/aiCustomer";
import type { ChatMessageDTO, ChatModel, ChatSession } from "@/api/types/aiCustomer";

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
    usage-message="消耗计入个人用量。"
    source-label="个人网页对话"
    v-bind="portalChatWorkspaceIconProps"
    :notify-success="notifySuccess"
    :notify-warning="notifyWarning"
    :notify-error="notifyError"
    :confirm-delete="confirmDelete"
  />
</template>
