<script setup lang="ts">
import { computed, shallowRef, watch } from "vue";
import { RefreshCw } from "lucide-vue-next";
import { DsEmpty, DsSkeleton, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

import type {
  TenantAiGroupEffectivePrice,
  TenantAiGroupEffectivePricesOutputBody,
  TenantAiVisibleGroup
} from "@/api/types/aiTenant";
import { capabilityLabels } from "../catalog";
import { errorMessage } from "../errorMessage";

interface PreviewRow extends TenantAiGroupEffectivePrice {
  key: string;
}

const props = defineProps<{
  modelValue: boolean;
  group: TenantAiVisibleGroup | null;
  loadModels: (groupId: string) => Promise<TenantAiGroupEffectivePricesOutputBody>;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

const loading = shallowRef(false);
const models = shallowRef<TenantAiGroupEffectivePrice[]>([]);
const loadError = shallowRef("");
let loadGeneration = 0;

const columns: DsTableColumn[] = [
  { key: "model", title: "模型编码", mono: true },
  { key: "capability", title: "能力类型", width: 150 }
];

const rows = computed<PreviewRow[]>(() =>
  [...models.value]
    .sort((left, right) => {
      const modelOrder = left.model_code.localeCompare(right.model_code, "zh-CN", { numeric: true });
      return modelOrder || left.capability_type.localeCompare(right.capability_type, "zh-CN");
    })
    .map((item) => ({ ...item, key: `${item.model_code}::${item.capability_type}` }))
);

const dialogTitle = computed(() => props.group ? `可用模型预览 · ${props.group.name}` : "可用模型预览");

function capabilityTone(value: string): "neutral" | "accent" | "positive" | "warning" | "info" {
  if (value === "chat") return "accent";
  if (value === "image") return "positive";
  if (value === "video") return "warning";
  if (value === "audio_tts" || value === "audio_stt") return "info";
  return "neutral";
}

async function loadPreview() {
  if (!props.group) return;
  const generation = ++loadGeneration;
  loading.value = true;
  loadError.value = "";
  models.value = [];
  try {
    const response = await props.loadModels(props.group.id);
    if (generation !== loadGeneration) return;
    models.value = response.items || [];
  } catch (error: unknown) {
    if (generation !== loadGeneration) return;
    loadError.value = errorMessage(error, "加载可用模型失败，请稍后重试");
  } finally {
    if (generation === loadGeneration) loading.value = false;
  }
}

watch(
  [() => props.modelValue, () => props.group?.id],
  ([open, groupId]) => {
    if (!open || !groupId) {
      loadGeneration += 1;
      return;
    }
    void loadPreview();
  },
  { immediate: true }
);
</script>

<template>
  <el-dialog
    :model-value="modelValue"
    :title="dialogTitle"
    width="680px"
    class="group-model-preview-dialog"
    append-to-body
    destroy-on-close
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="preview-heading">
      <p>按当前分组状态、可用上游关联和生效价格计算。</p>
      <DsTag v-if="!loadError" tone="info">{{ loading ? "加载中" : `${rows.length} 个模型` }}</DsTag>
    </div>

    <DsSkeleton v-if="loading" :rows="6" class="preview-loading" />

    <DsEmpty
      v-else-if="loadError"
      title="可用模型加载失败"
      :description="loadError"
      class="preview-empty"
    >
      <template #action>
        <el-button :icon="RefreshCw" @click="loadPreview">重试</el-button>
      </template>
    </DsEmpty>

    <div v-else class="preview-table-wrap">
      <DsTable :columns="columns" :rows="rows" row-key="key">
        <template #empty>
          <DsEmpty
            title="暂无可用模型"
            description="请检查分组状态、上游关联和价格表配置"
          />
        </template>
        <template #cell-model="{ row }">
          <code class="model-code">{{ row.model_code }}</code>
        </template>
        <template #cell-capability="{ row }">
          <DsTag :tone="capabilityTone(row.capability_type)">
            {{ capabilityLabels[row.capability_type] || row.capability_type }}
          </DsTag>
        </template>
      </DsTable>
    </div>

    <p class="preview-note">具体请求仍会继续应用客户端入口策略和模型调度规则。</p>

    <template #footer>
      <el-button @click="emit('update:modelValue', false)">关闭</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.preview-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.preview-heading p,
.preview-note {
  margin: 0;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.6;
}

.preview-loading {
  padding-block: 12px;
}

.preview-table-wrap {
  max-height: min(460px, 56vh);
  overflow: auto;
}

.preview-empty {
  min-height: 240px;
}

.model-code {
  color: var(--ds-ink);
  font-family: var(--ds-font-mono);
  font-size: 12px;
  font-weight: 600;
}

.preview-note {
  margin-top: 12px;
}

:global(.el-dialog.group-model-preview-dialog) {
  width: min(680px, calc(100vw - 24px)) !important;
}

@media (max-width: 640px) {
  .preview-heading {
    align-items: flex-start;
  }

  :global(.el-dialog.group-model-preview-dialog) {
    width: calc(100vw - 16px) !important;
    margin: 8px auto;
  }
}
</style>
