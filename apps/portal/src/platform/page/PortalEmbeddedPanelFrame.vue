<!--
  嵌入态面板裸框架:与 PortalPagePanel 同插槽(操作区在内容上方、分页脚沉底),
  用于已经由外层面板(如 PortalKeyManagementWorkspace)提供页头/边框的工作区。

  必须是真正的 SFC:此前各工作区用函数式组件 h() 就地拼这套结构,渲染出的元素拿不到
  宿主 SFC 的 scopeId,scoped 样式(flex 撑满 + 分页脚 border-top/padding)整段失效,
  分页脚因此不沉底也没有分隔线。
-->
<script setup lang="ts">
withDefaults(
  defineProps<{
    /** 是否渲染内容上方的操作条带;外层面板已统一渲染页头按钮时传 false */
    showActions?: boolean;
  }>(),
  { showActions: true }
);
</script>

<template>
  <div class="portal-embedded-frame">
    <div v-if="showActions && $slots.actions" class="portal-embedded-frame__actions">
      <slot name="actions" />
    </div>

    <div class="portal-embedded-frame__body">
      <slot />
    </div>

    <div v-if="$slots.pagination" class="portal-embedded-frame__pagination">
      <slot name="pagination" />
    </div>
  </div>
</template>

<style scoped>
.portal-embedded-frame {
  display: flex;
  flex: 1;
  min-height: 0;
  min-width: 0;
  flex-direction: column;
}

.portal-embedded-frame__actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  padding: 12px 24px;
  border-bottom: 1px solid var(--ds-line);
}

.portal-embedded-frame__body {
  display: flex;
  flex: 1;
  min-height: 0;
  min-width: 0;
  flex-direction: column;
}

/* DsTable 撑满剩余高度并内部滚动,空态纵向居中 */
.portal-embedded-frame__body :deep(.ds-table) {
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}

.portal-embedded-frame__body :deep(.ds-table__empty) {
  flex: 1;
  justify-content: center;
}

.portal-embedded-frame__pagination {
  display: flex;
  align-items: center;
  flex-shrink: 0;
  padding: 12px 24px;
  border-top: 1px solid var(--ds-line);
  background: var(--ds-panel);
}

@media (max-width: 768px) {
  .portal-embedded-frame__actions,
  .portal-embedded-frame__pagination {
    padding-inline: 16px;
  }
}
</style>
