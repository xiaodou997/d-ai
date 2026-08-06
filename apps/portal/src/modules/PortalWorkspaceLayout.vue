<script setup lang="ts">
import { computed } from "vue";
import { RouterView, useRoute, useRouter } from "vue-router";

import { DsTabs } from "@/shared/ui";
import { useAuthStore } from "@/stores/auth";

import { portalModulesById, userHasPortalCapability } from "./portalModules";

const props = defineProps<{ moduleId: string }>();
const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();

const portalModule = computed(() => {
  const module = portalModulesById.get(props.moduleId);
  if (!module) throw new Error(`Unknown portal module: ${props.moduleId}`);
  return module;
});

const visibleTabs = computed(() =>
  (portalModule.value.tabs ?? []).filter(
    (tab) => tab.nav !== false && userHasPortalCapability(authStore.userType, tab.capability ?? portalModule.value.capability)
  )
);

const tabs = computed(() => visibleTabs.value.map((tab) => ({ key: tab.id, label: tab.label })));
const activeTab = computed({
  get: () => String(route.meta.portalTabId || visibleTabs.value[0]?.id || ""),
  set: (tabId: string) => {
    const tab = visibleTabs.value.find((item) => item.id === tabId);
    if (!tab) return;
    void router.push(`${portalModule.value.path}/${tab.path.split("/:", 1)[0]}`);
  }
});
</script>

<template>
  <div class="portal-workspace-layout">
    <nav v-if="!portalModule.navTabs" class="portal-workspace-layout__tabs" :aria-label="`${portalModule.label}视图`">
      <DsTabs v-model="activeTab" :tabs="tabs" />
    </nav>
    <div class="portal-workspace-layout__view">
      <RouterView />
    </div>
  </div>
</template>

<style scoped>
.portal-workspace-layout {
  display: flex;
  flex: 1;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
  gap: 12px;
}

.portal-workspace-layout__tabs {
  display: flex;
  align-items: center;
  min-height: 42px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--ds-line);
}

.portal-workspace-layout__view {
  display: flex;
  flex: 1;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
}

.portal-workspace-layout__view :deep(> *) {
  flex: 1;
  min-width: 0;
}

@media (max-width: 768px) {
  .portal-workspace-layout {
    gap: 10px;
  }
}
</style>
