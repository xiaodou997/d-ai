<script setup lang="ts">
import { PortalContentCard } from "@/platform";

withDefaults(
  defineProps<{
    eyebrow?: string;
    title: string;
    description?: string;
    tone?: "neutral" | "primary" | "teal" | "blue" | "amber";
  }>(),
  {
    eyebrow: "",
    description: "",
    tone: "neutral"
  }
);
</script>

<template>
  <PortalContentCard class="tenant-workbench-section">
    <template #header>
      <div class="section-copy">
        <p v-if="eyebrow" class="section-eyebrow">{{ eyebrow }}</p>
        <h2 class="section-title">{{ title }}</h2>
        <p v-if="description" class="section-desc">{{ description }}</p>
      </div>
    </template>
    <template #actions>
      <slot name="actions" />
    </template>
    <slot />
  </PortalContentCard>
</template>

<style scoped>
.section-copy {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-width: 640px;
}

.section-eyebrow {
  margin: 0;
  color: var(--ds-accent-hover);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.section-title {
  margin: 0;
  color: var(--ds-ink);
  font-size: 16px;
  font-weight: 650;
}

.section-desc {
  margin: 0;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.6;
}
</style>
