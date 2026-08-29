<script setup lang="ts">
withDefaults(
  defineProps<{
    variant?:
      | "primary"
      | "secondary"
      | "ghost"
      | "danger"
      | "link"
      | "link-success"
      | "link-warning"
      | "link-danger";
    size?: "sm" | "md";
    type?: "button" | "submit" | "reset";
    disabled?: boolean;
    block?: boolean;
  }>(),
  {
    variant: "secondary",
    size: "md",
    type: "button",
    disabled: false,
    block: false
  }
);
</script>

<template>
  <button
    class="ds-btn"
    :class="[`ds-btn--${variant}`, `ds-btn--${size}`, { 'ds-btn--block': block }]"
    :type="type"
    :disabled="disabled"
  >
    <slot name="icon" />
    <slot />
  </button>
</template>

<style scoped>
.ds-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  border: 1px solid transparent;
  border-radius: var(--ds-radius-control);
  cursor: pointer;
  font-weight: 600;
  line-height: 1;
  white-space: nowrap;
  transition:
    background-color 140ms ease,
    border-color 140ms ease,
    color 140ms ease,
    box-shadow 140ms ease;
}

.ds-btn--md {
  min-height: 36px;
  padding: 0 14px;
  font-size: 13px;
}

.ds-btn--sm {
  min-height: 28px;
  padding: 0 10px;
  font-size: 12px;
  border-radius: var(--ds-radius-sm);
}

.ds-btn--block {
  width: 100%;
}

.ds-btn--primary {
  background: var(--ds-accent);
  border-color: var(--ds-accent);
  color: var(--ds-accent-contrast);
}

.ds-btn--primary:hover:not(:disabled) {
  background: var(--ds-accent-hover);
  border-color: var(--ds-accent-hover);
}

.ds-btn--secondary {
  background: var(--ds-panel);
  border-color: var(--ds-line-strong);
  color: var(--ds-ink-soft);
}

.ds-btn--secondary:hover:not(:disabled) {
  border-color: var(--ds-accent);
  color: var(--ds-accent);
}

.ds-btn--ghost {
  background: transparent;
  border-color: transparent;
  color: var(--ds-muted);
}

.ds-btn--ghost:hover:not(:disabled) {
  background: var(--ds-panel-muted);
  color: var(--ds-ink);
}

.ds-btn--danger {
  background: var(--ds-panel);
  border-color: var(--ds-line-strong);
  color: var(--ds-danger);
}

.ds-btn--danger:hover:not(:disabled) {
  background: var(--ds-danger);
  border-color: var(--ds-danger);
  color: var(--ds-white);
}

.ds-btn--link {
  background: transparent;
  border-color: transparent;
  color: var(--ds-accent);
  padding-left: 4px;
  padding-right: 4px;
}

.ds-btn--link:hover:not(:disabled) {
  color: var(--ds-accent-hover);
  background: transparent;
}

.ds-btn--link-success {
  background: transparent;
  border-color: transparent;
  color: var(--ds-positive);
  padding-left: 4px;
  padding-right: 4px;
}

.ds-btn--link-success:hover:not(:disabled) {
  opacity: 0.75;
  background: transparent;
}

.ds-btn--link-warning {
  background: transparent;
  border-color: transparent;
  color: var(--ds-warning);
  padding-left: 4px;
  padding-right: 4px;
}

.ds-btn--link-warning:hover:not(:disabled) {
  opacity: 0.75;
  background: transparent;
}

.ds-btn--link-danger {
  background: transparent;
  border-color: transparent;
  color: var(--ds-danger);
  padding-left: 4px;
  padding-right: 4px;
}

.ds-btn--link-danger:hover:not(:disabled) {
  opacity: 0.75;
  background: transparent;
}

.ds-btn:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}
</style>
