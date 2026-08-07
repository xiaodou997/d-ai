<script setup lang="ts">
import { computed, inject } from "vue";
import { KeyRound, Plus, RefreshCw } from "lucide-vue-next";

import PortalPagePanel from "../../page/PortalPagePanel.vue";

export type PortalKeyManagementTab = "api";

interface TabActionState {
  loading: boolean;
  refresh: () => void;
  create: () => void;
}

const tabActions = inject<Record<PortalKeyManagementTab, TabActionState> | null>(
  "keyManagementTabActions",
  null
);

const props = withDefaults(
  defineProps<{
    title?: string;
    description?: string;
    eyebrow?: string;
  }>(),
  {
    title: "API 密钥",
    description: "创建并管理用于直接调用模型的 API 密钥。",
    eyebrow: "智能服务 / 开发接入"
  }
);

const breadcrumbs = computed(() => [
  ...props.eyebrow
    .split("/")
    .map((segment) => segment.trim())
    .filter(Boolean)
    .map((label) => ({ label })),
  { label: props.title }
]);
</script>

<template>
  <div class="key-management-page">
    <PortalPagePanel fill :icon="KeyRound" :breadcrumbs="breadcrumbs" :description="description">
      <template #actions>
        <slot name="header-actions" />
        <template v-if="tabActions">
          <el-button :icon="RefreshCw" :loading="tabActions.api.loading" @click="tabActions.api.refresh()">
            刷新
          </el-button>
          <el-button type="primary" :icon="Plus" @click="tabActions.api.create()">
            创建 API 密钥
          </el-button>
        </template>
      </template>

      <section class="key-management-page__content" aria-label="API 密钥">
        <slot name="api" />
      </section>
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

.key-management-page__content {
  display: flex;
  min-width: 0;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}
</style>
