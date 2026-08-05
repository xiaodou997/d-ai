<!--
  调度规则表格 — 分组详情「请求规则」页签复用的展示组件。
  重构:el-table 改 DsTable(props/emits 与列语义保持不变,优先级右对齐);
       无拖拽排序等附加交互。
-->
<script setup lang="ts">
import { DsTable, type DsTableColumn } from "@dai/ui";

import type { TenantAiDispatchRule } from "../../../../types/aiTenant";
import { surfaceLabel } from "../catalog";
import { dispatchMatchOptions } from "../dispatchRulePresentation";

defineProps<{ rules: readonly TenantAiDispatchRule[]; loading: boolean; saving: boolean; unpricedRuleIds: ReadonlySet<string> }>();
const emit = defineEmits<{ edit: [rule: TenantAiDispatchRule]; remove: [rule: TenantAiDispatchRule]; toggle: [rule: TenantAiDispatchRule] }>();

const columns: DsTableColumn[] = [
  { key: "clientSurface", title: "客户端 API 格式", width: 210 },
  { key: "matchType", title: "匹配方式", width: 168 },
  { key: "match_value", title: "匹配值", width: 180 },
  { key: "target_model_code", title: "目标逻辑模型", width: 170 },
  { key: "priority", title: "优先级", width: 82, align: "right" },
  { key: "status", title: "状态", width: 92 },
  { key: "notes", title: "备注" },
  { key: "actions", title: "操作", width: 112, align: "right" }
];
</script>

<template>
  <DsTable
    :columns="columns"
    :rows="[...rules]"
    row-key="id"
    :loading="loading"
    empty-title="暂无调度规则"
  >
    <template #cell-clientSurface="{ row }">{{ surfaceLabel(row.client_surface) }}</template>
    <template #cell-matchType="{ row }">
      <span class="match-type-label">{{ dispatchMatchOptions.find((item) => item.value === row.match_type)?.label || row.match_type }}</span>
    </template>
    <template #cell-status="{ row }">
      <el-switch
        :model-value="row.status === 'active'"
        :loading="saving"
        :disabled="saving || (row.status !== 'active' && unpricedRuleIds.has(row.id))"
        inline-prompt
        active-text="启用"
        inactive-text="停用"
        @change="emit('toggle', row)"
      />
    </template>
    <template #cell-actions="{ row }">
      <el-button link type="primary" size="small" :disabled="saving" @click="emit('edit', row)">编辑</el-button>
      <el-button link type="danger" size="small" :disabled="saving" @click="emit('remove', row)">删除</el-button>
    </template>
  </DsTable>
</template>

<style scoped>
.match-type-label {
  display: inline-block;
  max-width: 100%;
  white-space: normal;
  line-height: 1.35;
}
</style>
