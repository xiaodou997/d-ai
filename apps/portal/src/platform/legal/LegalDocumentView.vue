<script setup lang="ts">
import { computed } from "vue";
import { RouterLink, useRoute } from "vue-router";

import { findLegalDocument } from "./catalog";

const route = useRoute();
const document = computed(() =>
  findLegalDocument(String(route.params.document || "")),
);
</script>

<template>
  <article v-if="document" class="legal-document">
    <header class="legal-document__header">
      <p class="legal-document__eyebrow">法律文件</p>
      <h1 class="legal-document__title">{{ document.title }}</h1>
      <p class="legal-document__summary">{{ document.summary }}</p>
      <dl class="legal-document__meta">
        <div>
          <dt>版本</dt>
          <dd>{{ document.version }}</dd>
        </div>
        <div>
          <dt>生效日期</dt>
          <dd>{{ document.effectiveDate }}</dd>
        </div>
      </dl>
    </header>

    <section
      v-for="section in document.sections"
      :key="section.heading"
      class="legal-document__section"
    >
      <h2 class="legal-document__section-title">{{ section.heading }}</h2>
      <p
        v-for="paragraph in section.paragraphs"
        :key="paragraph"
        class="legal-document__paragraph"
      >
        {{ paragraph }}
      </p>
      <ul v-if="section.items" class="legal-document__list">
        <li v-for="item in section.items" :key="item">{{ item }}</li>
      </ul>
    </section>
  </article>

  <section v-else class="legal-document__missing">
    <h1 class="legal-document__title">未找到该法律文件</h1>
    <RouterLink to="/legal/privacy">查看隐私政策</RouterLink>
  </section>
</template>

<style scoped>
.legal-document,
.legal-document__missing {
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-white);
  box-shadow: var(--ds-shadow-panel);
}

.legal-document__header {
  padding: 40px;
  border-bottom: 1px solid var(--ds-line);
}

.legal-document__eyebrow {
  margin: 0;
  color: var(--ds-accent);
  font-size: 12px;
  font-weight: 800;
}

.legal-document__title {
  margin: 10px 0 0;
  color: var(--ds-ink);
  font-size: 30px;
  line-height: 1.2;
}

.legal-document__summary {
  max-width: 640px;
  margin: 14px 0 0;
  color: var(--ds-muted);
  font-size: 15px;
  line-height: 1.7;
}

.legal-document__meta {
  display: flex;
  gap: 32px;
  margin: 28px 0 0;
}

.legal-document__meta div {
  display: grid;
  gap: 4px;
}

.legal-document__meta dt {
  color: var(--ds-faint);
  font-size: 12px;
}

.legal-document__meta dd {
  margin: 0;
  color: var(--ds-ink-soft);
  font-size: 13px;
  font-weight: 700;
}

.legal-document__section {
  padding: 30px 40px 0;
}

.legal-document__section:last-child {
  padding-bottom: 40px;
}

.legal-document__section-title {
  margin: 0 0 12px;
  color: var(--ds-ink);
  font-size: 18px;
}

.legal-document__paragraph,
.legal-document__list {
  margin: 0 0 14px;
  color: var(--ds-ink-soft);
  font-size: 14px;
  line-height: 1.85;
}

.legal-document__list {
  padding-left: 20px;
}

.legal-document__missing {
  padding: 40px;
}

.legal-document__missing a {
  display: inline-block;
  margin-top: 16px;
  color: var(--ds-accent);
}

@media (max-width: 720px) {
  .legal-document__header,
  .legal-document__section,
  .legal-document__section:last-child,
  .legal-document__missing {
    padding-right: 24px;
    padding-left: 24px;
  }

  .legal-document__title {
    font-size: 26px;
  }
}
</style>
