<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";

import { resolvePortalPublicBaseUrl } from "../../portal-router";
import type { PortalAiDocsScope, PortalAiDocsSectionKey } from "../docs";
import { portalAiDocsSectionByKey, PORTAL_AI_DOC_SECTIONS } from "../docs";
import PortalAiDocsLayout from "./PortalAiDocsLayout.vue";
import PortalAiDocsToolingSection from "./sections/PortalAiDocsToolingSection.vue";

const props = withDefaults(
  defineProps<{
    baseUrl: string;
    scope?: PortalAiDocsScope;
    section?: PortalAiDocsSectionKey;
  }>(),
  {
    scope: "tenant",
    section: "tooling"
  }
);

const route = useRoute();

const normalizedBaseUrl = computed(() => resolvePortalPublicBaseUrl(props.baseUrl));
const currentSection = computed(() => portalAiDocsSectionByKey(props.section));
const basePath = computed(() => {
  const suffix = `/${currentSection.value.slug}`;
  return route.path.endsWith(suffix) ? route.path.slice(0, -suffix.length) : route.path.replace(/\/$/, "");
});
</script>

<template>
  <PortalAiDocsLayout :scope="scope" :sections="PORTAL_AI_DOC_SECTIONS" :current-section="currentSection" :base-path="basePath">
    <PortalAiDocsToolingSection :base-url="normalizedBaseUrl" />
  </PortalAiDocsLayout>
</template>
