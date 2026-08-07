<script setup lang="ts">
import { computed } from "vue";
import { RouterLink } from "vue-router";

import type { PortalAiDocsScope, PortalAiDocsSection } from "../docs";
import { portalAiDocsScopeLabel } from "../docs";

const props = defineProps<{
  scope: PortalAiDocsScope;
  sections: ReadonlyArray<PortalAiDocsSection>;
  currentSection: PortalAiDocsSection;
  basePath: string;
}>();

const scopeLabel = computed(() => portalAiDocsScopeLabel(props.scope));
const sectionLinks = computed(() =>
  props.sections.map((section) => ({
    ...section,
    to: `${props.basePath}/${section.slug}`
  }))
);

</script>

<template>
  <div class="page-container ai-docs-page">
    <section class="ai-docs-header">
      <div class="ai-docs-header__top">
        <div class="ai-docs-header__copy">
          <p class="ai-docs-header__eyebrow">开发者</p>
          <h1 class="ai-docs-header__title">{{ currentSection.title }}</h1>
          <p class="ai-docs-header__desc">{{ currentSection.description }}</p>
        </div>
        <div class="ai-docs-header__badges">
          <span class="ai-docs-header__badge ai-docs-header__badge--scope">{{ scopeLabel }}</span>
          <span class="ai-docs-header__badge ai-docs-header__badge--key">sk_</span>
        </div>
      </div>
      <nav v-if="sectionLinks.length > 1" class="ai-docs-tabs">
        <RouterLink
          v-for="section in sectionLinks"
          :key="section.key"
          :to="section.to"
          class="ai-docs-tabs__item"
          :class="{ 'is-active': section.key === currentSection.key }"
        >
          {{ section.navLabel }}
        </RouterLink>
      </nav>
    </section>

    <div class="ai-docs-content">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.ai-docs-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* ===== Header 卡 ===== */
.ai-docs-header {
  overflow: hidden;
  border: 1px solid color-mix(in srgb, var(--ds-accent) 14%, var(--ds-line));
  border-radius: var(--ds-radius-panel);
  background: linear-gradient(
    180deg,
    color-mix(in srgb, var(--ds-accent) 5%, var(--ds-panel)) 0%,
    var(--ds-panel) 100%
  );
  box-shadow: var(--ds-shadow-sm);
}

.ai-docs-header__top {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: start;
  gap: 16px;
  padding: 20px 24px 18px;
}

.ai-docs-header__copy {
  min-width: 0;
}

.ai-docs-header__eyebrow {
  margin: 0 0 6px;
  color: var(--ds-accent-hover);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.04em;
}

.ai-docs-header__title {
  margin: 0;
  color: var(--ds-ink);
  font-size: 22px;
  font-weight: 700;
  line-height: 1.2;
}

.ai-docs-header__desc {
  margin: 8px 0 0;
  max-width: 680px;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.6;
}

.ai-docs-header__badges {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.ai-docs-header__badge {
  padding: 6px 10px;
  border-radius: var(--ds-radius-control);
  font-size: 11px;
  font-weight: 700;
  line-height: 1;
}

.ai-docs-header__badge--scope {
  background: var(--ds-panel-muted);
  color: var(--ds-muted);
}

.ai-docs-header__badge--key {
  background: color-mix(in srgb, var(--ds-accent) 10%, var(--ds-panel));
  color: var(--ds-accent-hover);
}

/* ===== Tab 条 ===== */
.ai-docs-tabs {
  display: flex;
  gap: 2px;
  padding: 0 20px;
  border-top: 1px solid var(--ds-line);
}

.ai-docs-tabs__item {
  padding: 12px 16px;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--ds-muted);
  font-size: 13px;
  font-weight: 700;
  line-height: 1.3;
  text-decoration: none;
  transition: background 0.16s ease, color 0.16s ease, border-color 0.16s ease;
}

.ai-docs-tabs__item:hover {
  background: var(--ds-panel-muted);
  color: var(--ds-ink-soft);
}

.ai-docs-tabs__item.is-active {
  color: var(--ds-accent-hover);
  border-bottom-color: var(--ds-accent);
}

/* ===== 内容区 ===== */
.ai-docs-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* ===== PortalContentCard body 内连续子元素间距 ===== */
:deep(.ai-docs-content .portal-content-card__body > * + *) {
  margin-top: 14px;
}

