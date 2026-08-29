<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, useId, watch } from "vue";

const props = withDefaults(
  defineProps<{
    open: boolean;
    title?: string;
    tone?: "default" | "danger";
  }>(),
  {
    tone: "default"
  }
);

const modalId = useId();
const titleId = `ds-modal-${modalId}-title`;
const bodyId = `ds-modal-${modalId}-body`;
const panel = ref<HTMLElement | null>(null);
const restoreFocus = ref<HTMLElement | null>(null);
const previousBodyOverflow = ref<string | null>(null);

function restoreFocusAndOverflow() {
  if (typeof document === "undefined") return;
  document.body.style.overflow = previousBodyOverflow.value ?? "";
  previousBodyOverflow.value = null;
  const target = restoreFocus.value;
  restoreFocus.value = null;
  if (target?.isConnected) nextTick(() => target.focus({ preventScroll: true }));
}

watch(
  () => props.open,
  (open) => {
    if (typeof document === "undefined") return;
    if (open) {
      restoreFocus.value = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      previousBodyOverflow.value = document.body.style.overflow;
      document.body.style.overflow = "hidden";
      nextTick(() => panel.value?.focus({ preventScroll: true }));
      return;
    }
    restoreFocusAndOverflow();
  },
  { immediate: true }
);

onBeforeUnmount(() => {
  if (props.open) restoreFocusAndOverflow();
});

const emit = defineEmits<{
  close: [];
}>();
</script>

<template>
  <Teleport to="body">
    <Transition name="ds-modal">
      <div v-if="open" class="ds-modal" @keydown.esc.stop.prevent="emit('close')">
        <div class="ds-modal__scrim" aria-hidden="true" @click="emit('close')"></div>
        <div
          ref="panel"
          class="ds-modal__panel"
          role="dialog"
          aria-modal="true"
          tabindex="-1"
          :aria-label="title ? undefined : '对话框'"
          :aria-labelledby="title ? titleId : undefined"
          :aria-describedby="bodyId"
        >
          <h2 v-if="title" :id="titleId" class="ds-modal__title" :class="{ 'is-danger': tone === 'danger' }">{{ title }}</h2>
          <div :id="bodyId" class="ds-modal__body">
            <slot />
          </div>
          <div class="ds-modal__foot">
            <slot name="footer" />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.ds-modal {
  position: fixed;
  inset: 0;
  z-index: 70;
  display: grid;
  place-items: center;
  padding: 24px;
}

.ds-modal__scrim {
  position: absolute;
  inset: 0;
  background: color-mix(in srgb, var(--ds-ink) 42%, transparent);
}

.ds-modal__panel {
  position: relative;
  width: min(440px, 100%);
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-pop);
  padding: 22px;
}

.ds-modal__panel:focus-visible {
  outline: 2px solid var(--ds-accent);
  outline-offset: 2px;
}

.ds-modal__title {
  margin: 0 0 10px;
  font-size: 16px;
  font-weight: 600;
  color: var(--ds-ink);
}

.ds-modal__title.is-danger {
  color: var(--ds-danger);
}

.ds-modal__body {
  color: var(--ds-ink-soft);
  font-size: 14px;
  line-height: 1.6;
}

.ds-modal__foot {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 20px;
}

.ds-modal-enter-active,
.ds-modal-leave-active {
  transition: opacity 160ms ease;
}

.ds-modal-enter-active .ds-modal__panel,
.ds-modal-leave-active .ds-modal__panel {
  transition: transform 180ms ease;
}

.ds-modal-enter-from,
.ds-modal-leave-to {
  opacity: 0;
}

.ds-modal-enter-from .ds-modal__panel,
.ds-modal-leave-to .ds-modal__panel {
  transform: translateY(8px) scale(0.98);
}
</style>
