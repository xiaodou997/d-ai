<script setup lang="ts">
import { computed } from "vue";

import {
  legalDocumentURL,
  legalFooterDocumentIDs,
  legalDocuments,
} from "./catalog";

const links = computed(() =>
  legalFooterDocumentIDs.map((id) => {
    const document = legalDocuments.find((item) => item.id === id)!;
    return {
      id,
      title: document.title,
      href: legalDocumentURL(id),
    };
  }),
);
</script>

<template>
  <footer class="portal-legal-footer" aria-label="法律与服务说明">
    <span class="portal-legal-footer__notice"
      >使用本服务即受适用法律文本约束</span
    >
    <nav class="portal-legal-footer__links" aria-label="法律文件">
      <a
        v-for="link in links"
        :key="link.id"
        :href="link.href"
        target="_blank"
        rel="noreferrer"
      >
        {{ link.title }}
      </a>
    </nav>
  </footer>
</template>

<style scoped>
.portal-legal-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-height: 48px;
  padding: 12px 28px;
  border-top: 1px solid var(--ds-line);
  background: var(--ds-paper);
  color: var(--ds-muted);
  font-size: 12px;
}

.portal-legal-footer__notice {
  white-space: nowrap;
}

.portal-legal-footer__links {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 16px;
}

.portal-legal-footer__links a {
  color: inherit;
  text-decoration: none;
}

.portal-legal-footer__links a:hover {
  color: var(--ds-accent);
  text-decoration: underline;
}

@media (max-width: 720px) {
  .portal-legal-footer {
    align-items: flex-start;
    flex-direction: column;
    padding: 16px;
  }

  .portal-legal-footer__notice {
    white-space: normal;
  }

  .portal-legal-footer__links {
    justify-content: flex-start;
    gap: 12px;
  }
}
</style>
