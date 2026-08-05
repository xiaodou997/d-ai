<script setup lang="ts">
import { DsToaster } from "@/shared/ui";

withDefaults(
  defineProps<{
    locale?: unknown;
    showToaster?: boolean;
  }>(),
  {
    locale: undefined,
    showToaster: false
  }
);
</script>

<template>
  <ElConfigProvider :locale="locale">
    <RouterView />
    <DsToaster v-if="showToaster" position="top-right" />
  </ElConfigProvider>
</template>
