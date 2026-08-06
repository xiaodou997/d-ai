<script setup lang="ts">
import { onBeforeMount } from "vue";
import { useRoute, useRouter } from "vue-router";

const route = useRoute();
const router = useRouter();

onBeforeMount(() => {
  const userId = typeof route.params.userId === "string" ? route.params.userId : "";
  if (userId) {
    void router.replace({
      name: "tenant-user-detail",
      params: { userId },
      query: { policy: "1" }
    });
    return;
  }
  void router.replace("/tenant/users/directory");
});
</script>

<template>
  <div />
</template>
