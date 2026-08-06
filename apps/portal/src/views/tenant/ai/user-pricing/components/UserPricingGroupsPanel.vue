<!--
  分组例外配置面板 — 为选中的终端用户打开/关闭分组例外,并设置用户扣费倍率。
  重构:el-table → DsTable(:frame="false",倍率列右对齐),el-tag → DsTag,
       el-switch → DsSwitch,空态 → DsEmpty;props/emits 与业务逻辑保持不变。
-->
<script setup lang="ts">
import { DsEmpty, DsSwitch, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

import type { UserAiPolicyTarget } from "@/features/ai/user-management/model";
import { formatMultiplier } from "../presentation";

interface UserPricingGroupRow {
  id: string;
  name: string;
  default_user_multiplier: number;
  user_default_visible: boolean;
  user_bound: boolean;
  user_multiplier_override: number | null;
  availability_state: "default" | "custom" | "unavailable";
}

// DsTable 列:倍率列右对齐走 #cell-*;例外开关/状态/扣费倍率走 #cell-* 插槽
const columns: DsTableColumn[] = [
  { key: "name", title: "分组" },
  { key: "defaultMultiplier", title: "分组默认用户倍率", width: 160, align: "right" },
  { key: "user_bound", title: "用户例外", width: 110 },
  { key: "availability_state", title: "当前状态", width: 140 },
  { key: "multiplier", title: "用户扣费倍率", width: 200 }
];

defineProps<{
  selectedUser: UserAiPolicyTarget | null;
  loading: boolean;
  rows: UserPricingGroupRow[];
}>()

defineEmits<{
  (e: "toggle-binding", row: UserPricingGroupRow, bind: boolean): void;
  (e: "edit-multiplier", row: UserPricingGroupRow): void;
}>()

function defaultMultiplier(row: UserPricingGroupRow) {
  return row.default_user_multiplier;
}

function availabilityTone(state: UserPricingGroupRow["availability_state"]) {
  return state === "custom" ? "warning" : state === "default" ? "positive" : "neutral";
}

function availabilityLabel(state: UserPricingGroupRow["availability_state"]) {
  return state === "custom" ? "用户例外" : state === "default" ? "默认公开" : "未开放";
}
</script>

<template>
  <section class="groups-panel">
    <header class="panel-title">
      分组例外配置{{ selectedUser ? ` · ${selectedUser.username}` : "" }}
    </header>

    <template v-if="selectedUser">
      <DsTable
        :frame="false"
        :columns="columns"
        :rows="rows"
        row-key="id"
        :loading="loading"
        empty-title="暂无分组"
        empty-description="当前租户尚未创建分组"
      >
        <template #cell-defaultMultiplier="{ row }">
          <span class="numeric-value">{{ formatMultiplier(row.default_user_multiplier) }}</span>
        </template>
        <template #cell-user_bound="{ row }">
          <DsSwitch
            :model-value="row.user_bound"
            @update:model-value="(value: boolean) => $emit('toggle-binding', row, value)"
          />
        </template>
        <template #cell-availability_state="{ row }">
          <DsTag :tone="availabilityTone(row.availability_state)">{{ availabilityLabel(row.availability_state) }}</DsTag>
        </template>
        <template #cell-multiplier="{ row }">
          <template v-if="row.user_bound">
            <el-button link type="primary" @click="$emit('edit-multiplier', row)">
              <span class="numeric-value">{{ row.user_multiplier_override == null ? `继承默认 ${formatMultiplier(defaultMultiplier(row))}` : formatMultiplier(row.user_multiplier_override) }}</span>
            </el-button>
          </template>
          <span v-else-if="row.user_default_visible" class="hint">默认 {{ formatMultiplier(defaultMultiplier(row)) }}</span>
          <span v-else class="hint">未开放</span>
        </template>
      </DsTable>
    </template>

    <DsEmpty v-else title="未选择用户" description="从左侧选择一个终端用户进行分组例外与限流配置" />
  </section>
</template>

<style scoped>
.groups-panel {
  min-height: 240px;
}

.panel-title {
  margin-bottom: 12px;
  font-weight: 700;
  color: var(--ds-ink);
}

.hint {
  color: var(--ds-faint);
  font-size: 12px;
}

.numeric-value {
  font-variant-numeric: tabular-nums;
}
</style>
