<script setup lang="ts">
import type { PortalMarkdownInlineNode } from "./portalMarkdown";

defineOptions({ name: "PortalMarkdownInlines" });

defineProps<{
  nodes: PortalMarkdownInlineNode[];
}>();
</script>

<template>
  <template v-for="(node, index) in nodes" :key="index">
    <span v-if="node.type === 'text'" class="markdown-text">{{ node.text }}</span>
    <code v-else-if="node.type === 'code'" class="markdown-inline-code">{{ node.text }}</code>
    <strong v-else-if="node.type === 'strong'" class="markdown-strong">
      <PortalMarkdownInlines :nodes="node.children" />
    </strong>
    <em v-else-if="node.type === 'em'" class="markdown-em">
      <PortalMarkdownInlines :nodes="node.children" />
    </em>
    <del v-else-if="node.type === 'del'" class="markdown-del">
      <PortalMarkdownInlines :nodes="node.children" />
    </del>
    <a
      v-else-if="node.type === 'link'"
      class="markdown-link"
      :href="node.href"
      target="_blank"
      rel="noopener noreferrer nofollow"
    >
      <PortalMarkdownInlines :nodes="node.children" />
    </a>
    <br v-else class="markdown-break" />
  </template>
</template>

<style scoped>
.markdown-text {
  white-space: pre-wrap;
}

.markdown-inline-code {
  padding: 0.1rem 0.35rem;
  color: var(--ds-ink);
  font-size: 0.92em;
  font-family: "SFMono-Regular", "SF Mono", Consolas, "Liberation Mono", monospace;
  background: var(--ds-line);
  border-radius: 0.35rem;
}

.markdown-strong {
  font-weight: 800;
}

.markdown-em {
  font-style: italic;
}

.markdown-del {
  text-decoration: line-through;
}

.markdown-link {
  color: var(--ds-positive);
  text-decoration: underline;
  text-decoration-thickness: 0.08em;
  text-underline-offset: 0.18em;
  word-break: break-word;
}
</style>
