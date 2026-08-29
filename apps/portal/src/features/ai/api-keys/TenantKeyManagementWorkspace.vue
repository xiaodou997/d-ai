<!-- 租户 API Key 页面壳；tab action provider 与 API Key feature 组合归属同一业务边界。 -->
<script setup lang="ts">
import { provide, reactive } from "vue";

import { PortalKeyManagementWorkspace, type PortalKeyManagementTab } from "@/platform/ai/keys";
import TenantApiKeysWorkspace from "./TenantApiKeysWorkspace.vue";

const tabActions = reactive<Record<PortalKeyManagementTab, { loading: boolean; refresh: () => void; create: () => void }>>({
  api: { loading: false, refresh: () => {}, create: () => {} }
});
provide("keyManagementTabActions", tabActions);
</script>

<template>
  <PortalKeyManagementWorkspace title="API 密钥" eyebrow="智能服务 / 开发接入">
    <template #api><TenantApiKeysWorkspace embedded /></template>
  </PortalKeyManagementWorkspace>
</template>
