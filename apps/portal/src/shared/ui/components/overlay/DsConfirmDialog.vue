<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, useId, watch } from "vue";
import { AlertTriangle } from "lucide-vue-next";
import DsButton from "../controls/DsButton.vue";

const props = withDefaults(
  defineProps<{
    open: boolean;
    title?: string;
    message?: string;
    tone?: "danger" | "warning" | "info";
    confirmText?: string;
    cancelText?: string;
    loading?: boolean;
  }>(),
  {
    title: "确认操作",
    tone: "danger",
    confirmText: "确认",
    cancelText: "取消",
    loading: false
  }
);

const emit = defineEmits<{
  confirm: [];
  cancel: [];
  "update:open": [value: boolean];
}>();

const dialogId = useId();
const titleId = `ds-confirm-${dialogId}-title`;
const messageId = `ds-confirm-${dialogId}-message`;
const panel = ref<HTMLElement | null>(null);
const restoreFocus = ref<HTMLElement | null>(null);
const previousBodyOverflow = ref<string | null>(null);

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
    document.body.style.overflow = previousBodyOverflow.value ?? "";
    previousBodyOverflow.value = null;
    const target = restoreFocus.value;
    restoreFocus.value = null;
    if (target?.isConnected) nextTick(() => target.focus({ preventScroll: true }));
  },
  { immediate: true }
);

onBeforeUnmount(() => {
  if (!props.open) return;
  document.body.style.overflow = previousBodyOverflow.value ?? "";
  previousBodyOverflow.value = null;
  const target = restoreFocus.value;
  if (target?.isConnected) target.focus({ preventScroll: true });
});

function onCancel() {
  if (!props.loading) {
    emit("update:open", false);
    emit("cancel");
  }
}

function onEscape() {
  if (!props.loading) onCancel();
}

const toneIcon = computed(() => {
  const map: Record<string, string> = {
    danger: "ds-confirm__icon--danger",
    warning: "ds-confirm__icon--warning",
    info: "ds-confirm__icon--info"
  };
  return map[props.tone] || map.danger;
});
</script>

<template>
  <Teleport to="body">
    <Transition name="ds-confirm">
      <div v-if="open" class="ds-confirm" @keydown.esc.stop.prevent="onEscape">
        <div class="ds-confirm__scrim" aria-hidden="true" @click="onCancel" />
        <div
          ref="panel"
          class="ds-confirm__panel"
          role="alertdialog"
          aria-modal="true"
          tabindex="-1"
          :aria-label="title ? undefined : '确认操作'"
          :aria-labelledby="title ? titleId : undefined"
          :aria-describedby="messageId"
        >
          <div class="ds-confirm__icon" :class="toneIcon">
            <AlertTriangle :size="22" />
          </div>
          <h3 v-if="title" :id="titleId" class="ds-confirm__title">{{ title }}</h3>
          <p v-if="message" :id="messageId" class="ds-confirm__message">{{ message }}</p>
          <div v-else :id="messageId" class="ds-confirm__message">
            <slot />
          </div>
          <div class="ds-confirm__actions">
            <DsButton variant="secondary" size="sm" :disabled="loading" @click="onCancel">
              {{ cancelText }}
            </DsButton>
            <DsButton
              :variant="tone === 'danger' ? 'danger' : 'primary'"
              size="sm"
              :loading="loading"
              @click="emit('confirm')"
            >
              {{ loading ? "处理中…" : confirmText }}
            </DsButton>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.ds-confirm {
  position: fixed;
  inset: 0;
  z-index: 80;
  display: grid;
  place-items: center;
  padding: 24px;
}

.ds-confirm__scrim {
  position: absolute;
  inset: 0;
  background: color-mix(in srgb, var(--ds-ink) 42%, transparent);
}

.ds-confirm__panel {
  position: relative;
  width: min(380px, 100%);
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-pop);
  padding: 24px;
  text-align: center;
}

.ds-confirm__panel:focus-visible {
  outline: 2px solid var(--ds-accent);
  outline-offset: 2px;
}

.ds-confirm__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border-radius: var(--ds-radius-circle);
  margin-bottom: 14px;
}

.ds-confirm__icon--danger {
  background: var(--ds-danger-soft);
  color: var(--ds-danger);
}

.ds-confirm__icon--warning {
  background: var(--ds-warning-soft);
  color: var(--ds-warning);
}

.ds-confirm__icon--info {
  background: var(--ds-accent-soft);
  color: var(--ds-accent);
}

.ds-confirm__title {
  margin: 0 0 6px;
  font-size: 16px;
  font-weight: 600;
  color: var(--ds-ink);
}

.ds-confirm__message {
  color: var(--ds-ink-soft);
  font-size: 14px;
  line-height: 1.6;
  margin-bottom: 20px;
}

.ds-confirm__actions {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
}

/* Transitions */
.ds-confirm-enter-active,
.ds-confirm-leave-active {
  transition: opacity 160ms ease;
}

.ds-confirm-enter-active .ds-confirm__panel,
.ds-confirm-leave-active .ds-confirm__panel {
  transition: transform 180ms ease;
}

.ds-confirm-enter-from,
.ds-confirm-leave-to {
  opacity: 0;
}

.ds-confirm-enter-from .ds-confirm__panel,
.ds-confirm-leave-to .ds-confirm__panel {
  transform: translateY(8px) scale(0.96);
}
</style>
