<script setup lang="ts">
import { AiDocsWorkspace } from "@/features/ai/docs";
import type { PortalAiDocsSectionKey } from "@/platform/ai";

defineProps<{ section: PortalAiDocsSectionKey }>();
</script>

<template>
  <AiDocsWorkspace :section="section" />
</template>
