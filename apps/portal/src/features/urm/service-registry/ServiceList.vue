<!--
  服务列表 — 迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       筛选/表格/分页同卡）。
  说明：服务注册为一次性全量拉取（listServices），关键字过滤仍由 useServiceRegistry
       在客户端完成；本组件只对过滤结果做前端切片分页，不引入服务端分页参数。
-->
<script setup lang="ts">
import { computed, ref, watch, type Component } from "vue";
import { Plus, RefreshCw, Search } from "lucide-vue-next";

import { PortalPagePanel, type PortalPagePanelBreadcrumb } from "@/platform";
import {
  DsFilterBar,
  DsFilterField,
  DsPagination,
  DsTable,
  DsTag,
  type DsTableColumn
} from "@/shared/ui";

import type { ServiceRegistryItem } from "@/api/types/admin";

const props = defineProps<{
  /** 页头身份图标(lucide 组件),由页面组合层传入 */
  icon?: Component;
  /** 面包屑路径,末级即页面标题 */
  breadcrumbs: PortalPagePanelBreadcrumb[];
  /** 页面描述,尾随面包屑同行 */
  description?: string;
  services: ServiceRegistryItem[];
  loading: boolean;
  keyword: string;
  portalModuleLabels: Readonly<Record<string, string>>;
}>();

const emit = defineEmits<{
  select: [service: ServiceRegistryItem];
  create: [];
  refresh: [];
  "update:keyword": [value: string];
}>();

const columns: DsTableColumn[] = [
  { key: "service", title: "服务" },
  { key: "status", title: "服务状态", width: 110 },
  { key: "portal", title: "门户入口", width: 120 },
  { key: "module", title: "前端模块", width: 170 },
  { key: "onlineInstances", title: "实例", width: 110, align: "right" },
  { key: "sourceCount", title: "外部来源", width: 100, align: "right" },
  { key: "lastSeen", title: "最后活动", width: 190 }
];

// 客户端分页：仅对传入的（已过滤）列表做切片
const page = ref(1);
const pageSize = ref(20);

const pagedServices = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return props.services.slice(start, start + pageSize.value);
});

// 过滤结果变化时回到第一页，避免停留在超出范围的页码
watch(
  () => props.services,
  () => {
    page.value = 1;
  }
);

function handlePageChange(value: number) {
  page.value = value;
}

function handlePageSizeChange(value: number) {
  pageSize.value = value;
  page.value = 1;
}

function statusTone(status: string): "positive" | "neutral" {
  return status === "active" ? "positive" : "neutral";
}

function portalTone(service: ServiceRegistryItem): "neutral" | "accent" | "warning" {
  if (!service.portalEnabled) return "neutral";
  return service.status === "active" ? "accent" : "warning";
}

function formatTime(value?: string) {
  return value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "从未连接";
}

function portalLabel(service: ServiceRegistryItem) {
  if (!service.portalEnabled) return "未开放";
  return service.status === "active" ? "已开放" : "待服务启用";
}

function portalModuleLabel(service: ServiceRegistryItem) {
  return props.portalModuleLabels[service.serviceId];
}
</script>

<template>
  <PortalPagePanel :icon="icon" :breadcrumbs="breadcrumbs" :description="description">
    <template #filters>
      <DsFilterBar>
        <DsFilterField label="关键词">
          <el-input
            :model-value="keyword"
            class="service-search"
            clearable
            placeholder="搜索 Service ID、名称或备注"
            @update:model-value="emit('update:keyword', $event)"
          >
            <template #prefix><Search :size="16" class="service-search__icon" /></template>
          </el-input>
        </DsFilterField>

        <template #actions>
          <el-button @click="emit('refresh')">
            <RefreshCw :size="16" class="service-button-icon" />
            刷新
          </el-button>
          <el-button type="primary" @click="emit('create')">
            <Plus :size="16" class="service-button-icon" />
            注册服务
          </el-button>
        </template>
      </DsFilterBar>
    </template>

    <DsTable
      :frame="false"
      :columns="columns"
      :rows="pagedServices"
      row-key="serviceId"
      :loading="loading"
      empty-title="暂无服务"
    >
      <template #cell-service="{ row }">
        <div class="service-cell">
          <button type="button" class="service-cell__name" @click="emit('select', row)">{{ row.displayName }}</button>
          <code class="service-cell__id">{{ row.serviceId }}</code>
        </div>
      </template>
      <template #cell-status="{ row }">
        <DsTag :tone="statusTone(row.status)">{{ row.status === "active" ? "已启用" : "已停用" }}</DsTag>
      </template>
      <template #cell-portal="{ row }">
        <DsTag :tone="portalTone(row)">{{ portalLabel(row) }}</DsTag>
      </template>
      <template #cell-module="{ row }">
        <DsTag v-if="portalModuleLabel(row)" tone="positive">已接入 · {{ portalModuleLabel(row) }}</DsTag>
        <DsTag v-else tone="neutral">未接入</DsTag>
      </template>
      <template #cell-onlineInstances="{ row }">
        <span class="service-num"><strong>{{ row.onlineInstances }}</strong> 在线</span>
      </template>
      <template #cell-sourceCount="{ row }">
        <span class="service-num">{{ row.sourceCount }}</span>
      </template>
      <template #cell-lastSeen="{ row }">
        <span class="service-time">{{ formatTime(row.lastSeen) }}</span>
      </template>
    </DsTable>

    <template #pagination>
      <DsPagination
        :page="page"
        :page-size="pageSize"
        :total="services.length"
        @update:page="handlePageChange"
        @update:page-size="handlePageSizeChange"
      />
    </template>
  </PortalPagePanel>
</template>

<style scoped>
.service-search {
  width: min(360px, 100%);
}

.service-search__icon {
  color: var(--ds-faint);
}

.service-button-icon {
  margin-right: 4px;
  vertical-align: -3px;
}

.service-cell {
  display: grid;
  gap: 3px;
  justify-items: start;
}

.service-cell__name {
  padding: 0;
  border: none;
  background: transparent;
  font-size: inherit;
  font-weight: 700;
  color: var(--ds-accent);
  cursor: pointer;
}

.service-cell__name:hover {
  color: var(--ds-accent-hover);
  text-decoration: underline;
}

.service-cell__id {
  color: var(--ds-muted);
  font-size: 12px;
}

.service-num {
  font-variant-numeric: tabular-nums;
}

.service-time {
  font-size: 12px;
  color: var(--ds-faint);
}
</style>
