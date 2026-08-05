<!--
  终端用户使用筛选带:DsFilterBar + DsFilterField(标签在上),
  el-select 固定 160px 宽;查询条件与原有字段完全一致。
-->
<script setup lang="ts">
import { Search } from "@element-plus/icons-vue";
import { DsFilterBar, DsFilterField } from "@dai/ui";
import { requestSourceOptions } from "@dai/app-core/ai/usage";

import type { UserUsageFilters } from "../model";

const filters = defineModel<UserUsageFilters>({ required: true });

defineProps<{
  loading: boolean;
}>();

defineEmits<{ search: [] }>();

const statusOptions = [
  { label: "成功", value: "success" },
  { label: "失败", value: "failed" },
  { label: "错误", value: "error" },
  { label: "待处理", value: "pending" }
];

const dateShortcuts = [
  { text: "近 7 天", value: () => rangeFromDays(6) },
  { text: "近 30 天", value: () => rangeFromDays(29) },
  { text: "近 90 天", value: () => rangeFromDays(89) }
];

function rangeFromDays(days: number) {
  const to = new Date();
  const from = new Date();
  from.setDate(from.getDate() - days);
  from.setHours(0, 0, 0, 0);
  return [from, to];
}
</script>

<template>
  <DsFilterBar>
    <DsFilterField label="时间范围">
      <el-date-picker
        v-model="filters.dateRange"
        type="datetimerange"
        range-separator="至"
        start-placeholder="开始时间"
        end-placeholder="结束时间"
        format="MM-DD HH:mm"
        value-format="x"
        :shortcuts="dateShortcuts"
        class="usage-filter-date"
      />
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
.usage-filter-date {
  width: 340px;
}

/* DsFilterField 内 el-select/el-input 固定 160px 宽 */
.usage-filter {
  flex: 0 1 160px;
  width: min(160px, 100%);
}
</style>
