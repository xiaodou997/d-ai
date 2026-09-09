<!--
  租户使用分析筛选带:DsFilterBar + DsFilterField(标签在上),
  时间范围复用管理端记录页的 UsageRangeSelector,其余查询条件保持一致。
-->
<script setup lang="ts">
import { Search } from "@element-plus/icons-vue";
import { DsFilterBar, DsFilterField } from "@/shared/ui";
import { requestSourceOptions } from "@/platform/ai/usage";
import UsageRangeSelector from "./UsageRangeSelector.vue";

import {
  DEFAULT_WORKBENCH_RANGE_ID,
  WORKBENCH_RANGE_OPTIONS,
  type WorkbenchRangeId,
  type WorkbenchRangeOption
} from "@/components/workbench/workbenchRanges";
import type { TenantUsageFilters, TenantUsageUser } from "../model";

withDefaults(defineProps<{
  loading: boolean;
  users: TenantUsageUser[];
  showUser?: boolean;
  rangeId?: WorkbenchRangeId;
  rangeOptions?: WorkbenchRangeOption[];
}>(), {
  rangeId: DEFAULT_WORKBENCH_RANGE_ID,
  rangeOptions: () => [...WORKBENCH_RANGE_OPTIONS]
});

const filters = defineModel<TenantUsageFilters>({ required: true });
const customRange = defineModel<[number, number] | null>("customRange", { default: null });
const emit = defineEmits<{
  search: [];
  rangeChange: [value: WorkbenchRangeId];
}>();

const statusOptions = [
  { label: "成功", value: "success" },
  { label: "失败", value: "failed" },
  { label: "错误", value: "error" },
  { label: "待处理", value: "pending" }
];

function handleCustomRange(value: unknown) {
  if (!Array.isArray(value) || value.length !== 2 || !value.every((part) => typeof part === "number")) {
    customRange.value = null;
    return;
  }
  const range: [number, number] = [value[0], value[1]];
  customRange.value = range;
  emit("rangeChange", "custom");
}
</script>

<template>
  <DsFilterBar>
    <DsFilterField label="时间范围">
      <UsageRangeSelector
        :model-value="rangeId"
        :options="rangeOptions"
        :custom-range="customRange"
        @update:model-value="emit('rangeChange', $event)"
        @update:custom-range="handleCustomRange"
      />
    </DsFilterField>
    <DsFilterField v-if="showUser !== false" label="用户">
      <el-select v-model="filters.userId" placeholder="全部用户" clearable filterable class="usage-filter">
        <el-option
          v-for="user in users"
          :key="user.userId"
          :label="user.username || user.email"
          :value="String(user.userId)"
        />
      </el-select>
    </DsFilterField>
    <DsFilterField label="模型">
      <el-input v-model="filters.modelCode" placeholder="模型（如 gpt-4o）" clearable class="usage-filter" />
    </DsFilterField>
    <DsFilterField label="状态">
      <el-select v-model="filters.requestStatus" placeholder="全部状态" clearable class="usage-filter">
        <el-option v-for="status in statusOptions" :key="status.value" :label="status.label" :value="status.value" />
      </el-select>
    </DsFilterField>
    <DsFilterField label="来源">
      <el-select v-model="filters.requestSource" placeholder="全部来源" clearable class="usage-filter">
        <el-option v-for="source in requestSourceOptions" :key="source.value" :label="source.label" :value="source.value" />
      </el-select>
    </DsFilterField>
    <template #actions>
      <el-button type="primary" :icon="Search" :loading="loading" @click="$emit('search')">查询</el-button>
    </template>
  </DsFilterBar>
</template>

<style scoped>
/* DsFilterField 是纵向 flex(shrink-to-fit),flex-basis 会作用到高度而非宽度,
   故只用固定 width,不用 flex;与订阅面板 filter-select 写法一致 */
.usage-filter {
  width: 200px;
}
</style>
