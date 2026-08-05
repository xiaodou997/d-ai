<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    modelValue?: boolean;
    disabled?: boolean;
    size?: "sm" | "md";
  }>(),
  {
    modelValue: false,
    disabled: false,
    size: "md"
  }
);

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

function toggle() {
  if (!props.disabled) {
    emit("update:modelValue", !props.modelValue);
  }
}
</script>

<template>
  <button
    type="button"
    role="switch"
    :aria-checked="modelValue"
    :disabled="disabled"
    class="ds-switch"
    :class="[`ds-switch--${size}`, { 'ds-switch--on': modelValue, 'ds-switch--disabled': disabled }]"
    @click="toggle"
  >
    <span class="ds-switch__thumb" />
  </button>
</template>

<style scoped>
.ds-switch {
  position: relative;
  display: inline-flex;
  align-items: center;
  border: 2px solid transparent;
  border-radius: 999px;
  background: var(--ds-line-strong);
  cursor: pointer;
  transition: background-color 180ms ease;
  flex-shrink: 0;
}

.ds-switch--md {
  width: 44px;
  height: 24px;
}

.ds-switch--sm {
  width: 36px;
  height: 20px;
}

.ds-switch__thumb {
  position: absolute;
  border-radius: 50%;
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
  transition: transform 180ms cubic-bezier(0.22, 1, 0.36, 1);
}

.ds-switch--md .ds-switch__thumb {
  top: 2px;
  left: 2px;
  width: 16px;
  height: 16px;
}

.ds-switch--sm .ds-switch__thumb {
  top: 2px;
  left: 2px;
  width: 12px;
  height: 12px;
}

.ds-switch--on {
  background: var(--ds-accent);
}

.ds-switch--md.ds-switch--on .ds-switch__thumb {
  transform: translateX(20px);
}

.ds-switch--sm.ds-switch--on .ds-switch__thumb {
  transform: translateX(16px);
}

.ds-switch--disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.ds-switch:not(.ds-switch--disabled):hover {
  box-shadow: 0 0 0 3px var(--ds-accent-soft);
}
</style>
