<script setup lang="ts">
// 密钥管理(用户端):模型 API 密钥 + 应用运行密钥两个页签,实际渲染在
// app-core 的 PortalKeyManagementWorkspace(DsUI 一体面板),此处只做页签与路由 query 同步。
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";

import { PortalKeyManagementWorkspace, type PortalKeyManagementTab } from "@dai/app-core/ai/keys";

import ApiKeysView from "./ApiKeysView.vue";
import AppKeysView from "./AppKeysView.vue";

const route = useRoute();
const router = useRouter();

const activeTab = computed<PortalKeyManagementTab>({
  get: () => (route.query.tab === "application" ? "application" : "api"),
  set: (tab) => {
    void router.replace({
      query: {
        ...route.query,
        tab: tab === "application" ? "application" : undefined
      }
    });
  }
});
</script>

<template>
  <!-- 应用层分阶段开放：保留应用密钥工作区代码，现阶段仅展示模型 API 密钥。 -->
  <PortalKeyManagementWorkspace
    v-model:active-tab="activeTab"
    eyebrow="智能服务 / 开发接入"
    :show-application-keys="false"
  >
    <template #api><ApiKeysView embedded /></template>
    <template #application><AppKeysView embedded /></template>
  </PortalKeyManagementWorkspace>
</template>
