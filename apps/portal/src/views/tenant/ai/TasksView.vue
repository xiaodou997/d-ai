<!--
  任务中心路由入口，实际内容由 features/ai/tasks 的租户工作区承载。
-->
<script setup lang="ts">
import { TenantTasksWorkspace } from "@/features/ai/tasks";
</script>

<template>
  <TenantTasksWorkspace />
</template>
