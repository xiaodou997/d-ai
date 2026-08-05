<!--
  密钥管理工作区(租户端/用户端共用):默认在模型 API 密钥与应用运行密钥间切换，
  应用层分阶段开放时可仅显示模型 API 密钥。
  重构:迁移至新设计系统一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行),
       自研密钥类型卡片切换 → DsTabs,面包屑由 eyebrow("/" 分隔)+ title 推导;
       两个密钥工作区仍以 embedded 模式经 slot 嵌入,业务逻辑与 slots 保持兼容。
-->
<script setup lang="ts">
import { computed, inject, watch } from "vue";
import { KeyRound, Plus, RefreshCw } from "lucide-vue-next";
import { DsTabs } from "@/shared/ui";

import PortalPagePanel from "../../page/PortalPagePanel.vue";

export type PortalKeyManagementTab = "api" | "application";

interface TabActionState {
  loading: boolean;
  refresh: () => void;
  create: () => void;
}

// 由消费方(如租户端 KeysView)provide;为 null 时不渲染页头按钮,操作按钮留在 embedded 条带
const tabActions = inject<Record<PortalKeyManagementTab, TabActionState> | null>(
  "keyManagementTabActions",
  null
);

const props = withDefaults(
  defineProps<{
    title?: string;
    description?: string;
    eyebrow?: string;
    showApplicationKeys?: boolean;
  }>(),
  {
    title: "密钥管理",
    eyebrow: "智能服务 / 开发接入",
    showApplicationKeys: true
  }
);

const activeTab = defineModel<PortalKeyManagementTab>("activeTab", { default: "api" });

const resolvedActiveTab = computed<PortalKeyManagementTab>(() =>
  props.showApplicationKeys && activeTab.value === "application" ? "application" : "api"
);

const panelDescription = computed(() =>
  props.description
  ?? (props.showApplicationKeys
    ? "按接入方式创建密钥。两类密钥不能互换使用。"
    : "创建并管理用于直接调用模型的 API 密钥。")
);

// 面包屑 = eyebrow 各段 + 页面标题(末级)
const breadcrumbs = computed(() => [
  ...props.eyebrow
    .split("/")
    .map((segment) => segment.trim())
    .filter(Boolean)
    .map((label) => ({ label })),
  { label: props.title }
]);

const tabs = [
  { key: "api", label: "模型 API 密钥" },
  { key: "application", label: "应用运行密钥" }
];

// 应用层分阶段开放期间，旧书签中的 ?tab=application 也统一回到模型 API 密钥。
watch(
  [activeTab, () => props.showApplicationKeys],
  ([tab, showApplicationKeys]) => {
    if (!showApplicationKeys && tab === "application") activeTab.value = "api";
  },
  { immediate: true }
);

function handleTabChange(key: string) {
  if (key === "application" && !props.showApplicationKeys) return;
  activeTab.value = key as PortalKeyManagementTab;
}

const createLabel = computed(() =>
  resolvedActiveTab.value === "api" ? "创建模型 API 密钥" : "新建应用运行密钥"
);
</script>

<template>
  <div class="key-management-page">
    <PortalPagePanel fill :icon="KeyRound" :breadcrumbs="breadcrumbs" :description="panelDescription">
      <template #actions>
        <slot name="header-actions" />
        <template v-if="tabActions">
          <el-button
            :icon="RefreshCw"
            :loading="tabActions[resolvedActiveTab].loading"
            @click="tabActions[resolvedActiveTab].refresh()"
          >
            刷新
          </el-button>
          <el-button
            type="primary"
            :icon="Plus"
            @click="tabActions[activeTab].create()"
          >
            {{ createLabel }}
          </el-button>
        </template>
      </template>

      <div class="key-management-page__body">
        <div v-if="showApplicationKeys" class="key-management-page__tabs">
          <DsTabs :tabs="tabs" :model-value="resolvedActiveTab" @update:model-value="handleTabChange" />
        </div>

        <section
          v-if="resolvedActiveTab === 'api'"
          id="key-panel-api"
          class="key-management-page__content"
          role="tabpanel"
          aria-label="模型 API 密钥"
        >
          <slot name="api" />
        </section>

        <section
          v-else
          id="key-panel-application"
          class="key-management-page__content"
          role="tabpanel"
          aria-label="应用运行密钥"
        >
          <slot name="application" />
        </section>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.key-management-page {
  display: flex;
  min-width: 0;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}

/* 面板 body 无内边距:Tab 条用 24px 容器,内容区通栏(嵌入的工作区自带与面板同构的
   操作区/分页脚内边距与分隔线,见 PortalApiKeyWorkspace/PortalAppKeyWorkspace 的嵌入框架) */
.key-management-page__body {
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}

/* 分隔线画在容器上才通栏(DsTabs 自身不带线) */
.key-management-page__tabs {
  padding: 16px 24px 14px;
  border-bottom: 1px solid var(--ds-line);
}

.key-management-page__content {
  display: flex;
  min-width: 0;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}
</style>
