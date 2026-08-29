<!--
  面板内子集切换控件。
  重构:下划线式 → 分段控件(Segmented) —— 灰底胶囊槽 + 白色激活块 + 细阴影。
       动因:旧版 2px 下划线在满屏表格线里读不出选中态;且组件自带的 border-bottom
       既当激活基线又当区域分隔线,而全部 14 处调用都把它放在带左右内边距的容器里,
       那条线于是每页都断在半路。现在激活态由色块承担,分隔线不再由本组件渲染 ——
       需要的页面在通栏容器上加 border-bottom 即可(容器边框才是真通栏的)。
-->
<script setup lang="ts">
export interface DsTabItem {
  key: string;
  label: string;
}

const props = defineProps<{
  tabs: DsTabItem[];
  modelValue: string;
}>();

const emit = defineEmits<{ "update:modelValue": [key: string] }>();

function onKeydown(event: KeyboardEvent, index: number) {
  const { key } = event;
  if (!["ArrowRight", "ArrowLeft", "Home", "End"].includes(key)) return;

  event.preventDefault();
  const targetIndex =
    key === "Home"
      ? 0
      : key === "End"
        ? props.tabs.length - 1
        : (index + (key === "ArrowRight" ? 1 : -1) + props.tabs.length) % props.tabs.length;
  const currentTarget = event.currentTarget as HTMLElement | null;
  const target = currentTarget?.parentElement?.querySelectorAll<HTMLButtonElement>("[role='tab']")?.[targetIndex];
  const targetTab = targetIndex >= 0 ? target : undefined;
  if (!targetTab) return;
  targetTab.focus();
  emit("update:modelValue", props.tabs[targetIndex]?.key ?? "");
}
</script>

<template>
  <div class="ds-tabs">
    <div class="ds-tabs__list" role="tablist" aria-orientation="horizontal">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        type="button"
        role="tab"
        class="ds-tabs__tab"
        :class="{ 'ds-tabs__tab--active': tab.key === modelValue }"
        :aria-selected="tab.key === modelValue"
        :tabindex="tab.key === modelValue ? 0 : -1"
        :data-tab-key="tab.key"
        @keydown="onKeydown($event, tabs.indexOf(tab))"
        @click="emit('update:modelValue', tab.key)"
      >
        {{ tab.label }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.ds-tabs {
  display: flex;
  align-items: center;
}

.ds-tabs__list {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  gap: 2px;
  padding: 3px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel-muted);
  overflow-x: auto;
  scrollbar-width: none;
}

.ds-tabs__list::-webkit-scrollbar {
  display: none;
}

.ds-tabs__tab {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  height: var(--ds-control-height-tab);
  padding: 0 14px;
  border: 1px solid transparent;
  border-radius: var(--ds-radius-control);
  background: transparent;
  color: var(--ds-muted);
  cursor: pointer;
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
  transition:
    background-color 140ms ease,
    color 140ms ease,
    box-shadow 140ms ease;
}

.ds-tabs__tab:hover:not(.ds-tabs__tab--active) {
  color: var(--ds-ink);
  background: color-mix(in srgb, var(--ds-panel) 70%, transparent);
}

.ds-tabs__tab:focus-visible {
  outline: 2px solid var(--ds-accent);
  outline-offset: 1px;
}

/* 激活块:白底 + 细边 + 细阴影,从灰槽里"浮起来" */
.ds-tabs__tab--active,
.ds-tabs__tab--active:hover {
  border-color: var(--ds-line);
  background: var(--ds-panel);
  color: var(--ds-ink);
  font-weight: 600;
  box-shadow: var(--ds-shadow-sm);
}
</style>
