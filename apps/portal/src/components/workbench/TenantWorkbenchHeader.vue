<script setup lang="ts">
import { PortalPageHeader } from "@/platform";

withDefaults(
  defineProps<{
    eyebrow: string;
    title: string;
    description: string;
    tone?: "primary" | "teal" | "blue";
  }>(),
  {
    tone: "primary"
  }
);
</script>

<template>
  <PortalPageHeader :eyebrow="eyebrow" :title="title" :description="description">
    <template #actions>
      <slot name="actions" />
    </template>
  </PortalPageHeader>
</template>
