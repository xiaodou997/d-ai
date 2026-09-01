<!--
  调度预览 — 分组详情「请求规则」页签内的试算面板。
  重构:el-table 改 DsTable(候选上游无唯一 id,合成 _key 行键),el-tag 改 DsTag,
       空态 DsEmpty;预览请求与交互保持不变。
-->
<script setup lang="ts">
import { computed, reactive, shallowRef, watch } from "vue";
import { ElMessage } from "element-plus";
import { Play } from "lucide-vue-next";
import { DsEmpty, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

import { aiTenantApi } from "@/api/aiTenant";
import type { TenantAiDispatchPreview } from "@/api/types/aiTenant";
import { clientSurfaceOptions, surfaceLabel } from "../catalog";
import { errorMessage } from "../problemPresentation";

const props = defineProps<{ groupId: string }>();
const loading = shallowRef(false);
const preview = shallowRef<TenantAiDispatchPreview | null>(null);
const form = reactive({
  client_surface: "openai_chat" as (typeof clientSurfaceOptions)[number]["id"],
  requested_model: ""
});
let generation = 0;

const upstreamColumns: DsTableColumn[] = [
  { key: "display_name", title: "候选上游", width: 180 },
  { key: "providerFamily", title: "协议", width: 130 },
  { key: "apiFormat", title: "上游 API 格式", width: 170 },
  { key: "conversion", title: "协议转换", width: 92 },
  { key: "priority", title: "优先级", width: 82, align: "right" },
  { key: "routingWeight", title: "分流权重", width: 92, align: "right" }
];

// 候选上游无唯一 id 字段,合成行 key 供 DsTable 使用
const upstreamRows = computed(() =>
  (preview.value?.candidate_upstreams ?? []).map((item, index) => ({
    ...item,
    _key: `${item.account_id ?? ""}:${item.credential_pool_id ?? ""}:${index}`
  }))
);

function providerFamilyLabel(value?: string) {
  return ({ openai_compatible: "OpenAI 兼容", anthropic: "Anthropic", gemini: "Gemini", google: "Gemini" } as Record<string, string>)[value || ""] || value || "不限制";
}

function routeStrategyLabel(value?: string) {
  return ({ failover: "严格故障转移", weighted: "按权重", adaptive: "自适应" } as Record<string, string>)[value || ""] || value || "未配置";
}

function routeObjectiveLabel(value?: string) {
  return ({ balanced: "均衡", cost: "成本", latency: "速度", stability: "稳定性" } as Record<string, string>)[value || ""] || value || "均衡";
}

async function runPreview() {
  const groupId = props.groupId;
  const requestedModel = form.requested_model.trim();
  if (!requestedModel) {
    ElMessage.warning("请输入请求模型");
    return;
  }
  const token = ++generation;
  loading.value = true;
  try {
    const result = await aiTenantApi.previewGroupDispatch(groupId, {
      client_surface: form.client_surface,
      requested_model: requestedModel
    });
    if (token === generation && groupId === props.groupId) preview.value = result;
  } catch (error: unknown) {
    if (token === generation) ElMessage.error(errorMessage(error, "调度预览失败"));
  } finally {
    if (token === generation) loading.value = false;
  }
}

watch(() => props.groupId, () => {
  generation++;
  preview.value = null;
  form.requested_model = "";
});
</script>

<template>
  <section class="preview-panel">
    <header class="panel-header">
      <h3>调度预览</h3>
      <el-button type="primary" size="small" :icon="Play" :loading="loading" @click="runPreview">执行预览</el-button>
    </header>

    <div class="preview-controls">
      <el-select v-model="form.client_surface" class="surface-select">
        <el-option v-for="item in clientSurfaceOptions" :key="item.id" :value="item.id" :label="`${item.name} · ${item.endpoint}`" />
      </el-select>
      <el-input v-model="form.requested_model" placeholder="请求模型" @keyup.enter="runPreview" />
    </div>

    <template v-if="preview">
      <el-descriptions :column="2" border size="small" class="preview-summary">
        <el-descriptions-item label="请求模型">{{ preview.requested_model }}</el-descriptions-item>
        <el-descriptions-item label="API 格式">{{ surfaceLabel(preview.client_surface) }}</el-descriptions-item>
        <el-descriptions-item label="命中规则">
          {{ preview.matched_rule ? `${preview.matched_rule.match_value} → ${preview.matched_rule.target_model_code}` : "未命中，按原模型路由" }}
        </el-descriptions-item>
        <el-descriptions-item label="映射后逻辑模型">{{ preview.resolved_logical_model }}</el-descriptions-item>
        <el-descriptions-item label="选择策略">{{ routeStrategyLabel(preview.route_strategy) }}</el-descriptions-item>
        <el-descriptions-item v-if="preview.route_strategy === 'adaptive'" label="优化目标">{{ routeObjectiveLabel(preview.route_objective) }}</el-descriptions-item>
        <el-descriptions-item label="候选上游">{{ preview.candidate_upstreams?.length || 0 }}</el-descriptions-item>
      </el-descriptions>

      <el-alert
        v-if="!preview.candidate_upstreams?.length"
        title="规则已完成解析，但当前没有可执行上游"
        type="warning"
        :closable="false"
        show-icon
      />

      <DsTable
        v-if="preview.candidate_upstreams?.length"
        :columns="upstreamColumns"
        :rows="upstreamRows"
        row-key="_key"
      >
        <template #cell-providerFamily="{ row }">{{ providerFamilyLabel(row.provider_family) }}</template>
        <template #cell-apiFormat="{ row }">{{ row.provider_api_format || "-" }}</template>
        <template #cell-conversion="{ row }">
          <DsTag :tone="row.protocol_conversion ? 'warning' : 'positive'">{{ row.protocol_conversion ? "需要" : "无需" }}</DsTag>
        </template>
        <template #cell-routingWeight="{ row }">{{ row.routing_weight }}</template>
      </DsTable>
    </template>

    <DsEmpty v-else title="暂无预览结果" description="选择 API 格式并输入请求模型后执行预览" />
  </section>
</template>

<style scoped>
.preview-panel {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.panel-header h3 {
  margin: 0;
  color: var(--ds-ink);
  font-size: 16px;
  letter-spacing: 0;
}

.preview-controls {
  display: grid;
  grid-template-columns: minmax(240px, 320px) minmax(220px, 1fr);
  gap: 10px;
}

.surface-select {
  width: 100%;
}

.preview-summary {
  margin-top: 2px;
}

@media (max-width: 680px) {
  .preview-controls {
    grid-template-columns: 1fr;
  }

  .preview-summary {
    overflow-x: auto;
  }

  .preview-summary :deep(.el-descriptions__table) {
    min-width: 620px;
  }
}
</style>
