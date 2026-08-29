<script setup lang="ts">
import { computed } from "vue";

import PortalMarkdownInlines from "./PortalMarkdownInlines.vue";
import { parsePortalMarkdown } from "./portalMarkdown";
import type { PortalMarkdownBlock } from "./portalMarkdown";

defineOptions({ name: "PortalMarkdownRenderer" });

const props = withDefaults(
  defineProps<{
    source?: string;
    blocks?: PortalMarkdownBlock[];
  }>(),
  {
    source: ""
  }
);

const renderedBlocks = computed(() => props.blocks ?? parsePortalMarkdown(props.source));

function headingTag(depth: number) {
  return `h${Math.min(Math.max(depth, 1), 6)}`;
}

function headingClass(depth: number) {
  return `markdown-heading markdown-heading-${Math.min(Math.max(depth, 1), 6)}`;
}
</script>

<template>
  <div class="markdown-root">
    <template v-for="(block, index) in renderedBlocks" :key="index">
      <component
        :is="headingTag(block.depth)"
        v-if="block.type === 'heading'"
        :class="headingClass(block.depth)"
      >
        <PortalMarkdownInlines :nodes="block.inlines" />
      </component>

      <p v-else-if="block.type === 'paragraph'" class="markdown-paragraph">
        <PortalMarkdownInlines :nodes="block.inlines" />
      </p>

      <div v-else-if="block.type === 'code'" class="markdown-code-surface">
        <span v-if="block.lang" class="markdown-code-lang">{{ block.lang }}</span>
        <pre class="markdown-code-block"><code class="markdown-code-text">{{ block.text }}</code></pre>
      </div>

      <blockquote v-else-if="block.type === 'blockquote'" class="markdown-blockquote">
        <PortalMarkdownRenderer :blocks="block.blocks" />
      </blockquote>

      <ol v-else-if="block.type === 'list' && block.ordered" class="markdown-list markdown-list-ordered" :start="block.start">
        <li v-for="(item, itemIndex) in block.items" :key="itemIndex" class="markdown-list-item">
          <div v-if="item.checked !== null" class="markdown-task-item">
            <input class="markdown-task-checkbox" type="checkbox" :checked="item.checked" disabled />
            <div class="markdown-task-content">
              <PortalMarkdownRenderer :blocks="item.blocks" />
            </div>
          </div>
          <PortalMarkdownRenderer v-else :blocks="item.blocks" />
        </li>
      </ol>

      <ul v-else-if="block.type === 'list'" class="markdown-list markdown-list-unordered">
        <li v-for="(item, itemIndex) in block.items" :key="itemIndex" class="markdown-list-item">
          <div v-if="item.checked !== null" class="markdown-task-item">
            <input class="markdown-task-checkbox" type="checkbox" :checked="item.checked" disabled />
            <div class="markdown-task-content">
              <PortalMarkdownRenderer :blocks="item.blocks" />
            </div>
          </div>
          <PortalMarkdownRenderer v-else :blocks="item.blocks" />
        </li>
      </ul>

      <div v-else class="markdown-rule" aria-hidden="true" role="separator"></div>
    </template>
  </div>
</template>

<style scoped>
.markdown-root {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 0.8rem;
}

.markdown-heading,
.markdown-paragraph,
.markdown-code-block,
.markdown-blockquote,
.markdown-list {
  margin: 0;
}

.markdown-heading {
  color: var(--ds-ink);
  line-height: 1.35;
}

.markdown-heading-1 {
  font-size: 1.25rem;
  font-weight: 900;
}

.markdown-heading-2 {
  font-size: 1.15rem;
  font-weight: 850;
}

.markdown-heading-3 {
  font-size: 1.05rem;
  font-weight: 800;
}

.markdown-heading-4,
.markdown-heading-5,
.markdown-heading-6 {
  font-size: 0.98rem;
  font-weight: 800;
}

.markdown-paragraph {
  word-break: break-word;
  line-height: 1.7;
}

.markdown-code-surface {
  min-width: 0;
  padding: 0.55rem;
  background: var(--ds-code-bg);
  border-radius: var(--ds-radius-panel);
}

.markdown-code-lang {
  display: inline-flex;
  margin-bottom: 0.45rem;
  padding: 0.15rem 0.45rem;
  color: var(--ds-line-strong);
  font-size: 0.72rem;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  background: var(--ds-line-soft);
  border-radius: var(--ds-radius-pill);
}

.markdown-code-block {
  overflow-x: auto;
}

.markdown-code-text {
  display: block;
  color: var(--ds-code-fg);
  font-size: 0.87rem;
  line-height: 1.6;
  font-family: "SFMono-Regular", "SF Mono", Consolas, "Liberation Mono", monospace;
  white-space: pre;
}

.markdown-blockquote {
  padding-left: 0.95rem;
  color: var(--ds-ink-soft);
  border-left: 0.24rem solid var(--ds-info);
}

.markdown-list {
  min-width: 0;
  padding-left: 1.25rem;
}

.markdown-list-item {
  min-width: 0;
}

.markdown-list-item + .markdown-list-item {
  margin-top: 0.55rem;
}

.markdown-task-item {
  min-width: 0;
  display: flex;
  align-items: flex-start;
  gap: 0.6rem;
}

.markdown-task-checkbox {
  margin-top: 0.28rem;
}

.markdown-task-content {
  min-width: 0;
  flex: 1;
}

.markdown-rule {
  height: 1px;
  background: var(--ds-line-strong);
}
</style>
