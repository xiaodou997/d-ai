<script setup lang="ts">
import { computed, ref, useAttrs, watch } from "vue";

defineOptions({ inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    modelValue?: number | null;
    min?: number;
    max?: number;
    step?: number;
    precision?: number;
    allowEmpty?: boolean;
    placeholder?: string;
    disabled?: boolean;
    readonly?: boolean;
    error?: string;
    size?: "sm" | "md";
    align?: "left" | "right";
  }>(),
  {
    modelValue: null,
    step: 1,
    precision: 0,
    allowEmpty: false,
    disabled: false,
    readonly: false,
    size: "md",
    align: "right"
  }
);

const emit = defineEmits<{
  "update:modelValue": [value: number | null];
  change: [value: number | null];
  focus: [event: FocusEvent];
  blur: [event: FocusEvent];
}>();

const attrs = useAttrs();
const input = ref<HTMLInputElement>();
const focused = ref(false);
const focusValue = ref<number | null>(props.modelValue ?? null);

const normalizedPrecision = computed(() => {
  const precision = Number.isFinite(props.precision) ? Math.trunc(props.precision) : 0;
  return Math.min(12, Math.max(0, precision));
});

const inputAttrs = computed(() => {
  const { class: _class, style: _style, ...rest } = attrs;
  return rest;
});

function round(value: number): number {
  const rounded = Number(value.toFixed(normalizedPrecision.value));
  return Object.is(rounded, -0) ? 0 : rounded;
}

function normalize(value: number): number {
  let normalized = value;
  if (props.min !== undefined) normalized = Math.max(props.min, normalized);
  if (props.max !== undefined) normalized = Math.min(props.max, normalized);
  normalized = round(normalized);
  if (props.min !== undefined) normalized = Math.max(props.min, normalized);
  if (props.max !== undefined) normalized = Math.min(props.max, normalized);
  return normalized;
}

function format(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return "";
  const fixed = round(value).toFixed(normalizedPrecision.value);
  if (normalizedPrecision.value === 0) return fixed;
  return fixed.replace(/\.?0+$/, "");
}

const draft = ref(format(props.modelValue));

watch(
  () => [props.modelValue, props.precision] as const,
  ([value]) => {
    if (!focused.value) draft.value = format(value);
  }
);

function isAllowedDraft(value: string): boolean {
  if (!/^-?\d*(?:\.\d*)?$/.test(value)) return false;
  return !(props.min !== undefined && props.min >= 0 && value.startsWith("-"));
}

