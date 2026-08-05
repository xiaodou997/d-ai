<script setup lang="ts">
import { onBeforeUnmount, shallowRef } from "vue";

const props = defineProps<{
  title?: string;
  code: string;
  caption?: string;
}>();

const copied = shallowRef(false);
let clearTimer: number | undefined;

async function handleCopy() {
  await navigator.clipboard.writeText(props.code);
  copied.value = true;
  if (clearTimer) window.clearTimeout(clearTimer);
  clearTimer = window.setTimeout(() => { copied.value = false; }, 1600);
}

onBeforeUnmount(() => {
  if (clearTimer) window.clearTimeout(clearTimer);
});
</script>

<template>
  <div class="guide-code-block">
    <div class="guide-code-block__bar">
      <div class="guide-code-block__meta">
        <span v-if="title" class="guide-code-block__title">{{ title }}</span>
        <p v-if="caption" class="guide-code-block__caption">{{ caption }}</p>
      </div>
      <button class="guide-code-block__action" type="button" @click="handleCopy">
        {{ copied ? "已复制" : "复制" }}
      </button>
    </div>
    <pre class="guide-code-block__pre">{{ code }}</pre>
  </div>
</template>

<style scoped>
.guide-code-block {
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-top: 2px solid var(--ds-accent);
  border-radius: var(--ds-radius-panel);
  background: #0f172a;
}

.guide-code-block__bar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border-bottom: 1px solid color-mix(in srgb, var(--ds-accent) 16%, rgba(148, 163, 184, 0.18));
  background: rgba(15, 23, 42, 0.72);
}

.guide-code-block__meta {
  min-width: 0;
}

.guide-code-block__title {
  display: block;
  color: #e2e8f0;
  font-family: var(--ds-font-mono);
  font-size: 12px;
  font-weight: 700;
}

.guide-code-block__caption {
  margin: 4px 0 0;
  color: #94a3b8;
  font-size: 12px;
  line-height: 1.5;
}

.guide-code-block__action {
  flex-shrink: 0;
  border: 0;
  border-radius: var(--ds-radius-control);
  background: color-mix(in srgb, var(--ds-accent) 20%, transparent);
  color: var(--ds-accent-soft);
  padding: 7px 12px;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
  transition: background 0.16s ease;
}

.guide-code-block__action:hover {
  background: color-mix(in srgb, var(--ds-accent) 32%, transparent);
}

.guide-code-block__pre {
  margin: 0;
  overflow-x: auto;
  padding: 16px 18px;
  color: #dbeafe;
  font-family: var(--ds-font-mono);
  font-size: 13px;
  line-height: 1.75;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
