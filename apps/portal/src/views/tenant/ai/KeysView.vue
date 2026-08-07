<!-- 租户 API 密钥页：应用密钥能力不再向租户开发中心开放。 -->
<script setup lang="ts">
import { provide, reactive } from "vue";

import { PortalKeyManagementWorkspace, type PortalKeyManagementTab } from "@/platform/ai/keys";

import ApiKeysView from "./ApiKeysView.vue";

// KeysView 是 ApiKeysView 的创建者，provide 必须放在这里，嵌入工作区才能 inject 到。
const tabActions = reactive<Record<PortalKeyManagementTab, { loading: boolean; refresh: () => void; create: () => void }>>({
  api: { loading: false, refresh: () => {}, create: () => {} }
});
provide("keyManagementTabActions", tabActions);
</script>

<template>
  <PortalKeyManagementWorkspace
    title="API 密钥"
    eyebrow="智能服务 / 开发接入"
  >
    <template #api><ApiKeysView embedded /></template>
  </PortalKeyManagementWorkspace>
</template>
