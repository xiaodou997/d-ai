<!--
  用户端使用记录筛选带:DsFilterBar + DsFilterField(标签在上),
  el-select 固定 200px 宽、关键词 260px;来源/记录范围变更即触发服务端查询,
  状态变更即触发本地搜索,交互语义与原 PortalFilterBar 版本一致。
-->
<script setup lang="ts">
import { Search } from "@element-plus/icons-vue";
import { DsFilterBar, DsFilterField } from "@/shared/ui";
import { requestSourceOptions } from "@/platform/ai/usage";

import type { CustomerUsageFilters } from "../model";

defineProps<{ loading: boolean }>();
const filters = defineModel<CustomerUsageFilters>({ required: true });
defineEmits<{ reset: []; search: []; serverChange: [] }>();

const sourceOptions = [{ label: "全部来源", value: "" }, ...requestSourceOptions];
const statusOptions = [
  { label: "全部状态", value: "" },
  { label: "成功", value: "success" },
  { label: "失败", value: "failed" },
  { label: "错误", value: "error" },
  { label: "拒绝", value: "rejected" },
  { label: "待处理", value: "pending" }
];
const limitOptions = [
  { label: "最近 20 条", value: 20 },
  { label: "最近 50 条", value: 50 },
  { label: "最近 100 条", value: 100 }
];
</script>

<template>
  <DsFilterBar>
    <DsFilterField label="来源">
      <el-select v-model="filters.requestSource" placeholder="全部来源" class="usage-filter" @change="$emit('serverChange')">
        <el-option v-for="option in sourceOptions" :key="option.value" :label="option.label" :value="option.value" />
      </el-select>
    </DsFilterField>
    <DsFilterField label="状态">
      <el-select v-model="filters.requestStatus" placeholder="全部状态" class="usage-filter" @change="$emit('search')">
        <el-option v-for="option in statusOptions" :key="option.value" :label="option.label" :value="option.value" />
      </el-select>
    </DsFilterField>
    <DsFilterField label="记录范围">
      <el-select v-model="filters.limit" placeholder="记录范围" class="usage-filter" @change="$emit('serverChange')">
        <el-option v-for="option in limitOptions" :key="option.value" :label="option.label" :value="option.value" />
      </el-select>
    </DsFilterField>
    <DsFilterField label="关键词">
      <el-input
        v-model="filters.keyword"
        clearable
        placeholder="搜索请求 ID / 模型 / 应用 / 错误"
        class="usage-keyword"
        @keyup.enter="$emit('search')"
      />
    </DsFilterField>
    <template #actions>
      <el-button type="primary" :icon="Search" :loading="loading" @click="$emit('search')">查询</el-button>
      <el-button plain @click="$emit('reset')">重置</el-button>
    </template>
  </DsFilterBar>
</template>

<style scoped>
/* DsFilterField 是纵向 flex(shrink-to-fit),flex-basis 会作用到高度而非宽度:
   原来的 flex: 0 1 160px 把 160px 设成了高度,输入框被撑成方形;width 的 100%
   又回指 shrink-to-fit 的父级形成循环依赖,下拉框因此塌成一条。只用固定 width。 */
.usage-filter {
  width: 200px;
}

.usage-keyword {
  width: 260px;
}
</style>
