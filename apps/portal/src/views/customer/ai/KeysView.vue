<script setup lang="ts">
// 用户 API 密钥页：应用密钥能力不再向用户开发中心开放。
import { PortalKeyManagementWorkspace } from "@/platform/ai/keys";

import ApiKeysView from "./ApiKeysView.vue";
</script>

<template>
  <PortalKeyManagementWorkspace
    title="API 密钥"
    eyebrow="智能服务 / 开发接入"
  >
    <template #api><ApiKeysView embedded /></template>
  </PortalKeyManagementWorkspace>
</template>
