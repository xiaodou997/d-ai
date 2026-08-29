<script setup lang="ts">
import { computed, onBeforeUnmount, shallowRef, watch } from "vue";

interface CodeTab {
  key: string;
  label: string;
  code: string;
}

const props = defineProps<{
  title: string;
  tabs: CodeTab[];
  caption?: string;
}>();

const activeKey = shallowRef(props.tabs[0]?.key ?? "");
const copied = shallowRef(false);
let clearTimer: number | undefined;

const activeTab = computed(() => props.tabs.find((item) => item.key === activeKey.value) ?? props.tabs[0]);

watch(
  () => props.tabs,
  (tabs) => {
    if (!tabs.length) {
      activeKey.value = "";
      return;
    }
    if (!tabs.some((item) => item.key === activeKey.value)) {
      activeKey.value = tabs[0].key;
    }
  },
  { immediate: true }
);

async function handleCopy() {
  if (!activeTab.value) return;
  await navigator.clipboard.writeText(activeTab.value.code);
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
  <div class="ai-docs-tabbed-code">
    <div class="ai-docs-tabbed-code__bar">
      <div class="ai-docs-tabbed-code__copy">
        <span class="ai-docs-tabbed-code__title">{{ title }}</span>
        <p v-if="caption" class="ai-docs-tabbed-code__caption">{{ caption }}</p>
      </div>
      <div class="ai-docs-tabbed-code__actions">
        <div class="ai-docs-tabbed-code__tabs">
          <button
            v-for="tab in tabs"
            :key="tab.key"
            class="ai-docs-tabbed-code__tab"
            :class="{ 'is-active': activeKey === tab.key }"
            type="button"
            @click="activeKey = tab.key"
          >
            {{ tab.label }}
          </button>
        </div>
        <button class="ai-docs-tabbed-code__copy-btn" type="button" @click="handleCopy">
          {{ copied ? "已复制" : "复制" }}
        </button>
      </div>
    </div>
    <pre class="ai-docs-tabbed-code__pre">{{ activeTab?.code }}</pre>
  </div>
</template>

<style scoped>
.ai-docs-tabbed-code {
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-top: 2px solid var(--ds-accent);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-code-bg);
}

.ai-docs-tabbed-code__bar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
  padding: 12px 14px;
  border-bottom: 1px solid color-mix(in srgb, var(--ds-accent) 16%, var(--ds-line-soft));
  background: var(--ds-code-header);
}

.ai-docs-tabbed-code__copy {
  min-width: 0;
}

.ai-docs-tabbed-code__title {
  display: block;
  color: var(--ds-code-fg);
  font-family: var(--ds-font-mono);
  font-size: 12px;
  font-weight: 700;
}

.ai-docs-tabbed-code__caption {
  margin: 4px 0 0;
  color: var(--ds-faint);
  font-size: 12px;
  line-height: 1.5;
}

.ai-docs-tabbed-code__actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.ai-docs-tabbed-code__tabs {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.ai-docs-tabbed-code__tab {
  border: 0;
  border-radius: var(--ds-radius-control);
  background: transparent;
  color: var(--ds-faint);
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
  transition: background 0.16s ease, color 0.16s ease;
}

.ai-docs-tabbed-code__tab:hover {
  background: var(--ds-line-soft);
  color: var(--ds-code-fg);
}

.ai-docs-tabbed-code__tab.is-active {
  background: var(--ds-accent);
  color: var(--ds-accent-contrast);
}

.ai-docs-tabbed-code__copy-btn {
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

.ai-docs-tabbed-code__copy-btn:hover {
  background: color-mix(in srgb, var(--ds-accent) 32%, transparent);
}

.ai-docs-tabbed-code__pre {
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
