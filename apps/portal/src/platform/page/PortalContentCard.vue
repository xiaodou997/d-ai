<script setup lang="ts">
import { computed, useSlots } from "vue";

const slots = useSlots();

const props = withDefaults(
  defineProps<{
    title?: string;
    description?: string;
    bodyPadding?: "none" | "md";
  }>(),
  {
    bodyPadding: "md"
  }
);

const hasHeader = computed(() =>
  Boolean(props.title || props.description || slots.header || slots.meta || slots.actions)
);

const hasFooter = computed(() => Boolean(slots.footer));
</script>

<template>
  <section class="portal-content-card">
    <header v-if="hasHeader" class="portal-content-card__head">
      <div class="portal-content-card__copy">
        <slot name="header">
          <div v-if="title || description">
            <h2 v-if="title" class="portal-content-card__title">{{ title }}</h2>
            <p v-if="description" class="portal-content-card__description">{{ description }}</p>
          </div>
        </slot>
      </div>

      <div v-if="$slots.meta || $slots.actions" class="portal-content-card__aside">
        <slot name="meta" />
        <slot name="actions" />
      </div>
    </header>

    <div class="portal-content-card__body" :class="`portal-content-card__body--${bodyPadding}`">
      <slot />
    </div>

    <footer v-if="hasFooter" class="portal-content-card__footer">
      <slot name="footer" />
    </footer>
  </section>
</template>

<style scoped>
.portal-content-card {
  overflow: hidden;
  min-width: 0;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.portal-content-card__head {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: start;
  gap: 16px;
  padding: 17px 20px;
  border-bottom: 1px solid var(--ds-line);
}

.portal-content-card__copy {
  min-width: 0;
}

.portal-content-card__title {
  margin: 0;
  font-size: 16px;
  font-weight: 650;
  letter-spacing: -0.02em;
  color: var(--ds-ink);
}

.portal-content-card__description {
  margin: 5px 0 0;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.6;
}

.portal-content-card__aside {
  display: flex;
  align-items: flex-start;
  justify-content: flex-end;
  gap: 10px;
  flex-wrap: wrap;
  min-width: 0;
}

.portal-content-card__body {
  min-width: 0;
}

.portal-content-card__body--none {
  padding: 0;
}

.portal-content-card__body--md {
  padding: 18px 20px;
}

.portal-content-card__footer {
  border-top: 1px solid var(--ds-line);
}

@media (max-width: 768px) {
  .portal-content-card__head {
    grid-template-columns: 1fr;
  }

  .portal-content-card__aside {
    justify-content: flex-start;
  }
}
</style>
