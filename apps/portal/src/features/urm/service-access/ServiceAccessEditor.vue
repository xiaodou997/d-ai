<script setup lang="ts">
import { computed } from "vue";
import { Check, Server } from "lucide-vue-next";

import type { ServiceRegistryItem } from "../../../types/admin";

const props = defineProps<{
  services: ServiceRegistryItem[];
  allowAll: boolean;
  disabled?: boolean;
}>();

const mode = defineModel<"all" | "selected">("mode", { required: true });
const serviceIds = defineModel<string[]>("serviceIds", { required: true });

const modeOptions = computed(() => [
  ...(props.allowAll ? [{ label: "全部服务", value: "all" as const }] : []),
  { label: "指定服务", value: "selected" as const }
]);

const orderedServices = computed(() =>
  [...props.services].sort((a, b) => a.displayName.localeCompare(b.displayName, "zh-CN"))
);

function setMode(value: "all" | "selected") {
  mode.value = value;
  if (value === "all") serviceIds.value = [];
}
</script>

<template>
  <section class="service-access-editor" aria-label="业务服务准入">
    <div class="service-access-editor__mode">
      <div>
        <strong>业务服务准入</strong>
        <span>控制顶部业务 Tab 与对应服务令牌的签发</span>
      </div>
      <el-segmented
        :model-value="mode"
        :options="modeOptions"
        :disabled="disabled"
        @update:model-value="setMode($event as 'all' | 'selected')"
      />
    </div>

    <el-checkbox-group
      v-if="mode === 'selected'"
      v-model="serviceIds"
      class="service-access-editor__services"
      :disabled="disabled"
    >
      <el-checkbox v-for="service in orderedServices" :key="service.serviceId" :value="service.serviceId" border>
        <span class="service-access-editor__service">
          <Server :size="15" />
          <span><strong>{{ service.displayName }}</strong><code>{{ service.serviceId }}</code></span>
          <span v-if="service.status !== 'active'" class="service-access-editor__disabled">已停用</span>
          <Check v-else :size="14" class="service-access-editor__active" />
        </span>
      </el-checkbox>
      <p v-if="orderedServices.length === 0" class="service-access-editor__empty">暂无可授权的门户业务服务</p>
    </el-checkbox-group>
  </section>
</template>

<style scoped>
.service-access-editor { display: grid; gap: 14px; }
.service-access-editor__mode { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.service-access-editor__mode > div { display: grid; gap: 3px; }
.service-access-editor__mode strong { font-size: 14px; color: var(--el-text-color-primary); }
.service-access-editor__mode span { font-size: 12px; color: var(--el-text-color-secondary); }
.service-access-editor__services { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
.service-access-editor__services :deep(.el-checkbox) { width: 100%; height: 58px; margin: 0; padding: 0 12px; border-radius: 6px; }
.service-access-editor__services :deep(.el-checkbox__label) { min-width: 0; flex: 1; }
.service-access-editor__service { min-width: 0; display: flex; align-items: center; gap: 9px; }
.service-access-editor__service > span:nth-child(2) { min-width: 0; display: grid; gap: 2px; }
.service-access-editor__service strong, .service-access-editor__service code { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.service-access-editor__service strong { font-size: 13px; }
.service-access-editor__service code { font-size: 11px; color: var(--el-text-color-secondary); }
.service-access-editor__disabled { margin-left: auto; color: var(--el-color-warning); font-size: 11px; }
.service-access-editor__active { margin-left: auto; color: var(--el-color-success); }
.service-access-editor__empty { grid-column: 1 / -1; margin: 0; padding: 18px; text-align: center; color: var(--el-text-color-secondary); font-size: 12px; border: 1px dashed var(--el-border-color); }
@media (max-width: 620px) { .service-access-editor__mode { align-items: flex-start; flex-direction: column; } .service-access-editor__services { grid-template-columns: 1fr; } }
</style>
