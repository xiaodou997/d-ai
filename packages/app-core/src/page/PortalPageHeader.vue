<script setup lang="ts">
import { DsTag } from "@dai/ui";

defineProps<{
  title: string;
  description?: string;
  eyebrow?: string;
  badge?: string;
}>();
</script>

<template>
  <section class="portal-page-header">
    <div class="portal-page-header__copy">
      <div v-if="eyebrow" class="portal-page-header__eyebrow">{{ eyebrow }}</div>

      <div class="portal-page-header__title-row">
        <h1 class="portal-page-header__title">{{ title }}</h1>
        <slot name="badge">
          <DsTag v-if="badge" tone="neutral">{{ badge }}</DsTag>
        </slot>
      </div>

      <p v-if="description" class="portal-page-header__description">{{ description }}</p>
    </div>

    <div v-if="$slots.actions" class="portal-page-header__actions">
      <slot name="actions" />
    </div>
  </section>
</template>

<style scoped>
.portal-page-header {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: start;
  gap: 16px;
  min-height: 88px;
  padding: 18px 22px;
  border: 1px solid color-mix(in srgb, var(--ds-accent) 14%, var(--ds-line));
  border-radius: var(--ds-radius-panel);
  /* 主题渐变身份卡：左侧主题色柔和渐隐到右侧纯白 —— 颜色即门户身份，
     一眼识别 admin/tenant/customer；左包标题、右留白给操作区，秩序更清爽。
     用 color-mix 把 accent 本色低比例混入，色相清晰但背景仍淡、不压文字。 */
  background: linear-gradient(
    90deg,
    color-mix(in srgb, var(--ds-accent) 9%, var(--ds-panel)) 0%,
    color-mix(in srgb, var(--ds-accent) 3%, var(--ds-panel)) 42%,
    var(--ds-panel) 70%
  );
  box-shadow: var(--ds-shadow-sm);
}

.portal-page-header__copy {
  min-width: 0;
}

.portal-page-header__eyebrow {
  margin-bottom: 8px;
  color: var(--ds-faint);
  font-size: 12px;
  font-weight: 700;
  line-height: 1.4;
}

.portal-page-header__title-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.portal-page-header__title {
  margin: 0;
  font-size: 24px;
  font-weight: 680;
  letter-spacing: -0.03em;
  line-height: 1.2;
  color: var(--ds-ink);
}

.portal-page-header__description {
  margin: 8px 0 0;
  max-width: 720px;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.6;
}

.portal-page-header__actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  flex-wrap: wrap;
}

@media (max-width: 768px) {
  .portal-page-header {
    grid-template-columns: 1fr;
    min-height: auto;
    padding: 16px 18px;
  }

  .portal-page-header__actions {
    justify-content: flex-start;
  }
}
</style>
