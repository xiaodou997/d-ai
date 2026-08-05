<!--
  套餐适用分组与扣额倍率编辑表 — 被套餐编辑弹窗等复用。
  重构:el-table 迁移至 DsTable(:frame="false",外层 section 自带边框),状态 el-tag 换成
       DsTag;倍率输入使用设计系统数字输入框,props/emits 与业务逻辑不变。
-->
<script setup lang="ts">
import { computed } from "vue";
import { formatMultiplier } from "@/platform/ai/utils";
import { DsNumberInput, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

import type { TenantAiVisibleGroup, TenantSubPlanGroupInput } from "@/api/types/aiTenant";

const props = defineProps<{
  groups: TenantAiVisibleGroup[];
}>();

const model = defineModel<TenantSubPlanGroupInput[]>({ required: true });

const columns: DsTableColumn[] = [
  { key: "enable", title: "启用", width: 64, align: "center" },
  { key: "name", title: "分组" },
  { key: "payg", title: "分组用户倍率", width: 132, align: "right" },
  { key: "multiplier", title: "套餐扣额倍率", width: 170 }
];

const selectedById = computed(() => new Map(model.value.map((item) => [item.group_id, item])));

function paygMultiplier(group: TenantAiVisibleGroup): number {
  return group.default_user_multiplier;
}

function toggleGroup(group: TenantAiVisibleGroup, enabled: boolean) {
  if (!enabled) {
    model.value = model.value.filter((item) => item.group_id !== group.id);
    return;
  }
  if (selectedById.value.has(group.id)) return;
  const initial = paygMultiplier(group) > 0 ? paygMultiplier(group) : 1;
  model.value = [...model.value, { group_id: group.id, quota_debit_multiplier: Number(initial.toFixed(4)) }];
}

function updateMultiplier(groupId: string, value: number | null) {
  model.value = model.value.map((item) => item.group_id === groupId
    ? { ...item, quota_debit_multiplier: Number(value ?? 0) }
    : item);
}

function fillFromPayg() {
  model.value = model.value.map((item) => {
    const group = props.groups.find((candidate) => candidate.id === item.group_id);
    if (group?.status !== "active") return item;
    const value = group ? paygMultiplier(group) : item.quota_debit_multiplier;
    return { ...item, quota_debit_multiplier: Number(value.toFixed(4)) };
  });
}
</script>

<template>
  <section class="group-pricing">
    <div class="group-pricing__header">
      <div>
        <strong>适用分组</strong>
        <span>{{ model.length }} 个已启用</span>
      </div>
      <el-button size="small" :disabled="!model.length" @click="fillFromPayg">按分组用户倍率填充</el-button>
    </div>

    <el-alert
      v-if="!groups.length"
      type="warning"
      :closable="false"
      show-icon
      title="当前没有可加入套餐的分组"
    />

    <DsTable
      v-else
      :frame="false"
      :columns="columns"
      :rows="groups"
      row-key="id"
      empty-title="暂无可加入套餐的分组"
    >
      <template #cell-enable="{ row }">
        <el-checkbox
          :model-value="selectedById.has(row.id)"
          :disabled="row.status !== 'active' && !selectedById.has(row.id)"
          :aria-label="`启用 ${row.name}`"
          @change="toggleGroup(row, Boolean($event))"
        />
      </template>
      <template #cell-name="{ row }">
        <span>{{ row.name }}</span>
        <DsTag v-if="row.status !== 'active'" tone="danger" class="sale-state">不可用</DsTag>
      </template>
      <template #cell-payg="{ row }">×{{ formatMultiplier(paygMultiplier(row)) }}</template>
      <template #cell-multiplier="{ row }">
        <div class="multiplier-control">
          <DsNumberInput
            :model-value="selectedById.get(row.id)?.quota_debit_multiplier"
            :disabled="!selectedById.has(row.id)"
            :min="0.0001"
            :step="0.01"
            :precision="4"
            class="multiplier-input"
            @update:model-value="updateMultiplier(row.id, $event)"
          />
        </div>
      </template>
    </DsTable>
  </section>
</template>

<style scoped>
.group-pricing {
  overflow: hidden;
  width: 100%;
  border: 1px solid var(--ds-line);
  border-radius: 8px;
}

.group-pricing__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 48px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--ds-line);
  background: var(--ds-panel-muted);
}

.group-pricing__header > div {
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.group-pricing__header span {
  color: var(--ds-muted);
  font-size: 12px;
}

.multiplier-input {
  width: 138px;
}

.multiplier-control {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.multiplier-control small {
  color: var(--ds-warning);
  font-size: 11px;
  line-height: 1.2;
}

.sale-state {
  margin-left: 8px;
}

</style>
