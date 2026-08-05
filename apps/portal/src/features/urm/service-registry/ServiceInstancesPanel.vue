<script setup lang="ts">
import type { ServiceInstanceItem } from "../../../types/admin";

defineProps<{ instances: ServiceInstanceItem[] }>();

function formatTime(value: string) {
  return new Date(value).toLocaleString("zh-CN", { hour12: false });
}
</script>

<template>
  <section class="instances-panel">
    <div v-if="instances.length === 0" class="instances-panel__empty">暂无在线实例</div>
    <div v-for="instance in instances" v-else :key="instance.instanceId" class="instance-row">
      <span :class="['instance-row__signal', { 'instance-row__signal--online': instance.online }]" />
      <div class="instance-row__identity">
        <strong>{{ instance.serviceName || instance.instanceId }}</strong>
        <code>{{ instance.instanceId }}</code>
      </div>
      <div class="instance-row__meta">
        <span>{{ instance.version || "版本未知" }}</span>
        <span>{{ instance.environment || "环境未知" }}</span>
      </div>
      <div class="instance-row__source">
        <code>{{ instance.observedIp }}</code>
        <span>{{ formatTime(instance.lastSeen) }}</span>
      </div>
    </div>
  </section>
</template>

<style scoped>
.instances-panel { display: grid; border-top: 1px solid var(--ds-line); }
.instances-panel__empty { padding: 28px 0; color: var(--ds-muted); text-align: center; }
.instance-row { min-height: 66px; display: grid; grid-template-columns: 12px minmax(180px, 1fr) minmax(130px, .7fr) minmax(180px, .8fr); align-items: center; gap: 12px; border-bottom: 1px solid var(--ds-line); }
.instance-row__signal { width: 8px; height: 8px; border-radius: 50%; background: var(--ds-faint); }
.instance-row__signal--online { background: var(--ds-positive); }
.instance-row__identity, .instance-row__source { display: grid; gap: 3px; min-width: 0; }
.instance-row__identity code, .instance-row__source code { overflow: hidden; color: var(--ds-muted); font-size: 11px; text-overflow: ellipsis; }
.instance-row__meta { display: flex; gap: 7px; color: var(--ds-muted); font-size: 12px; }
.instance-row__source { justify-items: end; color: var(--ds-muted); font-size: 11px; }
@media (max-width: 720px) { .instance-row { padding: 12px 0; grid-template-columns: 12px 1fr; } .instance-row__meta, .instance-row__source { grid-column: 2; justify-items: start; } }
</style>
