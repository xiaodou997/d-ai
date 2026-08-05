<script setup lang="ts">
interface RangeOption {
  id: string;
  label: string;
}

const props = withDefaults(
  defineProps<{
    modelValue: string;
    options: RangeOption[];
    ariaLabel?: string;
    loading?: boolean;
    tone?: "blue" | "teal" | "amber";
  }>(),
  {
    ariaLabel: "时间窗口",
    loading: false,
    tone: "blue"
  }
);

const emit = defineEmits<{
  "update:modelValue": [value: string];
}>();

const selectOption = (id: string) => {
  if (props.loading || id === props.modelValue) return;
  emit("update:modelValue", id);
};
</script>

<template>
  <div class="range-tabs" role="group" :aria-label="ariaLabel">
    <button
      v-for="option in options"
      :key="option.id"
      type="button"
      class="range-tabs__btn"
      :class="{ 'is-active': option.id === modelValue }"
      :aria-pressed="option.id === modelValue"
      :disabled="loading"
      @click="selectOption(option.id)"
    >
      {{ option.label }}
    </button>
  </div>
</template>

<style scoped>
.range-tabs {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 2px;
  border-radius: var(--ds-radius-pill);
  border: 1px solid var(--ds-line);
  background: var(--ds-panel-muted);
  padding: 3px;
}

.range-tabs__btn {
  white-space: nowrap;
  border-radius: var(--ds-radius-pill);
  border: 0;
  background: transparent;
  padding: 6px 10px;
  font-size: 11px;
  font-weight: 700;
  color: var(--ds-muted);
  cursor: pointer;
  transition: all 0.15s ease;
}

.range-tabs__btn:hover {
  color: var(--ds-ink-soft);
}

.range-tabs__btn.is-active {
  background: var(--ds-panel);
  color: var(--ds-accent-hover);
  box-shadow: var(--ds-shadow-sm);
}

.range-tabs__btn:disabled {
  cursor: not-allowed;
}
</style>
