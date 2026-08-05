<script setup lang="ts">
import { watch } from "vue";

const props = withDefaults(
  defineProps<{
    open: boolean;
    title?: string;
    subtitle?: string;
    width?: string;
  }>(),
  {
    width: "520px"
  }
);

const emit = defineEmits<{
  close: [];
}>();

watch(
  () => props.open,
  (open) => {
    if (typeof document === "undefined") return;
    document.body.style.overflow = open ? "hidden" : "";
  }
);
</script>

<template>
  <Teleport to="body">
    <Transition name="ds-drawer">
      <div v-if="open" class="ds-drawer" @keydown.esc="emit('close')">
        <div class="ds-drawer__scrim" @click="emit('close')"></div>
        <aside class="ds-drawer__panel" :style="{ width }" role="dialog" aria-modal="true">
          <header class="ds-drawer__head">
            <div class="ds-drawer__copy">
              <h2 v-if="title" class="ds-drawer__title">{{ title }}</h2>
              <p v-if="subtitle" class="ds-drawer__subtitle">{{ subtitle }}</p>
            </div>
            <button type="button" class="ds-drawer__close" aria-label="关闭" @click="emit('close')">×</button>
          </header>
          <div class="ds-drawer__body">
            <slot />
          </div>
          <footer v-if="$slots.footer" class="ds-drawer__foot">
            <slot name="footer" />
          </footer>
        </aside>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.ds-drawer {
  position: fixed;
  inset: 0;
  z-index: 60;
  display: flex;
  justify-content: flex-end;
}

.ds-drawer__scrim {
  position: absolute;
  inset: 0;
  background: color-mix(in srgb, var(--ds-ink) 38%, transparent);
}

.ds-drawer__panel {
  position: relative;
  display: flex;
  max-width: 92vw;
  height: 100%;
  flex-direction: column;
  background: var(--ds-panel);
  border-left: 1px solid var(--ds-line);
  box-shadow: var(--ds-shadow-pop);
}

.ds-drawer__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid var(--ds-line);
  padding: 18px 22px;
}

.ds-drawer__title {
  margin: 0;
  font-size: 17px;
  font-weight: 600;
  color: var(--ds-ink);
}

.ds-drawer__subtitle {
  margin: 4px 0 0;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.5;
}

.ds-drawer__close {
  display: grid;
  height: 30px;
  width: 30px;
  flex: 0 0 auto;
  place-items: center;
  border: 1px solid transparent;
  border-radius: var(--ds-radius-control);
  background: transparent;
  color: var(--ds-muted);
  font-size: 20px;
  line-height: 1;
  cursor: pointer;
}

.ds-drawer__close:hover {
  background: var(--ds-panel-muted);
  color: var(--ds-ink);
}

.ds-drawer__body {
  flex: 1;
  overflow: auto;
  padding: 22px;
}

.ds-drawer__foot {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  border-top: 1px solid var(--ds-line);
  padding: 14px 22px;
}

.ds-drawer-enter-active,
.ds-drawer-leave-active {
  transition: opacity 180ms ease;
}

.ds-drawer-enter-active .ds-drawer__panel,
.ds-drawer-leave-active .ds-drawer__panel {
  transition: transform 220ms cubic-bezier(0.22, 1, 0.36, 1);
}

.ds-drawer-enter-from,
.ds-drawer-leave-to {
  opacity: 0;
}

.ds-drawer-enter-from .ds-drawer__panel,
.ds-drawer-leave-to .ds-drawer__panel {
  transform: translateX(100%);
}
</style>
