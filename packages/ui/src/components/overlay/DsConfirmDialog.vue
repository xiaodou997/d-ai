<script setup lang="ts">
import { computed } from "vue";
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

function onCancel() {
  if (!props.loading) {
    emit("update:open", false);
    emit("cancel");
  }
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
      <div v-if="open" class="ds-confirm">
        <div class="ds-confirm__scrim" @click="onCancel" />
        <div class="ds-confirm__panel" role="alertdialog" aria-modal="true">
          <div class="ds-confirm__icon" :class="toneIcon">
            <AlertTriangle :size="22" />
          </div>
          <h3 v-if="title" class="ds-confirm__title">{{ title }}</h3>
          <p v-if="message" class="ds-confirm__message">{{ message }}</p>
          <div v-else class="ds-confirm__message">
            <slot />
          </div>
          <div class="ds-confirm__actions">
            <DsButton variant="secondary" size="sm" :disabled="loading" @click="onCancel">
              {{ cancelText }}
            </DsButton>
            <DsButton
              :variant="tone === 'danger' ? 'danger' : 'primary'"
              size="sm"
              :disabled="loading"
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

.ds-confirm__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border-radius: 50%;
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
