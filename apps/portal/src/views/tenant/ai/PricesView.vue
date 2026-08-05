<!--
  租户端路由页「价格表」(/workspace/prices) — 薄壳,仅挂载价格表工作区组件。
  实际页面实现见 features/ai/price-books/TenantPriceBookWorkspace.vue(已迁移至 DsUI 一体面板)。
-->
<script setup lang="ts">
import { TenantPriceBookWorkspace } from "../../features/ai/price-books";
</script>

<template>
  <TenantPriceBookWorkspace />
</template>
