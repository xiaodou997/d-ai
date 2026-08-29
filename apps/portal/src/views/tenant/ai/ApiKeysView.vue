<script setup lang="ts">
import { TenantApiKeysWorkspace } from "@/features/ai/api-keys";

defineProps<{
  embedded?: boolean;
}>();
</script>

<template>
  <TenantApiKeysWorkspace :embedded="embedded" />
</template>
