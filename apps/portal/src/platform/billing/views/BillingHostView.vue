<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";

const route = useRoute();

const currentPath = computed(() => {
  const pathMatch = route.params.pathMatch;
  if (Array.isArray(pathMatch)) {
    return pathMatch.join("/");
  }
  return typeof pathMatch === "string" ? pathMatch : "";
});
</script>

<template>
  <section class="billing-host">
    <div class="billing-host__eyebrow">URM Shared Surface</div>
    <h2 class="billing-host__title">共享账单与账户页面将在下一步迁入</h2>
    <p class="billing-host__description">
      当前路由已挂载 `@/platform/billing` 宿主入口，后续 T4.3/T4.4/T4.5 会把 V1 的账户、
      充值、流水、租户与用户管理页面按 portal 逐步迁移进来。
    </p>
    <div class="billing-host__path">当前共享路由：{{ currentPath || "index" }}</div>
  </section>
</template>

<style scoped>
.billing-host {
  border: 1px dashed var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: color-mix(in srgb, var(--ds-panel) 88%, #ffffff 12%);
  padding: 28px;
}

.billing-host__eyebrow {
  color: var(--ds-accent);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.billing-host__title {
  margin: 12px 0 10px;
  font-family: var(--ds-font-display);
  font-size: 28px;
  font-weight: 600;
}

.billing-host__description {
  margin: 0;
  max-width: 720px;
  color: var(--ds-muted);
  line-height: 1.7;
}

.billing-host__path {
  margin-top: 18px;
  color: var(--ds-ink);
  font-size: 13px;
  font-weight: 600;
}
</style>
