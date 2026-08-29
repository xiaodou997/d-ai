<!-- AI 接入文档工作区适配层：根据统一 auth store 注入租户/客户 scope。 -->
<script setup lang="ts">
import { computed } from "vue";

import { portalEnv } from "@/env";
import { PortalAiDocsPage, type PortalAiDocsSectionKey } from "@/platform/ai";
import { useAuthStore } from "@/stores/auth";

defineProps<{ section: PortalAiDocsSectionKey }>();

const authStore = useAuthStore();
const scope = computed(() => (authStore.userType === 4 ? "user" : "tenant"));
</script>

<template>
  <PortalAiDocsPage :base-url="portalEnv.apiBaseUrl" :scope="scope" :section="section" />
</template>
