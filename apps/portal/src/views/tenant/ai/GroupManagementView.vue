<!--
  分组管理列表页 — 仅承担路由入口,实际内容见 features/ai/group-management/GroupManagementWorkspace
  (已迁移 DsUI 一体面板:PortalPagePanel + DsFilterBar + DsTable + DsPagination)。
-->
<script setup lang="ts">
import { GroupManagementWorkspace } from "@/features/ai/group-management";
</script>

<template>
  <GroupManagementWorkspace />
</template>
