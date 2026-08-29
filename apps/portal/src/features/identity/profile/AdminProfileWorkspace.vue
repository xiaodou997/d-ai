<!-- 管理员个人中心适配层；资料/密码/MFA 表单由共享 PortalProfileWorkspace 负责。 -->
<script setup lang="ts">
import { computed } from "vue";

import { createTypedOperationRequest } from "@/api";
import { platformAdminApi } from "@/api/platformAdmin";
import { portalEnv } from "@/env";
import { createFetchAdapter, PortalProfileWorkspace } from "@/platform";
import { useAuthStore } from "@/stores/auth";

const authStore = useAuthStore();
const request = createTypedOperationRequest(createFetchAdapter({ getAccessToken: () => authStore.accessToken }));
const mfaApi = {
  get enabled() {
    return Boolean(authStore.userInfo?.mfaEnabled);
  },
  enroll: () => request<"auth-mfa-enroll">({ method: "POST", path: "/api/auth/mfa/enroll", baseUrl: portalEnv.apiBaseUrl }).then(({ secret, otpauthUrl }) => ({ secret, otpauthUrl })),
  confirm: (code: string) => request<"auth-mfa-confirm">({ method: "POST", path: "/api/auth/mfa/confirm", body: { code }, baseUrl: portalEnv.apiBaseUrl }).then(({ message }) => ({ message }))
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
