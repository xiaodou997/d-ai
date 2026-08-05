<script setup lang="ts">
import { computed } from "vue";
import { Image, MessageSquareText, Save, ScanSearch } from "lucide-vue-next";

import type { TenantAiClientSurface } from "@/api/types/aiTenant";
import { clientSurfaceOptions } from "../catalog";
import { useGroupClientSurfacePolicy } from "../composables/useGroupClientSurfacePolicy";

const props = defineProps<{ groupId: string }>();
const policy = useGroupClientSurfacePolicy({ groupId: () => props.groupId });

const modeModel = computed<"all" | "restricted">({
  get: () => policy.mode.value,
  set: policy.setMode
});
const selectedModel = computed<TenantAiClientSurface[]>({
  get: () => [...policy.selectedSurfaces.value],
  set: policy.setSelectedSurfaces
});
const groupedOptions = [
  { id: "chat", label: "对话与文本", icon: MessageSquareText, items: clientSurfaceOptions.filter((item) => item.capability === "chat") },
  { id: "embedding", label: "向量嵌入", icon: ScanSearch, items: clientSurfaceOptions.filter((item) => item.capability === "embedding") },
  { id: "image", label: "图片生成", icon: Image, items: clientSurfaceOptions.filter((item) => item.capability === "image") }
];

defineExpose({ confirmDiscardChanges: policy.confirmDiscardChanges });
</script>

<template>
  <section v-loading="policy.loading.value" class="policy-panel">
    <header class="panel-header">
      <div>
        <div class="title-row">
          <h3>API 入口</h3>
          <el-tag :type="policy.mode.value === 'all' ? 'success' : 'warning'" effect="plain" size="small">
            {{ policy.mode.value === "all" ? "全部开放" : `${policy.selectedCount.value} 个入口` }}
          </el-tag>
        </div>
      </div>
      <el-segmented v-model="modeModel" :disabled="policy.saving.value" :options="[{ label: '全部开放', value: 'all' }, { label: '自定义限制', value: 'restricted' }]" />
    </header>

    <div v-if="policy.mode.value === 'restricted'" class="preset-row">
      <span>快捷预设</span>
      <el-button size="small" :icon="MessageSquareText" @click="policy.applyPreset('conversation')">仅对话</el-button>
      <el-button size="small" :icon="ScanSearch" @click="policy.applyPreset('embedding')">仅向量</el-button>
      <el-button size="small" :icon="Image" @click="policy.applyPreset('image')">仅图片</el-button>
    </div>

    <el-checkbox-group v-if="policy.mode.value === 'restricted'" v-model="selectedModel" class="surface-groups" :disabled="policy.saving.value">
      <section v-for="category in groupedOptions" :key="category.id" class="surface-category">
        <div class="category-label">
          <component :is="category.icon" :size="17" />
          <strong>{{ category.label }}</strong>
        </div>
        <div class="surface-list">
          <el-checkbox v-for="item in category.items" :key="item.id" :value="item.id" class="surface-option">
            <span class="surface-copy">
              <strong>{{ item.name }}</strong>
              <code>{{ item.endpoint }}</code>
            </span>
          </el-checkbox>
        </div>
      </section>
    </el-checkbox-group>

    <div v-else class="all-open-state">
      <MessageSquareText :size="20" />
      <strong>8 个客户端入口均已开放</strong>
    </div>

    <footer class="panel-footer">
      <el-button type="primary" :icon="Save" :loading="policy.saving.value" :disabled="!policy.canSave.value" @click="policy.save">保存</el-button>
    </footer>
  </section>
</template>

<style scoped>
.policy-panel {
  display: flex;
  min-height: 360px;
  flex-direction: column;
  gap: 18px;
}

.panel-header,
.title-row,
.preset-row,
.category-label,
.surface-option,
.all-open-state,
.panel-footer {
  display: flex;
  align-items: center;
}

.panel-header {
  justify-content: space-between;
  gap: 16px;
}

.title-row {
  gap: 9px;
}

.title-row h3 {
  margin: 0;
  color: var(--ds-ink);
  font-size: 16px;
  letter-spacing: 0;
}

.preset-row {
  flex-wrap: wrap;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--ds-line);
  border-radius: 6px;
  background: var(--ds-panel-muted);
  color: var(--ds-muted);
  font-size: 12px;
}

.surface-groups {
  display: grid;
  border-top: 1px solid var(--ds-line);
}

.surface-category {
  display: grid;
  grid-template-columns: 170px minmax(0, 1fr);
  gap: 20px;
  padding: 16px 0;
  border-bottom: 1px solid var(--ds-line);
}

.category-label {
  align-self: start;
  gap: 8px;
  min-height: 32px;
  color: var(--ds-ink);
  font-size: 13px;
}

.surface-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 18px;
}

.surface-option {
  min-width: 0;
  min-height: 52px;
  align-items: flex-start;
}

.surface-copy {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 4px;
}

.surface-copy strong {
  overflow-wrap: anywhere;
  color: var(--ds-ink);
  font-size: 13px;
}

.surface-copy code {
  overflow: hidden;
  color: var(--ds-muted);
  font-size: 11px;
  text-overflow: ellipsis;
}

.all-open-state {
  min-height: 72px;
  gap: 12px;
  padding: 14px;
  border: 1px solid color-mix(in srgb, var(--ds-positive) 28%, var(--ds-line));
  border-left: 3px solid var(--ds-positive);
  border-radius: 6px;
  background: color-mix(in srgb, var(--ds-positive) 7%, var(--ds-panel));
  color: var(--ds-positive);
}

.all-open-state strong {
  color: var(--ds-ink);
  font-size: 13px;
}

.panel-footer {
  justify-content: flex-end;
  gap: 10px;
}

@media (max-width: 720px) {
  .panel-header {
    align-items: stretch;
    flex-direction: column;
  }

  .surface-category,
  .surface-list {
    grid-template-columns: 1fr;
  }
}
</style>
