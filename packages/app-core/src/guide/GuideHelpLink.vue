<script setup lang="ts">
// 刻意用原生 <a> 而不是 RouterLink：target="_blank" 本来就绕过 SPA 路由走整页
// 加载，RouterLink 在这里只是把同样的绝对路径渲染成同样的 href（三端 router
// 都是 createWebHistory() 无 base，也没有 <base> 标签）。去掉它，展示型组件
// 就不必依赖 vue-router。
defineProps<{
  to: string;
}>();
</script>

<template>
  <a :href="to" target="_blank" class="guide-help-link">
    使用说明
  </a>
</template>

<style scoped>
.guide-help-link {
  display: inline-flex;
  align-items: center;
  height: 32px;
  padding: 0 12px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel);
  color: var(--ds-muted);
  font-size: 13px;
  font-weight: 600;
  text-decoration: none;
  white-space: nowrap;
  transition: background 0.15s ease, color 0.15s ease, border-color 0.15s ease;
}

.guide-help-link:hover {
  background: var(--ds-panel-muted);
  color: var(--ds-ink);
  border-color: color-mix(in srgb, var(--ds-accent) 24%, var(--ds-line));
}
</style>
