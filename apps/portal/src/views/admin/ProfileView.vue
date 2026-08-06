<script setup lang="ts">
import { computed } from "vue";
import { PortalProfileWorkspace } from "@/platform";

import { useAuthStore } from "@/stores/auth";
import { platformAdminApi } from "@/api/platformAdmin";

const authStore = useAuthStore();

const profileFields = computed(() => [
  { label: "用户名", value: authStore.username || "", tone: "strong" as const },
  { label: "用户 ID", value: authStore.userInfo?.sub || "", tone: "mono" as const }
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
    :change-password="platformAdminApi.changePassword"
    :after-password-changed="handlePasswordChanged"
  />
</template>