function parseDraft(value: string): number | null {
  if (value === "" || value === "-" || value === "." || value === "-.") return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function withinRange(value: number): boolean {
  if (props.min !== undefined && value < props.min) return false;
  if (props.max !== undefined && value > props.max) return false;
  return true;
}

function onInput(event: Event) {
  const target = event.target as HTMLInputElement;
  const nextDraft = target.value;
  if (!isAllowedDraft(nextDraft)) {
    target.value = draft.value;
    return;
  }

  draft.value = nextDraft;
  if (nextDraft === "") {
    if (props.allowEmpty) emit("update:modelValue", null);
    return;
  }

  const parsed = parseDraft(nextDraft);
  if (parsed !== null && withinRange(parsed)) {
    emit("update:modelValue", normalize(parsed));
  }
}

function commit(): number | null {
  const parsed = parseDraft(draft.value);
  if (parsed === null) {
    if (props.allowEmpty && draft.value === "") {
      emit("update:modelValue", null);
      return null;
    }
    const fallback = props.modelValue ?? focusValue.value;
    draft.value = format(fallback);
    return fallback;
  }

  const value = normalize(parsed);
  draft.value = format(value);
  emit("update:modelValue", value);
  return value;
}

function onFocus(event: FocusEvent) {
  focused.value = true;
  focusValue.value = props.modelValue ?? null;
  emit("focus", event);
}

function onBlur(event: FocusEvent) {
  const value = commit();
  focused.value = false;
  if (value !== focusValue.value) emit("change", value);
  emit("blur", event);
}

function stepBy(direction: 1 | -1) {
  const parsed = parseDraft(draft.value);
  const base = parsed ?? props.modelValue ?? 0;
  const step = Number.isFinite(props.step) && props.step > 0 ? props.step : 1;
  const value = normalize(base + step * direction);
  draft.value = format(value);
  emit("update:modelValue", value);
}

function onKeydown(event: KeyboardEvent) {
  if (props.disabled || props.readonly) return;
  if (event.key === "Enter") {
    event.preventDefault();
    input.value?.blur();
    return;
  }
  if (event.key === "ArrowUp" || event.key === "ArrowDown") {
    event.preventDefault();
    stepBy(event.key === "ArrowUp" ? 1 : -1);
  }
}
</script>

<template>
  <div
    class="ds-number-input"
    :class="[
      `ds-number-input--${size}`,
      `ds-number-input--${align}`,
      { 'ds-number-input--error': error, 'ds-number-input--disabled': disabled },
      attrs.class
    ]"
    :style="attrs.style"
  >
    <div v-if="$slots.prefix" class="ds-number-input__prefix"><slot name="prefix" /></div>
    <input
      ref="input"
      v-bind="inputAttrs"
      class="ds-number-input__field"
      type="text"
      :inputmode="normalizedPrecision > 0 ? 'decimal' : 'numeric'"
      :value="draft"
      :placeholder="placeholder"
      :disabled="disabled"
      :readonly="readonly"
      role="spinbutton"
      :aria-valuemin="min"
      :aria-valuemax="max"
      :aria-valuenow="modelValue ?? undefined"
      :aria-invalid="error ? 'true' : undefined"
      @input="onInput"
      @focus="onFocus"
      @blur="onBlur"
      @keydown="onKeydown"
    />
    <div v-if="$slots.suffix" class="ds-number-input__suffix"><slot name="suffix" /></div>
    <p v-if="error" class="ds-number-input__error">{{ error }}</p>
  </div>
</template>

<style scoped>
.ds-number-input {
  position: relative;
  display: flex;
  align-items: center;
  width: 100%;
  border: 1px solid var(--ds-line-strong);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel);
  transition:
    border-color 140ms ease,
    box-shadow 140ms ease;
}

.ds-number-input:focus-within {
  border-color: var(--ds-accent);
  box-shadow: var(--ds-shadow-focus);
}

.ds-number-input--error {
  border-color: var(--ds-danger);
}

.ds-number-input--error:focus-within {
  box-shadow: var(--ds-shadow-focus-danger);
}

.ds-number-input--disabled {
  background: var(--ds-panel-muted);
  opacity: 0.6;
  cursor: not-allowed;
}

.ds-number-input__field {
  flex: 1;
  min-width: 0;
  border: none;
  background: transparent;
  color: var(--ds-ink);
  font-variant-numeric: tabular-nums;
  outline: none;
}

.ds-number-input--right .ds-number-input__field {
  text-align: right;
}

.ds-number-input--left .ds-number-input__field {
  text-align: left;
}

.ds-number-input--md .ds-number-input__field {
  height: 32px;
  padding: 0 11px;
  font-size: 13px;
}

.ds-number-input--sm .ds-number-input__field {
  height: 24px;
  padding: 0 8px;
  font-size: 12px;
}

.ds-number-input__field::placeholder {
  color: var(--ds-muted);
}

.ds-number-input__prefix,
.ds-number-input__suffix {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  color: var(--ds-muted);
  font-size: 12px;
}

.ds-number-input__prefix {
  padding-left: 10px;
}

.ds-number-input__suffix {
  padding-right: 10px;
}

.ds-number-input__error {
  position: absolute;
  bottom: -18px;
  left: 0;
  margin: 0;
  color: var(--ds-danger);
  font-size: 11px;
  white-space: nowrap;
}
</style>
