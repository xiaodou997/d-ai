<script setup lang="ts">
import { onBeforeUnmount, shallowRef } from "vue";

const props = defineProps<{
  title: string;
  code: string;
  caption?: string;
  copyText?: string;
}>();

const copied = shallowRef(false);
let clearTimer: number | undefined;

async function handleCopy() {
  await navigator.clipboard.writeText(props.copyText || props.code);
  copied.value = true;
  if (clearTimer) {
    window.clearTimeout(clearTimer);
  }
  clearTimer = window.setTimeout(() => {
    copied.value = false;
  }, 1600);
}

onBeforeUnmount(() => {
  if (clearTimer) {
    window.clearTimeout(clearTimer);
  }
});
</script>

<template>
  <div class="ai-docs-code-block">
    <div class="ai-docs-code-block__bar">
      <div class="ai-docs-code-block__copy">
        <span class="ai-docs-code-block__title">{{ title }}</span>
        <p v-if="caption" class="ai-docs-code-block__caption">{{ caption }}</p>
      </div>
      <button class="ai-docs-code-block__action" type="button" @click="handleCopy">
        {{ copied ? "已复制" : "复制" }}
      </button>
    </div>
    <pre class="ai-docs-code-block__pre">{{ code }}</pre>
  </div>
</template>

<style scoped>
.ai-docs-code-block {
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-top: 2px solid var(--ds-accent);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-code-bg);
}

.ai-docs-code-block__bar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border-bottom: 1px solid color-mix(in srgb, var(--ds-accent) 16%, var(--ds-line-soft));
  background: var(--ds-code-header);
}

.ai-docs-code-block__copy {
  min-width: 0;
}

.ai-docs-code-block__title {
  display: block;
  color: var(--ds-code-fg);
  font-family: var(--ds-font-mono);
  font-size: 12px;
  font-weight: 700;
}

.ai-docs-code-block__caption {
  margin: 4px 0 0;
  color: var(--ds-faint);
  font-size: 12px;
  line-height: 1.5;
}

.ai-docs-code-block__action {
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

.ai-docs-code-block__action:hover {
  background: color-mix(in srgb, var(--ds-accent) 32%, transparent);
}

.ai-docs-code-block__pre {
  margin: 0;
  overflow-x: auto;
  padding: 16px 18px;
  color: var(--ds-code-accent);
  font-family: var(--ds-font-mono);
  font-size: 13px;
  line-height: 1.75;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
