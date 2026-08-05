<!--
  密钥管理页(智能服务/应用与密钥):本身只是容器,按 ?tab 在「模型 API 密钥」与
  「应用运行密钥」两个 embedded 工作区之间切换,实际渲染在
  @dai/app-core 的 PortalKeyManagementWorkspace(DsUI 一体面板 + DsTabs)。
  每把模型密钥的接入配置由行内操作打开，避免页头说明和具体密钥脱节。
-->
<script setup lang="ts">
import { computed, provide, reactive } from "vue";
import { useRoute, useRouter } from "vue-router";

import { PortalKeyManagementWorkspace, type PortalKeyManagementTab } from "@dai/app-core/ai/keys";

import ApiKeysView from "./ApiKeysView.vue";
import AppKeysView from "./apps/AppKeysView.vue";

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

// KeysView 是 slot 内容(ApiKeysView/AppKeysView)的创建者,组件实例父链经过这里;
// provide 必须放在这里,子组件(含 slot 里的嵌入工作区)才能 inject 到。
const tabActions = reactive<Record<PortalKeyManagementTab, { loading: boolean; refresh: () => void; create: () => void }>>({
  api: { loading: false, refresh: () => {}, create: () => {} },
  application: { loading: false, refresh: () => {}, create: () => {} }
});
provide("keyManagementTabActions", tabActions);
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
