<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    modelValue?: string;
    placeholder?: string;
    type?: string;
    disabled?: boolean;
    error?: string;
    size?: "sm" | "md";
  }>(),
  {
    modelValue: "",
    type: "text",
    disabled: false,
    size: "md"
  }
);

const emit = defineEmits<{
  "update:modelValue": [value: string];
}>();

function onInput(event: Event) {
  emit("update:modelValue", (event.target as HTMLInputElement).value);
}
</script>

<template>
  <div class="ds-input" :class="[`ds-input--${size}`, { 'ds-input--error': error, 'ds-input--disabled': disabled }]">
    <div v-if="$slots.prefix" class="ds-input__prefix">
      <slot name="prefix" />
    </div>
    <input
      class="ds-input__field"
      :type="type"
      :value="modelValue"
      :placeholder="placeholder"
      :disabled="disabled"
      @input="onInput"
    />
    <div v-if="$slots.suffix" class="ds-input__suffix">
      <slot name="suffix" />
    </div>
    <p v-if="error" class="ds-input__error">{{ error }}</p>
  </div>
</template>

<style scoped>
.ds-input {
  position: relative;
  display: flex;
  align-items: center;
  border: 1px solid var(--ds-line-strong);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel);
  transition:
    border-color 140ms ease,
    box-shadow 140ms ease;
}

.ds-input:focus-within {
  border-color: var(--ds-accent);
  box-shadow: var(--ds-shadow-focus);
}

.ds-input--error {
  border-color: var(--ds-danger);
}

.ds-input--error:focus-within {
  box-shadow: var(--ds-shadow-focus-danger);
}

.ds-input--disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.ds-input__field {
  flex: 1;
  min-width: 0;
  border: none;
  background: transparent;
  color: var(--ds-ink);
  outline: none;
}

.ds-input--md .ds-input__field {
  height: 36px;
  padding: 0 12px;
  font-size: 13px;
}

.ds-input--sm .ds-input__field {
  height: 28px;
  padding: 0 8px;
  font-size: 12px;
}

.ds-input__field::placeholder {
  color: var(--ds-muted);
}

.ds-input__prefix,
.ds-input__suffix {
  display: flex;
  align-items: center;
  color: var(--ds-muted);
  flex-shrink: 0;
}

.ds-input--md .ds-input__prefix,
.ds-input--md .ds-input__suffix {
  padding: 0 10px;
}

.ds-input--sm .ds-input__prefix,
.ds-input--sm .ds-input__suffix {
  padding: 0 6px;
}

.ds-input__error {
  position: absolute;
  left: 0;
  bottom: -18px;
  margin: 0;
  color: var(--ds-danger);
  font-size: 11px;
  white-space: nowrap;
}
</style>