/* ===== 公共 :deep 样式（供 section 组件使用）===== */
:deep(.ai-docs-stack) {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

:deep(.ai-docs-grid) {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

:deep(.ai-docs-grid--three) {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

:deep(.ai-docs-lead) {
  margin: 10px 0 0;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.7;
}

/* ===== Note 卡 ===== */
:deep(.ai-docs-note) {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 14px 16px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel-muted);
}

:deep(.ai-docs-note__head) {
  color: var(--ds-ink);
  font-size: 13px;
  font-weight: 700;
  line-height: 1.4;
}

:deep(.ai-docs-note__body) {
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.7;
}

:deep(.ai-docs-note__chips) {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

/* ===== Inline code ===== */
:deep(.ai-docs-inline-code) {
  padding: 1px 6px;
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
  color: var(--ds-ink);
  font-family: var(--ds-font-mono);
  font-size: 12px;
  font-weight: 700;
}

/* ===== List ===== */
:deep(.ai-docs-list) {
  margin: 0;
  padding-left: 18px;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.8;
}

:deep(.ai-docs-list li + li) {
  margin-top: 6px;
}

/* ===== Chip row / Chip ===== */
:deep(.ai-docs-chip-row) {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

:deep(.ai-docs-chip) {
  padding: 7px 10px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel);
  color: var(--ds-muted);
  font-family: var(--ds-font-mono);
  font-size: 12px;
  font-weight: 700;
}

/* ===== Badge ===== */
:deep(.ai-docs-badge) {
  display: inline-flex;
  align-items: center;
  padding: 5px 9px;
  border-radius: var(--ds-radius-control);
  font-size: 11px;
  font-weight: 700;
  line-height: 1;
}

:deep(.ai-docs-badge--chat) {
  background: var(--ds-info-soft);
  color: var(--ds-info);
}

:deep(.ai-docs-badge--image) {
  background: var(--ds-warning-soft);
  color: var(--ds-warning);
}

:deep(.ai-docs-badge--accent) {
  background: color-mix(in srgb, var(--ds-accent) 12%, var(--ds-panel));
  color: var(--ds-accent-hover);
}

/* ===== Callout ===== */
:deep(.ai-docs-callout) {
  padding: 14px 16px;
  border: 1px solid var(--ds-line);
  border-left: 3px solid var(--ds-accent);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel-muted);
}

:deep(.ai-docs-callout--info) {
  border-color: color-mix(in srgb, var(--ds-info) 22%, var(--ds-line));
  border-left-color: var(--ds-info);
  background: color-mix(in srgb, var(--ds-info-soft) 55%, var(--ds-panel));
}

:deep(.ai-docs-callout--warning) {
  border-color: color-mix(in srgb, var(--ds-warning) 22%, var(--ds-line));
  border-left-color: var(--ds-warning);
  background: color-mix(in srgb, var(--ds-warning-soft) 55%, var(--ds-panel));
}

:deep(.ai-docs-callout__title) {
  display: block;
  color: var(--ds-ink);
  font-size: 13px;
  font-weight: 700;
}

:deep(.ai-docs-callout__body) {
  margin: 8px 0 0;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.75;
}

/* ===== Rule grid / Rule card ===== */
:deep(.ai-docs-rule-grid) {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

:deep(.ai-docs-rule-card) {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
  padding: 14px 16px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel-muted);
  transition: border-color 0.16s ease;
}

:deep(.ai-docs-rule-card:hover) {
  border-color: color-mix(in srgb, var(--ds-accent) 24%, var(--ds-line));
}

:deep(.ai-docs-rule-card__label) {
  color: var(--ds-faint);
  font-size: 11px;
  font-weight: 700;
  line-height: 1.4;
}

:deep(.ai-docs-rule-card__value) {
  color: var(--ds-ink);
  font-family: var(--ds-font-mono);
  font-size: 13px;
  font-weight: 700;
  line-height: 1.5;
  word-break: break-word;
}

:deep(.ai-docs-rule-card__desc) {
  margin: 0;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.7;
}

/* ===== Section head / Section stack ===== */
:deep(.ai-docs-section-head) {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 14px;
}

:deep(.ai-docs-section-stack) {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

/* ===== Table ===== */
:deep(.ai-docs-table-caption) {
  color: var(--ds-ink);
  font-size: 12px;
  font-weight: 700;
  line-height: 1.5;
}

:deep(.ai-docs-table) {
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  overflow: hidden;
  font-size: 13px;
}

:deep(.ai-docs-table th) {
  padding: 10px 12px;
  background: var(--ds-panel-muted);
  border-bottom: 1px solid var(--ds-line);
  color: var(--ds-faint);
  text-align: left;
  font-size: 11px;
  font-weight: 700;
  line-height: 1.4;
}

:deep(.ai-docs-table td) {
  padding: 11px 12px;
  border-bottom: 1px solid var(--ds-line);
  color: var(--ds-muted);
  vertical-align: top;
  line-height: 1.7;
}

:deep(.ai-docs-table tr:last-child td) {
  border-bottom: 0;
}

:deep(.ai-docs-table code) {
  padding: 1px 6px;
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
  color: var(--ds-ink);
  font-family: var(--ds-font-mono);
  font-size: 12px;
  font-weight: 700;
}

/* ===== Endpoint list ===== */
:deep(.ai-docs-endpoint-list) {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  overflow: hidden;
}

:deep(.ai-docs-endpoint) {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 14px 16px;
  background: var(--ds-panel-muted);
  border-bottom: 1px solid var(--ds-line);
}

:deep(.ai-docs-endpoint-list .ai-docs-endpoint:last-child) {
  border-bottom: 0;
}

:deep(.ai-docs-endpoint__method) {
  flex-shrink: 0;
  min-width: 42px;
  padding: 4px 8px;
  border-radius: var(--ds-radius-control);
  background: color-mix(in srgb, var(--ds-accent) 12%, var(--ds-panel));
  color: var(--ds-accent-hover);
  font-family: var(--ds-font-mono);
  font-size: 11px;
  font-weight: 700;
  text-align: center;
}

:deep(.ai-docs-endpoint__path) {
  color: var(--ds-ink);
  font-family: var(--ds-font-mono);
  font-size: 13px;
  font-weight: 700;
  line-height: 1.6;
}

:deep(.ai-docs-endpoint__desc) {
  margin: 4px 0 0;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.7;
}

/* ===== 响应式（仅中等屏幕）===== */
@media (max-width: 1200px) {
  .ai-docs-tabs {
    overflow-x: auto;
  }
}

@media (max-width: 900px) {
  :deep(.ai-docs-grid),
  :deep(.ai-docs-grid--three),
  :deep(.ai-docs-rule-grid) {
    grid-template-columns: minmax(0, 1fr);
  }

  .ai-docs-header__top {
    grid-template-columns: 1fr;
  }
}
</style>
