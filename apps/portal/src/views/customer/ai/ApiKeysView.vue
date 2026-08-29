<script setup lang="ts">
import { CustomerApiKeysWorkspace } from "@/features/ai/api-keys";

defineProps<{
  embedded?: boolean;
}>();
</script>

<template>
  <CustomerApiKeysWorkspace :embedded="embedded" />
</template>
