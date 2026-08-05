<script setup lang="ts">
import { computed } from "vue";
import { DsTag } from "@dai/ui";

import { usageTagMeta } from "./meta";
import type { UsageTagKind } from "./types";

const props = defineProps<{
  kind: UsageTagKind;
  value: string | boolean | null | undefined;
}>();

const meta = computed(() => usageTagMeta(props.kind, props.value));
</script>

<template>
  <DsTag v-if="meta" :tone="meta.tone" class="usage-tag">{{ meta.label }}</DsTag>
  <span v-else class="usage-tag-empty">—</span>
</template>

<style scoped>
.usage-tag {
  padding: 1px 8px;
  font-size: 11px;
}

.usage-tag-empty {
  color: var(--ds-faint);
  font-size: 12px;
}
</style>
