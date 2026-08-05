<!--
  租户端账户中心页面(/account):仅承载 TenantAccountCenterWorkspace。
  页面骨架已迁移至 DsUI 一体面板(PortalPagePanel),详见 workspace 组件。
-->
<script setup lang="ts">
import TenantAccountCenterWorkspace from "@/features/urm/account-center/TenantAccountCenterWorkspace.vue";
</script>

<template>
  <TenantAccountCenterWorkspace />
</template>
