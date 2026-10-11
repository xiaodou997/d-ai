<script setup lang="ts">
import { ElConfigProvider, type ConfigProviderProps } from "element-plus";
import DsToaster from "@/shared/ui/components/overlay/DsToaster.vue";

withDefaults(
  defineProps<{
    locale?: ConfigProviderProps["locale"];
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
