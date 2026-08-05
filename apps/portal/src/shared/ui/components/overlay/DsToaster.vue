<script setup lang="ts">
import { Toaster } from "vue-sonner";

withDefaults(
  defineProps<{
    position?: "top-left" | "top-right" | "bottom-left" | "bottom-right" | "top-center" | "bottom-center";
    richColors?: boolean;
  }>(),
  {
    position: "top-right",
    richColors: true
  }
);
</script>

<template>
  <Toaster :position="position" :rich-colors="richColors" :toast-options="{ duration: 3500 }" />
</template>
