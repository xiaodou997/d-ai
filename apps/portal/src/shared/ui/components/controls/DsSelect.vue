<script setup lang="ts">
import { computed, useAttrs, useId } from "vue";
import { ChevronDown } from "lucide-vue-next";

defineOptions({ inheritAttrs: false });

interface SelectOption {
  label: string;
  value: string | number;
  disabled?: boolean;
}

const props = withDefaults(
  defineProps<{
    modelValue?: string | number;
    placeholder?: string;
    options: SelectOption[];
    disabled?: boolean;
    error?: string;
    size?: "sm" | "md";
  }>(),
  {
    modelValue: "",
    placeholder: "请选择",
    disabled: false,
    size: "md"
  }
);

const attrs = useAttrs();
const generatedId = useId();
const selectId = computed(() =>
  typeof attrs.id === "string" ? attrs.id : `ds-select-${generatedId}`
);
const selectAttrs = computed(() => {
  const {
    class: _class,
    style: _style,
    id: _id,
    "aria-invalid": _ariaInvalid,
    "aria-describedby": _ariaDescribedBy,
    ...rest
  } = attrs;
  return rest;
});
const describedBy = computed(() => {
  const ids = [attrs["aria-describedby"], props.error ? `${selectId.value}-error` : undefined]
    .filter((value): value is string => typeof value === "string" && value.length > 0)
    .join(" ");
  return ids || undefined;
});

const emit = defineEmits<{
  "update:modelValue": [value: string | number];
}>();

function onChange(event: Event) {
  const val = (event.target as HTMLSelectElement).value;
  emit("update:modelValue", val === "" ? "" : Number(val) || val);
}
</script>

<template>
  <div
    class="ds-select"
    :class="[
      `ds-select--${size}`,
      { 'ds-select--error': error, 'ds-select--disabled': disabled },
      attrs.class
    ]"
    :style="attrs.style"
  >
    <select
      v-bind="selectAttrs"
      class="ds-select__field"
      :id="selectId"
      :value="modelValue"
      :disabled="disabled"
      :aria-invalid="error ? 'true' : undefined"
      :aria-describedby="describedBy"
      @change="onChange"
    >
      <option value="" disabled>{{ placeholder }}</option>
      <option
        v-for="opt in options"
        :key="opt.value"
        :value="opt.value"
        :disabled="opt.disabled"
      >
        {{ opt.label }}
      </option>
    </select>
    <ChevronDown class="ds-select__arrow" :size="14" />
    <p v-if="error" :id="`${selectId}-error`" class="ds-select__error" aria-live="polite">{{ error }}</p>
  </div>
</template>

<style scoped>
.ds-select {
  position: relative;
  display: flex;
  align-items: center;
}

.ds-select__field {
  appearance: none;
  width: 100%;
  border: 1px solid var(--ds-line-strong);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel);
  color: var(--ds-ink);
  outline: none;
  padding-right: 30px;
  cursor: pointer;
  transition:
    border-color 140ms ease,
    box-shadow 140ms ease;
}

.ds-select__field:focus {
  border-color: var(--ds-accent);
  box-shadow: var(--ds-shadow-focus);
}

.ds-select--md .ds-select__field {
  height: var(--ds-control-height-md);
  padding-left: 12px;
  font-size: 13px;
}

.ds-select--sm .ds-select__field {
  height: var(--ds-control-height-sm);
  padding-left: 8px;
  font-size: 12px;
}

.ds-select--error .ds-select__field {
  border-color: var(--ds-danger);
}

.ds-select--error .ds-select__field:focus {
  box-shadow: var(--ds-shadow-focus-danger);
}

.ds-select--disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.ds-select__arrow {
  position: absolute;
  right: 10px;
  color: var(--ds-muted);
  pointer-events: none;
}

.ds-select__error {
  position: absolute;
  left: 0;
  bottom: -18px;
  margin: 0;
  color: var(--ds-danger);
  font-size: 11px;
  white-space: nowrap;
}
</style>
