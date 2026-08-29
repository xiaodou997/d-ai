<!-- 客户 API Key 页面壳；直接组合客户 API Key feature，保持路由层无业务编排。 -->
<script setup lang="ts">
import { PortalKeyManagementWorkspace } from "@/platform/ai/keys";
import CustomerApiKeysWorkspace from "./CustomerApiKeysWorkspace.vue";
</script>

<template>
  <PortalKeyManagementWorkspace title="API 密钥" eyebrow="智能服务 / 开发接入">
    <template #api><CustomerApiKeysWorkspace embedded /></template>
  </PortalKeyManagementWorkspace>
</template>
