<script setup lang="ts">
import type { PortalShellLayoutProps } from "./portal-shell";
import { DsAppShell } from "@/shared/ui";
import { RouterView } from "vue-router";

defineProps<PortalShellLayoutProps>();

const emit = defineEmits<{ logout: [] }>();
defineSlots<{ "topbar-actions"(): unknown }>();
</script>

<template>
  <DsAppShell
    :theme="theme"
    :version="appVersion"
    :brand="brand"
    :brand-icon-url="brandIconUrl"
    :nav="nav"
    :user="user"
    :user-menu="userMenu"
    :logout-label="logoutLabel"
    @logout="emit('logout')"
  >
    <template v-if="$slots['topbar-actions']" #topbar-actions>
      <slot name="topbar-actions" />
    </template>
    <RouterView v-slot="{ Component }">
      <transition name="ds-fade" mode="out-in">
        <component :is="Component" />
      </transition>
    </RouterView>
  </DsAppShell>
</template>

<style>
/* 路由切换淡入淡出（全局类名，配合 transition name="ds-fade"） */
.ds-fade-enter-active,
.ds-fade-leave-active {
  transition: opacity 150ms ease-out;
}

.ds-fade-enter-from,
.ds-fade-leave-to {
  opacity: 0;
}
</style>
