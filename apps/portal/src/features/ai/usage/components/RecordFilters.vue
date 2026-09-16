<script setup lang="ts">
import { DsFilterBar } from "@/shared/ui";
import type { RecordRole } from "../recordPresentation";
export interface RecordFilterValues { model: string; tenant_name: string; user_name: string; group: string; api_key_name: string; request_id: string; source: string }
const filters = defineModel<RecordFilterValues>({ required: true });
defineProps<{ role: RecordRole; fixedUser?: boolean; busy: boolean }>();
defineEmits<{ search: []; reset: []; refresh: [] }>();
</script>
<template>
  <DsFilterBar class="record-filters" @keyup.enter="$emit('search')">
    <el-input v-model="filters.model" clearable placeholder="模型 ID（完整编码）" aria-label="模型 ID" />
    <el-input v-if="role === 'admin'" v-model="filters.tenant_name" clearable placeholder="租户名" aria-label="租户名" />
    <el-input v-if="role !== 'customer' && !fixedUser" v-model="filters.user_name" clearable placeholder="用户名 / 昵称" aria-label="用户名" />
    <el-input v-model="filters.group" clearable placeholder="分组名称 / ID" aria-label="分组" />
    <el-input v-model="filters.api_key_name" clearable placeholder="API Key 名称" aria-label="API Key 名称" />
    <el-input v-model="filters.request_id" clearable placeholder="请求 ID（精确）" aria-label="请求 ID" />
    <el-select v-model="filters.source" clearable placeholder="全部来源" aria-label="请求来源"><el-option label="API Key" value="api_key" /><el-option label="网页对话" value="web_chat" /><el-option label="网页生图" value="web_image" /><el-option label="网页视频" value="web_video" /></el-select>
    <template #actions><el-button @click="$emit('reset')">重置</el-button><el-button :loading="busy" @click="$emit('refresh')">刷新</el-button><el-button type="primary" :loading="busy" @click="$emit('search')">查询</el-button></template>
  </DsFilterBar>
</template>
<style scoped>
.record-filters :deep(.el-input), .record-filters :deep(.el-select) { flex: 1 1 160px; width: auto; min-width: 145px; max-width: 230px; }
</style>
