<script setup lang="ts">
import { copyRecordText } from "../recordPresentation";
defineProps<{ title: string; note?: string; featured?: boolean; facts: Array<{ label: string; value: string; copy?: string }> }>();
</script>
<template>
  <section class="record-facts" :class="{ 'record-facts--featured': featured }">
    <header><span class="record-facts__marker" aria-hidden="true" /><h2>{{ title }}</h2></header><p v-if="note">{{ note }}</p>
    <dl><div v-for="(fact, index) in facts" :key="`${fact.label}:${index}`"><dt>{{ fact.label }}</dt><dd>{{ fact.value }}<button v-if="fact.copy" type="button" @click="copyRecordText(fact.copy)">复制</button></dd></div></dl>
    <slot />
  </section>
</template>
<style scoped>
.record-facts { position: relative; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-panel); background: var(--ds-panel); padding: 20px; min-width: 0; box-shadow: var(--ds-shadow-panel); overflow: hidden; }
.record-facts--featured { background: linear-gradient(135deg, color-mix(in srgb, var(--ds-accent-soft) 48%, var(--ds-panel)), var(--ds-panel) 55%); border-color: color-mix(in srgb, var(--ds-accent) 22%, var(--ds-line)); }
.record-facts header { display: flex; align-items: center; gap: 9px; margin-bottom: 16px; }
.record-facts__marker { width: 4px; height: 16px; flex: none; border-radius: var(--ds-radius-pill); background: var(--ds-accent); }
.record-facts h2 { font-size: 15px; margin: 0; color: var(--ds-ink); }
.record-facts p { font-size: 12px; line-height: 1.7; color: var(--ds-muted); margin: -6px 0 16px; }
.record-facts dl { display: grid; grid-template-columns: repeat(auto-fit, minmax(185px, 1fr)); gap: 10px; margin: 0; }
.record-facts dl > div { min-width: 0; padding: 11px 12px; border: 1px solid color-mix(in srgb, var(--ds-line) 72%, transparent); border-radius: var(--ds-radius-control); background: color-mix(in srgb, var(--ds-panel-muted) 72%, transparent); }
.record-facts dt { color: var(--ds-muted); font-size: 11px; margin-bottom: 5px; }
.record-facts dd { margin: 0; color: var(--ds-ink); font-size: 13px; font-weight: 520; overflow-wrap: anywhere; white-space: pre-wrap; }
.record-facts button { border: 0; background: none; color: var(--ds-accent); cursor: pointer; margin-left: 8px; font-size: 11px; }
</style>
