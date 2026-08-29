<!-- 为指定用户沿用租户默认分组策略，或增加单独配置。 -->
<script setup lang="ts">
import { DsEmpty, DsSwitch, DsTable, type DsTableColumn } from "@/shared/ui";

import type { UserPolicyTarget } from "@/features/ai/user-management/model";
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

const columns: DsTableColumn[] = [
  { key: "name", title: "分组" },
  { key: "tenantDefault", title: "租户默认", width: 180 },
  { key: "user_bound", title: "单独配置", width: 110 },
  { key: "multiplier", title: "该用户策略", width: 220 }
];

withDefaults(defineProps<{
  selectedUser: UserPolicyTarget | null;
  loading: boolean;
  rows: UserPricingGroupRow[];
  showTitle?: boolean;
}>(), {
  showTitle: true
})

defineEmits<{
  (e: "toggle-binding", row: UserPricingGroupRow, bind: boolean): void;
  (e: "edit-multiplier", row: UserPricingGroupRow): void;
}>()

function defaultMultiplier(row: UserPricingGroupRow) {
  return row.default_user_multiplier;
}

</script>

<template>
  <section class="groups-panel">
    <header v-if="showTitle" class="panel-title">
      分组策略{{ selectedUser ? ` · ${selectedUser.username}` : "" }}
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
        <template #cell-tenantDefault="{ row }">
          <span class="hint">
            {{ row.user_default_visible ? `开放 · ${formatMultiplier(row.default_user_multiplier)}` : "不开放" }}
          </span>
        </template>
        <template #cell-user_bound="{ row }">
          <DsSwitch
            :model-value="row.user_bound"
            @update:model-value="(value: boolean) => $emit('toggle-binding', row, value)"
          />
        </template>
        <template #cell-multiplier="{ row }">
          <template v-if="row.user_bound">
            <el-button link type="primary" @click="$emit('edit-multiplier', row)">
              <span class="numeric-value">
                {{ row.user_multiplier_override == null ? `已开放 · ${formatMultiplier(defaultMultiplier(row))}` : `已开放 · ${formatMultiplier(row.user_multiplier_override)}` }}
              </span>
            </el-button>
          </template>
          <span v-else class="hint">沿用租户默认</span>
        </template>
      </DsTable>
    </template>

    <DsEmpty v-else title="未选择用户" description="请先选择需要配置分组策略的用户" />
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
