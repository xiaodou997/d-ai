<script setup lang="ts">
import { Github } from "lucide-vue-next";

const repositoryUrl = "https://github.com/xiaodou997/d-ai";
</script>

<template>
  <a
    class="portal-github-link"
    :href="repositoryUrl"
    target="_blank"
    rel="noopener noreferrer"
    aria-label="访问 D-AI GitHub 仓库"
    title="GitHub 仓库"
  >
    <Github :size="18" :stroke-width="2" aria-hidden="true" />
  </a>
</template>

<style scoped>
.portal-github-link {
  display: grid;
  width: 32px;
  height: 32px;
  place-items: center;
  border-radius: var(--ds-radius-control);
  color: var(--ds-ink-soft);
  text-decoration: none;
  transition:
    background-color 140ms ease,
    color 140ms ease;
}

.portal-github-link:hover {
  background: var(--ds-panel-muted);
  color: var(--ds-ink);
}

.portal-github-link:focus-visible {
  outline: none;
  box-shadow: var(--ds-shadow-focus);
}
</style>
