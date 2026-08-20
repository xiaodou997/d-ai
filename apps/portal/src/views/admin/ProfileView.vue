<script setup lang="ts">
import { computed } from "vue";
import { createFetchAdapter, PortalProfileWorkspace } from "@/platform";
import { portalEnv } from "@/env";

import { useAuthStore } from "@/stores/auth";
import { platformAdminApi } from "@/api/platformAdmin";

const authStore = useAuthStore();
const request = createFetchAdapter({ getAccessToken: () => authStore.accessToken });
const mfaApi = {
  get enabled() {
    return Boolean(authStore.userInfo?.mfaEnabled);
  },
  enroll: () => request<{ secret: string; otpauthUrl: string }>({ method: "POST", path: "/api/auth/mfa/enroll", baseUrl: portalEnv.apiBaseUrl }),
  confirm: (code: string) => request<{ message: string }>({ method: "POST", path: "/api/auth/mfa/confirm", body: { code }, baseUrl: portalEnv.apiBaseUrl })
};

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
    :mfa="mfaApi"
  />
</template>
