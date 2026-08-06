<script setup lang="ts">
import { computed } from "vue";
import { PortalProfileWorkspace } from "@/platform";

import { useAuthStore } from "@/stores/auth";
import { platformTenantApi } from "@/api/platformTenant";

const authStore = useAuthStore();

const profileFields = computed(() => [
  { label: "用户名", value: authStore.username || "", tone: "strong" as const },
  { label: "用户 ID", value: authStore.userInfo?.sub || "", tone: "mono" as const },
  { label: "所属租户", value: authStore.tenantName || "" }
]);

const handlePasswordChanged = async () => {
  const redirected = await authStore.logout();
  if (!redirected) {
    window.location.reload();
  }
};
</script>

<template>
  <PortalProfileWorkspace
    :fields="profileFields"
    :change-password="platformTenantApi.changePassword"
    :after-password-changed="handlePasswordChanged"
  />
</template>
