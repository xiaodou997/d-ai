<script setup lang="ts">
import { computed, defineAsyncComponent } from "vue";
import { useAuthStore } from "@/stores/auth";

const authStore = useAuthStore();
const adminProfile = defineAsyncComponent(() => import("./admin/ProfileView.vue"));
const tenantProfile = defineAsyncComponent(() => import("./tenant/ProfileView.vue"));
const customerProfile = defineAsyncComponent(() => import("./customer/ProfileView.vue"));

const profileView = computed(() => {
  if (authStore.userType <= 2) return adminProfile;
  if (authStore.userType === 3) return tenantProfile;
  return customerProfile;
});
</script>

<template>
  <component :is="profileView" />
</template>
