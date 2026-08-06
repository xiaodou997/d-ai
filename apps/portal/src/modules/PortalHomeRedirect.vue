<script setup lang="ts">
import { onMounted } from "vue";
import { useRouter } from "vue-router";

import { useAuthStore } from "@/stores/auth";

import { defaultPortalPathForUserType } from "./portalModules";

const router = useRouter();
const authStore = useAuthStore();

onMounted(() => {
  void router.replace(defaultPortalPathForUserType(authStore.userType));
});
</script>

<template>
  <div class="portal-home-redirect" aria-live="polite">正在进入工作台...</div>
</template>

<style scoped>
.portal-home-redirect {
  display: grid;
  flex: 1;
  min-height: 240px;
  place-items: center;
  color: var(--ds-muted);
  font-size: 13px;
}
</style>
